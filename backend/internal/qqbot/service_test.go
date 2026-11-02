package qqbot

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

type testEncryptor struct{}

func (testEncryptor) Encrypt(v string) (string, error) { return "encrypted:" + v, nil }
func (testEncryptor) Decrypt(v string) (string, error) {
	if !strings.HasPrefix(v, "encrypted:") {
		return "", errors.New("bad ciphertext")
	}
	return strings.TrimPrefix(v, "encrypted:"), nil
}

type testRepo struct {
	Repository
	mu         sync.Mutex
	raw        string
	seen       map[string]bool
	finished   chan EventRecord
	reserved   int
	quotaErr   error
	checkinErr error
	stored     []Message
	codeHash   string
}

func (r *testRepo) LoadConfig(context.Context) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.raw, nil
}
func (r *testRepo) SaveConfig(_ context.Context, raw string, version int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var c Config
	_ = json.Unmarshal([]byte(r.raw), &c)
	if c.Version != version {
		return ErrConflict
	}
	r.raw = raw
	return nil
}
func (r *testRepo) BeginEvent(_ context.Context, e EventRecord) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.seen[e.ID] {
		return false, nil
	}
	r.seen[e.ID] = true
	return true, nil
}
func (r *testRepo) FinishEvent(_ context.Context, id, status, result string) error {
	r.finished <- EventRecord{ID: id, Status: status, Result: result}
	return nil
}
func (r *testRepo) Account(context.Context, string) (*Account, error) {
	return &Account{UserID: 42, Balance: 10, Active: true}, nil
}
func (r *testRepo) ReserveChat(context.Context, string, string, string, string, time.Time, int) ([]Message, error) {
	r.reserved++
	return []Message{{Role: "user", Content: "old group question"}}, r.quotaErr
}
func (r *testRepo) FinishChat(_ context.Context, _, _, _ string, m []Message) error {
	r.stored = m
	return nil
}
func (r *testRepo) CreateBindCode(_ context.Context, _ int64, hash string, _ time.Time) error {
	r.codeHash = hash
	return nil
}
func (r *testRepo) Checkin(context.Context, string, string, string, time.Time) (*Reward, error) {
	return &Reward{UserID: 42, Amount: 1, BalanceAfter: 11}, r.checkinErr
}

type testHost struct {
	invalidated int
	scope       int64
}

