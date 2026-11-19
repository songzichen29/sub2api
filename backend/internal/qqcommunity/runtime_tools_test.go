package qqcommunity

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	htmlpkg "golang.org/x/net/html"
)

func TestRuntimeToolsRejectWrongAuthScopeDisabledAndUnexpectedArguments(t *testing.T) {
	for _, tc := range []struct {
		name, body, token string
		code              int
	}{
		{"unauthenticated", `{}`, "", 401},
		{"other-group", `{"tool":"web_search","group_id":"999","member_id":"789","message_id":"42","query":"weather"}`, "secret", 400},
		{"model-target", `{"tool":"generate_image","group_id":"456","member_id":"789","message_id":"42","prompt":"猫","target":"999"}`, "secret", 400},
		{"invalid-id", `{"tool":"generate_image","group_id":"456","member_id":"789","message_id":"other","prompt":"猫"}`, "secret", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, mock, c, _ := observationFixture(t)
			s.encryptor = previewEncryptor{}
			c.AppSecret = "secret"
			c.RuntimeChat = DefaultRuntimeChatConfig()
			c.RuntimeChat.Enabled = true
			expectObservationConfig(mock, c)
			req := httptest.NewRequest(http.MethodPost, "/runtime/tools", strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer "+tc.token)
			req.Header.Set("X-Self-ID", c.AppID)
			w := httptest.NewRecorder()
			s.ServeRuntimeTools(w, req)
			require.Equal(t, tc.code, w.Code)
			require.NotContains(t, w.Body.String(), "secret")
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestRuntimeMemeToolUsesNewSwitchAndPreservesImageBytes(t *testing.T) {
	s, mock, c, _ := observationFixture(t)
	s.encryptor = previewEncryptor{}
	c.AppSecret = "secret"
	c.MemeEnabled = false
	c.RuntimeChat = DefaultRuntimeChatConfig()
	c.RuntimeChat.Enabled = true
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2))))
	s.memeSearch = func(context.Context, string) ([]memeCandidate, error) {
		return []memeCandidate{{URL: "https://img.soutula.com/test.png"}}, nil
	}
	s.imageDownload = func(context.Context, string) ([]byte, error) { return buf.Bytes(), nil }
	expectObservationConfig(mock, c)
	req := httptest.NewRequest(http.MethodPost, "/runtime/tools", strings.NewReader(`{"tool":"search_meme","group_id":"456","member_id":"789","message_id":"42","keyword":"开心"}`))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("X-Self-ID", c.AppID)
	w := httptest.NewRecorder()
	s.ServeRuntimeTools(w, req)
	require.Equal(t, 200, w.Code)
	var out map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	data, err := base64.StdEncoding.DecodeString(out["data"])
	require.NoError(t, err)
	require.Equal(t, buf.Bytes(), data)
	require.Equal(t, "image/png", out["mimeType"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRuntimeNaturalImageRequestKeepsOriginalIdentityAndUsesBusinessCommand(t *testing.T) {
	e, err := runtimeImageEvent(runtimeToolRequest{GroupID: "456", MemberID: "789", MessageID: "-42", Prompt: "疯狂水世界启动"})
	require.NoError(t, err)
	require.Equal(t, "-42", e.ID)
	require.Equal(t, "456", e.GroupID)
	require.Equal(t, "789", e.Author.MemberID)
	command, arg := command(e.Content)
	require.Equal(t, "生图", command)
	require.Equal(t, "疯狂水世界启动", arg)
	_, err = runtimeImageEvent(runtimeToolRequest{Prompt: " "})
	require.Error(t, err)
	c := DefaultConfig()
	require.NoError(t, json.Unmarshal([]byte(`{"runtime_chat":{"enabled":true}}`), &c))
	require.True(t, c.RuntimeChat.WebEnabled)
	require.True(t, c.RuntimeChat.MemesEnabled)
	require.True(t, c.RuntimeChat.CollectMemes)
}

func TestRuntimeWebParsesActualSourcesAndRejectsInternalURLs(t *testing.T) {
	rss := []byte(`<rss><channel><item><title>新闻标题</title><link>https://example.com/news</link><description>摘要</description><pubDate>2026-11-05</pubDate></item><item><title>internal</title><link>https://127.0.0.1/</link></item></channel></rss>`)
	sources := parseRuntimeSearch(rss, true)
	require.Len(t, sources, 1)
	require.Equal(t, "2026-11-05", sources[0].PublishedAt)
	html := []byte(`<table><tr><td><a class="result-link" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fnews">新闻标题</a></td></tr><tr><td class="result-snippet">正文摘要</td></tr></table>`)
	sources = parseRuntimeSearch(html, false)
	require.Len(t, sources, 1)
	require.Equal(t, "https://example.com/news", sources[0].URL)
	require.Equal(t, "正文摘要", sources[0].Snippet)
	for _, u := range []string{"http://example.com/", "https://127.0.0.1/", "https://[::1]/", "https://10.0.0.1/", "https://user:secret@example.com/", "https://example.com:8080/"} {
		_, err := runtimePublicURL(u)
		require.Error(t, err, u)
	}
	root, _ := htmlpkg.Parse(strings.NewReader(`<div>公开正文<script>steal secrets</script><span hidden>private</span></div>`))
	require.Equal(t, "公开正文", runtimeNodeText(root))
}

func TestRuntimePrivateImageEnqueuesOnceWithPrivateIdentity(t *testing.T) {
	s, mock, c, _ := observationFixture(t)
	c.RuntimeChat = DefaultRuntimeChatConfig()
	c.Enabled, c.ImageEnabled, c.RuntimeChat.Enabled, c.RuntimeChat.PrivateChatEnabled = true, true, true, true
	c.RuntimeChat.PrivateUsers = []string{"789"}
	e, err := runtimeImageEvent(runtimeToolRequest{Tool: "generate_image", MemberID: "789", MessageID: "42", Prompt: "窗边的猫"})
	require.NoError(t, err)
	require.Empty(t, e.GroupID)
	raw, err := json.Marshal(e)
	require.NoError(t, err)
	for _, inserted := range []int64{1, 0} {
		mock.ExpectBegin()
		mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectExec("INSERT INTO qqc_jobs.*ON CONFLICT.*DO NOTHING").WithArgs(sqlmock.AnyArg(), c.AppID, runtimePrivateImageCreate+":789:42", runtimePrivateImageCreate, "", "789", "42", string(raw), "image").WillReturnResult(sqlmock.NewResult(0, inserted))
		mock.ExpectCommit()
		require.NoError(t, s.Enqueue(context.Background(), c, Payload{Type: runtimePrivateImageCreate, Data: raw}))
	}
	c.RuntimeChat.PrivateChatEnabled = false
	require.ErrorIs(t, s.Enqueue(context.Background(), c, Payload{Type: runtimePrivateImageCreate, Data: raw}), ErrDisabled)
	require.NoError(t, mock.ExpectationsWereMet(), "private requests must not write group history or discovered groups")
}

func TestRuntimePrivateImageDeliveryUsesSenderAndRechecksPermission(t *testing.T) {
	s, mock, c, _ := observationFixture(t)
	c.RuntimeChat = DefaultRuntimeChatConfig()
	c.Enabled, c.ImageEnabled, c.RuntimeChat.Enabled, c.RuntimeChat.PrivateChatEnabled = true, true, true, true
	c.RuntimeChat.PrivateUsers = []string{"789"}
	j := Job{ID: "private-image", AppID: c.AppID, EventType: runtimePrivateImageCreate, MemberID: "789", MessageID: "42", PrivateReply: true, Kind: "image", CreatedAt: time.Now()}
	requests := bindingTestBot(t, s, c, map[string]any{"message_id": 1234})
	expectObservationConfig(mock, c)
	require.NoError(t, s.send(context.Background(), c, j, "图片已生成。", testInputPNG(t), 1))
	req := bindingTestRequest(t, requests)
	require.JSONEq(t, `"send_private_msg"`, string(req["action"]))
	var params struct {
		UserID  string  `json:"user_id"`
		GroupID *string `json:"group_id"`
		Message []struct {
			Type string `json:"type"`
		} `json:"message"`
	}
	require.NoError(t, json.Unmarshal(req["params"], &params))
	require.Equal(t, "789", params.UserID)
	require.Nil(t, params.GroupID)
	require.Equal(t, "image", params.Message[1].Type)
	c.RuntimeChat.PrivateChatEnabled = false
	expectObservationConfig(mock, c)
	require.ErrorIs(t, s.send(context.Background(), c, j, "图片已生成。", testInputPNG(t), 1), ErrDisabled)
	require.Empty(t, requests)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRuntimePrivateImageWorkerReservesPersonalQuotaAndDeliversImage(t *testing.T) {
	s, mock, c, _ := observationFixture(t)
	c.RuntimeChat = DefaultRuntimeChatConfig()
	c.Enabled, c.ImageEnabled, c.RuntimeChat.Enabled, c.RuntimeChat.PrivateChatEnabled = true, true, true, true
	c.RuntimeChat.PrivateUsers = []string{"789"}
	original := testInputPNG(t)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "/images/generations", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"b64_json": base64.StdEncoding.EncodeToString(original)}}})
	}))
	defer server.Close()
	c.ImageBaseURL = server.URL
	e, err := runtimeImageEvent(runtimeToolRequest{Tool: "generate_image", MemberID: "789", MessageID: "42", Prompt: "窗边的猫", OutputFormat: "png"})
	require.NoError(t, err)
	raw, err := json.Marshal(e)
	require.NoError(t, err)
	requests := bindingTestBot(t, s, c, map[string]any{"message_id": 1234})
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT .* FROM qqc_jobs q").WillReturnRows(sqlmock.NewRows(strings.Split(jobColumns, ",")).AddRow("private-job", c.AppID, runtimePrivateImageCreate, "", "789", "42", raw, "image", "queued", "", []byte{}, "", "", 0, 0, time.Now(), nil, "", "", []byte(`{}`), []byte(`{}`), nil))
	mock.ExpectExec("UPDATE qqc_jobs SET status='running'").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	expectObservationConfig(mock, c)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock_shared").WillReturnResult(sqlmock.NewResult(0, 1))
	expectObservationConfig(mock, c)
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("INSERT INTO qqc_daily_usage").WithArgs(c.AppID, "all", sqlmock.AnyArg(), "image", c.TotalDailyImageLimit).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("INSERT INTO qqc_daily_usage").WithArgs(c.AppID, "member:789", sqlmock.AnyArg(), "image", c.DailyImageLimit).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec("UPDATE qqc_jobs SET kind").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectExec("DELETE FROM qqc_pending_image_plans").WithArgs(c.AppID, "", "789").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE qqc_jobs SET image_decision").WillReturnResult(sqlmock.NewResult(0, 1))
	expectObservationConfig(mock, c)
	mock.ExpectExec("UPDATE qqc_jobs SET image_decision").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE qqc_jobs SET result_text").WillReturnResult(sqlmock.NewResult(0, 1))
	expectObservationConfig(mock, c)
	mock.ExpectExec("UPDATE qqc_jobs SET status").WithArgs("private-job", "completed", "").WillReturnResult(sqlmock.NewResult(0, 1))
	s.runOne(context.Background())
	require.NoError(t, mock.ExpectationsWereMet(), "no group quota, group history or binding mutation is allowed")
	require.Equal(t, 1, calls)
	for i := 0; i < 2; i++ {
		req := bindingTestRequest(t, requests)
		require.JSONEq(t, `"send_private_msg"`, string(req["action"]))
		require.Contains(t, string(req["params"]), `"user_id":"789"`)
		require.NotContains(t, string(req["params"]), `"group_id"`)
		if i == 1 {
			require.Contains(t, string(req["params"]), base64.StdEncoding.EncodeToString(original))
		}
	}
}
