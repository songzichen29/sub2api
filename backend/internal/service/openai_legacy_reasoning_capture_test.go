package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIGatewayService_Forward_OAuthLegacyPreservesEncryptedReasoningToUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.0.0-test")
	c.Request.Header.Set("originator", "codex_cli_rs")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
			"x-request-id": []string{"rid_reasoning_capture"},
		},
		Body: io.NopCloser(strings.NewReader(`{"id":"resp_reasoning_capture","object":"response","created_at":1,"model":"gpt-5.4","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`)),
	}}

	svc := &OpenAIGatewayService{
		cfg:          &config.Config{},
		httpUpstream: upstream,
	}

	account := &Account{
		ID:          1957,
		Name:        "oauth-legacy-no-passthrough",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token-for-test",
			"expires_at":         time.Now().Add(time.Hour).Unix(),
			"chatgpt_account_id": "chatgpt-account-for-test",
		},
		Extra:  map[string]any{"openai_passthrough": false},
		Status: StatusActive,
	}

	validEC := "gAAAAAAAAAAACQoLDA0ODxAREhMUFRYXGBkaGxwdHh8gISIjJCUmJygpKissLS4vMDEyMzQ1Njc4OTo7PD0-P0BBQkNERUZHSA"
	body := []byte(`{"model":"gpt-5.4","stream":false,"instructions":"capture reasoning","input":[{"type":"message","role":"user","content":"hi"},{"type":"reasoning","id":"rs_capture_should_survive_with_ec","encrypted_content":"` + validEC + `","summary":[{"type":"summary_text","text":"keep me"}]},{"type":"reasoning","id":"rs_bare_should_drop","summary":[]},{"type":"function_call_output","call_id":"call_1","output":"{}"}]}`)

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, upstream.lastBody)

	captured := upstream.lastBody
	t.Logf("captured upstream body: %s", string(captured))
	require.Equal(t, "reasoning", gjson.GetBytes(captured, "input.1.type").String())
	require.Equal(t, validEC, gjson.GetBytes(captured, "input.1.encrypted_content").String())
	require.Equal(t, "keep me", gjson.GetBytes(captured, "input.1.summary.0.text").String())

	var reasoningCount int
	gjson.GetBytes(captured, "input").ForEach(func(_, item gjson.Result) bool {
		if item.Get("type").String() == "reasoning" {
			reasoningCount++
		}
		return true
	})
	require.Equal(t, 1, reasoningCount, "valid encrypted reasoning should survive; bare rs_* reasoning should be dropped")
	require.False(t, bytes.Contains(captured, []byte("rs_bare_should_drop")))
}