func (h *testHost) ChannelStatus(_ context.Context, id int64) (string, error) {
	h.scope = id
	return "scoped status", nil
}
func (h *testHost) InvalidateBalance(context.Context, int64) { h.invalidated++ }
func testService(t *testing.T) (*Service, *testRepo, *testHost, Config) {
	t.Helper()
	c := DefaultConfig()
	c.Enabled = true
	c.BotQQ = "123456"
	c.AllowedGroups = []string{"987654"}
	c.AccessToken = "encrypted:a-long-random-test-token-12345"
	c.ChatEnabled = true
	c.ChatModel = "site-model"
	c.APIKey = "encrypted:site-secret"
	raw, err := json.Marshal(c)
	require.NoError(t, err)
	r := &testRepo{raw: string(raw), seen: map[string]bool{}, finished: make(chan EventRecord, 20)}
	h := &testHost{}
	s := NewService(r, testEncryptor{}, h, "http://127.0.0.1:1")
	t.Cleanup(s.Stop)
	return s, r, h, c
}
func TestQQBotSecretsAndCAS(t *testing.T) {
	s, r, _, c := testService(t)
	public, err := s.Config(context.Background())
	require.NoError(t, err)
	require.Empty(t, public.AccessToken)
	require.Empty(t, public.APIKey)
	require.True(t, public.AccessTokenSet)
	public.ChatModel = "changed"
	saved, err := s.SaveConfig(context.Background(), public)
	require.NoError(t, err)
	require.Equal(t, 1, saved.Version)
	var stored Config
	require.NoError(t, json.Unmarshal([]byte(r.raw), &stored))
	require.Equal(t, c.AccessToken, stored.AccessToken)
	require.Equal(t, c.APIKey, stored.APIKey)
	_, err = s.SaveConfig(context.Background(), public)
	require.ErrorIs(t, err, ErrConflict)
	saved.APIKey = "replacement"
	saved.Enabled = false
	saved.ClearAccessToken = true
	_, err = s.SaveConfig(context.Background(), saved)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(r.raw), &stored))
	require.Equal(t, "encrypted:replacement", stored.APIKey)
	require.Empty(t, stored.AccessToken)
}
func TestQQBotCodeIsRandomAndOnlyHashStored(t *testing.T) {
	s, r, _, _ := testService(t)
	code, expiry, err := s.CreateCode(context.Background(), 42)
	require.NoError(t, err)
	require.Regexp(t, bindCodePattern, code)
	require.Equal(t, Hash(code), r.codeHash)
	require.WithinDuration(t, time.Now().Add(5*time.Minute), expiry, time.Second)
	second, _, err := s.CreateCode(context.Background(), 42)
	require.NoError(t, err)
	require.NotEqual(t, code, second)
}
func TestQQBotCommandParsing(t *testing.T) {
	cases := []struct {
		name, raw, command, arg string
		ok                      bool
	}{
		{"plain", `[{"type":"text","data":{"text":"余额"}}]`, "", "", false},
		{"command", `[{"type":"text","data":{"text":"/余额"}}]`, "余额", "", true},
		{"cannot target user", `[{"type":"text","data":{"text":"/余额 999999"}}]`, "帮助", "", true},
		{"mention", `[{"type":"at","data":{"qq":"123456"}},{"type":"text","data":{"text":" 你好"}}]`, "聊天", "你好", true},
		{"numeric mention", `[{"type":"at","data":{"qq":123456}},{"type":"text","data":{"text":"/签到"}}]`, "签到", "", true},
		{"other mention", `[{"type":"at","data":{"qq":"999999"}},{"type":"text","data":{"text":"你好"}}]`, "", "", false},
		{"raw CQ", `"[CQ:at,qq=123456]你好"`, "", "", false},
		{"attachment", `[{"type":"image","data":{"file":"x"}},{"type":"text","data":{"text":"/聊天 hi"}}]`, "", "", false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			c, a, ok := parseCommand(groupEvent{SelfID: "123456", Message: json.RawMessage(tt.raw)})
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.command, c)
			require.Equal(t, tt.arg, a)
		})
	}
}
func TestQQBotCompletionUsesSiteGatewayAndBounds(t *testing.T) {
	s, r, h, c := testService(t)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls++
		require.Equal(t, "/v1/chat/completions", req.URL.Path)
		require.Equal(t, "Bearer site-secret", req.Header.Get("Authorization"))
		var b map[string]any
		require.NoError(t, json.NewDecoder(req.Body).Decode(&b))
		require.Equal(t, "site-model", b["model"])
		require.NotContains(t, b, "tools")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hello"}}]}`))
	}))
	defer server.Close()
	s.gateway = server.URL
	e := groupEvent{UserID: "777777", GroupID: "987654", SelfID: "123456"}
	c.HistoryMessages = 0
	reply, err := s.execute(context.Background(), c, e, "event", "聊天", "question")
	require.NoError(t, err)
	require.Equal(t, "hello", reply)
	require.Equal(t, 1, calls)
	require.Empty(t, r.stored)
	require.Zero(t, h.invalidated)
	r.quotaErr = ErrChatLimit
	_, err = s.execute(context.Background(), c, e, "event2", "聊天", "question")
	require.ErrorIs(t, err, ErrChatLimit)
	require.Equal(t, 1, calls)
}
func TestQQBotUpstreamErrorIsRedactedAndNotRetried(t *testing.T) {
	s, _, _, c := testService(t)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Error(w, "secret upstream account and key", 402)
	}))
	defer server.Close()
	s.gateway = server.URL
	_, err := s.completion(context.Background(), c, []Message{{Role: "user", Content: "hi"}})
	require.ErrorIs(t, err, ErrModelRequest)
	require.NotContains(t, publicError(err), "secret upstream")
	require.Equal(t, 1, calls)
	require.NotContains(t, publicError(errors.New("mysql password secret")), "password")
}
func TestQQBotCacheInvalidationOnlyAfterAward(t *testing.T) {
	s, r, h, c := testService(t)
	e := groupEvent{UserID: "777777", GroupID: "987654", SelfID: "123456"}
	r.checkinErr = ErrAlreadyCheckedIn
	reply, err := s.execute(context.Background(), c, e, "event", "签到", "")
	require.NoError(t, err)
	require.Contains(t, reply, "今天已签到")
	require.Contains(t, reply, "$1.00000000")
	require.Zero(t, h.invalidated)
	r.checkinErr = nil
	_, err = s.execute(context.Background(), c, e, "event2", "签到", "")
	require.NoError(t, err)
	require.Equal(t, 1, h.invalidated)
	_, err = s.execute(context.Background(), c, e, "event3", "渠道", "")
	require.NoError(t, err)
	require.EqualValues(t, 42, h.scope)
}
func dialBot(t *testing.T, s *Service) (*websocket.Conn, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(s.ServeOneBot))
	headers := http.Header{"Authorization": []string{"Bearer a-long-random-test-token-12345"}, "X-Self-ID": []string{"123456"}, "X-Client-Role": []string{"Universal"}}
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), headers)
	require.NoError(t, err)
	return conn, server
}
func TestQQBotOneBotRoundTripDedupAndRotation(t *testing.T) {
	s, r, _, _ := testService(t)
	conn, server := dialBot(t, s)
	defer server.Close()
	defer conn.Close()
	event := map[string]any{"post_type": "message", "message_type": "group", "sub_type": "normal", "self_id": 123456, "user_id": 777777, "group_id": 987654, "message_id": -123, "message": []any{map[string]any{"type": "text", "data": map[string]string{"text": "/帮助"}}}}
	require.NoError(t, conn.WriteJSON(event))
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var action struct {
		Action string `json:"action"`
		Echo   string `json:"echo"`
		Params struct {
			Message []map[string]any `json:"message"`
		} `json:"params"`
	}
	require.NoError(t, conn.ReadJSON(&action))
	require.Equal(t, "send_group_msg", action.Action)
	require.Equal(t, "at", action.Params.Message[0]["type"])
	require.NoError(t, conn.WriteJSON(map[string]any{"echo": action.Echo, "status": "ok", "retcode": 0}))
	select {
	case finished := <-r.finished:
		require.Equal(t, "success", finished.Status)
	case <-time.After(5 * time.Second):
		t.Fatal("event not recorded")
	}
	time.Sleep(1100 * time.Millisecond)
	require.NoError(t, conn.WriteJSON(event))
	event["group_id"] = 111111
	event["message_id"] = 456
	require.NoError(t, conn.WriteJSON(event))
	time.Sleep(100 * time.Millisecond)
	r.mu.Lock()
	require.Len(t, r.seen, 1)
	r.mu.Unlock()
	c, err := s.Config(context.Background())
	require.NoError(t, err)
	c.Enabled = false
	_, err = s.SaveConfig(context.Background(), c)
	require.NoError(t, err)
	_, _, err = conn.ReadMessage()
	require.Error(t, err)
}
func TestQQBotRejectUnauthenticatedConnection(t *testing.T) {
	s, _, _, _ := testService(t)
	server := httptest.NewServer(http.HandlerFunc(s.ServeOneBot))
	defer server.Close()
	for _, headers := range []http.Header{nil, {"Authorization": []string{"Bearer wrong"}, "X-Self-ID": []string{"123456"}, "X-Client-Role": []string{"Universal"}}, {"Authorization": []string{"Bearer a-long-random-test-token-12345"}, "X-Self-ID": []string{"654321"}, "X-Client-Role": []string{"Universal"}}} {
		conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), headers)
		require.Error(t, err)
		require.Nil(t, conn)
		require.Equal(t, 401, resp.StatusCode)
		resp.Body.Close()
	}
}

