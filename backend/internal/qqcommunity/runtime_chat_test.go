package qqcommunity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestRuntimeConfigurationAuthenticationAndRetirement(t *testing.T) {
	s, mock, c, _ := observationFixture(t)
	s.encryptor = previewEncryptor{}
	c.AppSecret, c.APIKey = "onebot-test-token", "model-test-secret"
	c.RuntimeChat = DefaultRuntimeChatConfig()
	c.RuntimeChat.Enabled = true
	raw, err := json.Marshal(c.Public())
	require.NoError(t, err)
	for _, key := range []string{"\"autonomous\"", "\"history_messages\"", "\"meme_enabled\"", c.APIKey, c.AppSecret} {
		require.NotContains(t, string(raw), key)
	}
	var roundTrip Config
	require.NoError(t, json.Unmarshal(raw, &roundTrip))
	require.Zero(t, roundTrip.HistoryMessages)
	require.False(t, roundTrip.Autonomous.Enabled)
	for _, tc := range []struct {
		token, bot string
		code       int
	}{
		{"", c.AppID, 401}, {"wrong", c.AppID, 401}, {c.AppSecret, "other", 401}, {c.AppSecret, c.AppID, 200},
	} {
		expectObservationConfig(mock, c)
		req := httptest.NewRequest(http.MethodGet, "/runtime/config", nil)
		req.Header.Set("Authorization", "Bearer "+tc.token)
		req.Header.Set("X-Self-ID", tc.bot)
		w := httptest.NewRecorder()
		s.ServeRuntime(w, req)
		require.Equal(t, tc.code, w.Code)
		require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		if tc.code == 200 {
			require.Contains(t, w.Body.String(), c.APIKey)
		} else {
			require.NotContains(t, w.Body.String(), c.APIKey)
		}
	}
	revision := runtimeRevision(runtimePolicy(c))
	expectObservationConfig(mock, c)
	req := httptest.NewRequest(http.MethodPost, "/runtime/status", strings.NewReader(`{"revision":"`+revision+`","messages":150,"summaries":1,"sessions":1}`))
	req.Header.Set("Authorization", "Bearer "+c.AppSecret)
	req.Header.Set("X-Self-ID", c.AppID)
	w := httptest.NewRecorder()
	s.ServeRuntime(w, req)
	require.Equal(t, 200, w.Code)
	require.True(t, s.ChatRuntimeStatus(c).Connected)
	c.SystemPrompt += "changed"
	require.False(t, s.ChatRuntimeStatus(c).Connected)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRuntimeRejectsNativeModelEntrypoints(t *testing.T) {
	s, mock, c, _ := observationFixture(t)
	c.RuntimeChat.Enabled = true
	for _, kind := range []string{"chat", "vision", "tools"} {
		expectObservationConfig(mock, c)
		_, err := s.Probe(context.Background(), kind)
		require.ErrorContains(t, err, "已停用")
	}
	expectObservationConfig(mock, c)
	_, err := s.PreviewChat(context.Background(), ChatPreviewRequest{GroupID: "456", Question: "测试一下"})
	require.ErrorContains(t, err, "已停用")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRuntimeAcceptsExistingDSHPersona(t *testing.T) {
	c := DefaultConfig()
	c.RuntimeChat = DefaultRuntimeChatConfig()
	c.RuntimeChat.Enabled = true
	c.SystemPrompt = strings.Repeat("人设", 2200)
	require.NoError(t, validateConfig(c))
	c.RuntimeChat.Enabled = false
	require.Error(t, validateConfig(c))
}

func TestRuntimeMemberWelcomeRequiresFreshAllowedJoinAndOneBotReceipt(t *testing.T) {
	for _, name := range []string{"welcome", "duplicate", "expired", "disabled", "other-group", "self", "muted-notifications", "missing-receipt"} {
		t.Run(name, func(t *testing.T) {
			s, mock, c, _ := observationFixture(t)
			c.RuntimeChat = DefaultRuntimeChatConfig()
			c.RuntimeChat.Enabled = true
			c.RuntimeChat.WelcomeMessage = "欢迎 {name}，QQ {qq}，本群 {group}"
			at := time.Now().Add(-time.Second)
			e := Event{GroupID: "456", MemberID: "789"}
			e.Author.Username = "新朋友"
			j := Job{ID: "welcome-job", AppID: c.AppID, GroupID: "456", EventType: "GROUP_MEMBER_ADD", CreatedAt: time.Now(), TransientReply: true}
			count := int64(1)
			switch name {
			case "duplicate":
				count = 0
			case "expired":
				at = time.Now().Add(-11 * time.Minute)
			case "disabled":
				c.RuntimeChat.WelcomeEnabled = false
			case "other-group":
				j.GroupID = "999"
				e.GroupID = "999"
			case "self":
				e.MemberID = c.AppID
			case "muted-notifications":
				c.GroupPolicies = []GroupPolicy{{GroupID: "456", DisableNotifications: true}}
			}
			e.Timestamp, _ = json.Marshal(at.Unix())
			mock.ExpectExec("INSERT INTO qqc_members").WithArgs(c.AppID, e.GroupID, e.MemberID, time.Unix(at.Unix(), 0)).WillReturnResult(sqlmock.NewResult(0, count))
			var requests <-chan map[string]json.RawMessage
			if name == "welcome" || name == "missing-receipt" {
				ack := map[string]any{"message_id": 1234}
				if name == "missing-receipt" {
					ack = map[string]any{}
				}
				requests = bindingTestBot(t, s, c, ack)
			}
			err := s.memberEvent(context.Background(), c, j, e)
			if name == "missing-receipt" {
				require.ErrorContains(t, err, "message_id")
			} else {
				require.NoError(t, err)
			}
			if requests != nil {
				req := bindingTestRequest(t, requests)
				require.JSONEq(t, `"send_group_msg"`, string(req["action"]))
				require.Contains(t, string(req["params"]), `"qq":"789"`)
				require.Contains(t, string(req["params"]), "欢迎 新朋友")
				require.Empty(t, requests, "a welcome must not be retried")
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