func TestQQBotRejectUnsafeConfiguration(t *testing.T) {
	_, _, _, valid := testService(t)
	for _, change := range []func(*Config){
		func(c *Config) { c.CheckinReward = -1 }, func(c *Config) { c.CheckinReward = 100.01 }, func(c *Config) { c.CheckinReward = 0.000000001 },
		func(c *Config) { c.CheckinRewardMax = -1 }, func(c *Config) { c.CheckinRewardMax = 100.01 }, func(c *Config) { c.CheckinRewardMax = 0.000000001 },
		func(c *Config) { c.CheckinReward = 0.1; c.CheckinRewardMax = 0.05 }, func(c *Config) { c.CheckinReward = 0; c.CheckinRewardMax = 0.1 },
		func(c *Config) { c.AllowedGroups = []string{"987654", "987654"} }, func(c *Config) { c.AllowedGroups = nil },
		func(c *Config) { c.AccessToken = "" }, func(c *Config) { c.APIKey = "" }, func(c *Config) { c.TotalDailyChatLimit = 0 }, func(c *Config) { c.MaxOutputTokens = 100000 },
	} {
		c := valid
		change(&c)
		require.Error(t, Validate(c))
	}
}

func TestQQBotRandomRewardRange(t *testing.T) {
	c := DefaultConfig()
	c.CheckinReward, c.CheckinRewardMax = 0.05, 0.1
	for i := 0; i < 100; i++ {
		amount, err := c.RewardAmount()
		require.NoError(t, err)
		require.GreaterOrEqual(t, amount, 0.05)
		require.LessOrEqual(t, amount, 0.1)
	}
	c.CheckinRewardMax = 0
	amount, err := c.RewardAmount()
	require.NoError(t, err)
	require.Equal(t, 0.05, amount)
}
