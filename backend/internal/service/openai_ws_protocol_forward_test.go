package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type httpUpstreamSequenceRecorder struct {
	mu     sync.Mutex
	bodies [][]byte
	reqs   []*http.Request

	responses []*http.Response
	errs      []error
	callCount int
}

func (u *httpUpstreamSequenceRecorder) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	idx := u.callCount
	u.callCount++
	u.reqs = append(u.reqs, req)
	if req != nil && req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		u.bodies = append(u.bodies, b)
		_ = req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(b))
	} else {
		u.bodies = append(u.bodies, nil)
	}
	if idx < len(u.errs) && u.errs[idx] != nil {
		return nil, u.errs[idx]
	}
	if idx < len(u.responses) {
		return u.responses[idx], nil
	}
	if len(u.responses) == 0 {
		return nil, nil
	}
	return u.responses[len(u.responses)-1], nil
}

func (u *httpUpstreamSequenceRecorder) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func TestOpenAIGatewayService_Forward_PreservePreviousResponseIDWhenWSEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wsFallbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer wsFallbackServer.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "custom-client/1.0")

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"usage":{"input_tokens":1,"output_tokens":2,"input_tokens_details":{"cached_tokens":0}}}`,
			)),
		},
	}

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
	}

	account := &Account{
		ID:          1,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": wsFallbackServer.URL,
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	body := []byte(`{"model":"gpt-5.1","stream":false,"previous_response_id":"resp_123","input":[{"type":"input_text","text":"hello"}]}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.Error(t, err)
	require.Nil(t, result)
	require.Nil(t, upstream.lastReq, "WS 模式下失败时不应回退 HTTP")
}

func TestOpenAIGatewayService_Forward_HTTPIngressStaysHTTPWhenWSEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wsFallbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer wsFallbackServer.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "custom-client/1.0")
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"usage":{"input_tokens":1,"output_tokens":2,"input_tokens_details":{"cached_tokens":0}}}`,
			)),
		},
	}

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
	}

	account := &Account{
		ID:          101,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": wsFallbackServer.URL,
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	body := []byte(`{"model":"gpt-5.1","stream":false,"previous_response_id":"resp_http_keep","input":[{"type":"input_text","text":"hello"}]}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.OpenAIWSMode, "HTTP 入站应保持 HTTP 转发")
	require.NotNil(t, upstream.lastReq, "HTTP 入站应命中 HTTP 上游")
	require.False(t, gjson.GetBytes(upstream.lastBody, "previous_response_id").Exists(), "HTTP 路径应沿用原逻辑移除 previous_response_id")

	decision, _ := c.Get("openai_ws_transport_decision")
	reason, _ := c.Get("openai_ws_transport_reason")
	require.Equal(t, string(OpenAIUpstreamTransportHTTPSSE), decision)
	require.Equal(t, "client_protocol_http", reason)
}

func TestOpenAIGatewayService_Forward_HTTPIngressRetriesInvalidEncryptedContentOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wsFallbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer wsFallbackServer.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "custom-client/1.0")
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	upstream := &httpUpstreamSequenceRecorder{
		responses: []*http.Response{
			{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(
					`{"error":{"code":"invalid_encrypted_content","type":"invalid_request_error","message":"The encrypted content could not be verified."}}`,
				)),
			},
			{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(
					`{"id":"resp_http_retry_ok","usage":{"input_tokens":1,"output_tokens":2,"input_tokens_details":{"cached_tokens":0}}}`,
				)),
			},
		},
	}

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
	}

	account := &Account{
		ID:          102,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": wsFallbackServer.URL,
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	body := []byte(`{"model":"gpt-5.1","stream":false,"previous_response_id":"resp_http_retry","input":[{"type":"reasoning","encrypted_content":"gAAAAAAAAAAACQoLDA0ODxAREhMUFRYXGBkaGxwdHh8gISIjJCUmJygpKissLS4vMDEyMzQ1Njc4OTo7PD0-P0BBQkNERUZHSA","summary":[{"type":"summary_text","text":"keep me"}]},{"type":"input_text","text":"hello"}]}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.OpenAIWSMode, "HTTP 入站应保持 HTTP 转发")
	require.Equal(t, 2, upstream.callCount, "命中 invalid_encrypted_content 后应只在 HTTP 路径重试一次")
	require.Len(t, upstream.bodies, 2)

	firstBody := upstream.bodies[0]
	secondBody := upstream.bodies[1]
	require.False(t, gjson.GetBytes(firstBody, "previous_response_id").Exists(), "HTTP 首次请求仍应沿用原逻辑移除 previous_response_id")
	require.True(t, gjson.GetBytes(firstBody, "input.0.encrypted_content").Exists(), "首次请求不应做发送前预清理")
	require.Equal(t, "keep me", gjson.GetBytes(firstBody, "input.0.summary.0.text").String())

	require.False(t, gjson.GetBytes(secondBody, "previous_response_id").Exists(), "HTTP 精确重试不应重新带回 previous_response_id")
	require.False(t, gjson.GetBytes(secondBody, "input.0.encrypted_content").Exists(), "精确重试应移除 reasoning.encrypted_content")
	require.Equal(t, "keep me", gjson.GetBytes(secondBody, "input.0.summary.0.text").String(), "精确重试应保留有效 reasoning summary")
	require.Equal(t, "input_text", gjson.GetBytes(secondBody, "input.1.type").String(), "非 reasoning input 应保持原样")

	decision, _ := c.Get("openai_ws_transport_decision")
	reason, _ := c.Get("openai_ws_transport_reason")
	require.Equal(t, string(OpenAIUpstreamTransportHTTPSSE), decision)
	require.Equal(t, "client_protocol_http", reason)
}

func TestOpenAIGatewayService_Forward_HTTPIngressRetriesNestedInvalidEncryptedContentOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wsFallbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer wsFallbackServer.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "custom-client/1.0")
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	upstream := &httpUpstreamSequenceRecorder{
		responses: []*http.Response{
			{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(
					`{"error":{"code":"invalid_encrypted_content","type":"invalid_request_error","message":"The encrypted content could not be verified."}}`,
				)),
			},
			{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(
					`{"id":"resp_http_nested_retry_ok","usage":{"input_tokens":1,"output_tokens":2,"input_tokens_details":{"cached_tokens":0}}}`,
				)),
			},
		},
	}

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
	}

	account := &Account{
		ID:          104,
		Name:        "openai-apikey-nested",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": wsFallbackServer.URL,
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	body := []byte(`{"model":"gpt-5.1","stream":false,"previous_response_id":"resp_http_nested_retry","input":[{"type":"message","role":"assistant","content":[{"type":"reasoning","encrypted_content":"gAAAAAAAAAAACQoLDA0ODxAREhMUFRYXGBkaGxwdHh8gISIjJCUmJygpKissLS4vMDEyMzQ1Njc4OTo7PD0-P0BBQkNERUZHSA","summary":[{"type":"summary_text","text":"keep nested"}]},{"type":"output_text","text":"visible"}]},{"type":"input_text","text":"hello"}]}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.OpenAIWSMode, "HTTP 入站应保持 HTTP 转发")
	require.Equal(t, 2, upstream.callCount, "嵌套 invalid_encrypted_content 应触发一次 HTTP 清洗重试")
	require.Len(t, upstream.bodies, 2)

	firstBody := upstream.bodies[0]
	secondBody := upstream.bodies[1]
	require.True(t, gjson.GetBytes(firstBody, "input.0.content.0.encrypted_content").Exists(), "首次请求不应做发送前预清理")
	require.False(t, gjson.GetBytes(secondBody, "input.0.content.0.encrypted_content").Exists(), "重试应移除嵌套 reasoning.encrypted_content")
	require.Equal(t, "reasoning", gjson.GetBytes(secondBody, "input.0.content.0.type").String())
	require.Equal(t, "keep nested", gjson.GetBytes(secondBody, "input.0.content.0.summary.0.text").String(), "重试应保留嵌套 reasoning summary")
	require.Equal(t, "output_text", gjson.GetBytes(secondBody, "input.0.content.1.type").String(), "非 reasoning 内容应保持原样")
	require.Equal(t, "input_text", gjson.GetBytes(secondBody, "input.1.type").String())

	decision, _ := c.Get("openai_ws_transport_decision")
	reason, _ := c.Get("openai_ws_transport_reason")
	require.Equal(t, string(OpenAIUpstreamTransportHTTPSSE), decision)
	require.Equal(t, "client_protocol_http", reason)
}

func TestTrimOpenAIEncryptedReasoningItems_CleansMessagesContent(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.4-mini",
		"messages": []any{
			map[string]any{
				"role": "assistant",
				"content": []any{
					map[string]any{
						"type":              "reasoning",
						"encrypted_content": "gAAA",
						"summary": []any{
							map[string]any{"type": "summary_text", "text": "keep messages summary"},
						},
					},
					map[string]any{"type": "text", "text": "visible"},
				},
			},
		},
	}

	require.True(t, trimOpenAIEncryptedReasoningItems(reqBody))

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(body, "messages.0.content.0.encrypted_content").Exists())
	require.Equal(t, "reasoning", gjson.GetBytes(body, "messages.0.content.0.type").String())
	require.Equal(t, "keep messages summary", gjson.GetBytes(body, "messages.0.content.0.summary.0.text").String())
	require.Equal(t, "text", gjson.GetBytes(body, "messages.0.content.1.type").String())
	require.Equal(t, "visible", gjson.GetBytes(body, "messages.0.content.1.text").String())
}

func TestPresanitizeInvalidEncryptedReasoningItems_RemovesInvalidKeepsValid(t *testing.T) {
	validEC := "gAAAAAAAAAAACQoLDA0ODxAREhMUFRYXGBkaGxwdHh8gISIjJCUmJygpKissLS4vMDEyMzQ1Njc4OTo7PD0-P0BBQkNERUZHSA"
	reqBody := map[string]any{
		"model": "gpt-5.1",
		"input": []any{
			map[string]any{
				"type":              "reasoning",
				"encrypted_content": validEC,
				"summary": []any{
					map[string]any{"type": "summary_text", "text": "valid reasoning"},
				},
			},
			map[string]any{
				"type":              "reasoning",
				"encrypted_content": "gAAA",
				"summary": []any{
					map[string]any{"type": "summary_text", "text": "invalid reasoning"},
				},
			},
			map[string]any{"type": "input_text", "text": "hello"},
		},
	}

	require.True(t, presanitizeInvalidEncryptedReasoningItems(reqBody, "test-account"))

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)
	require.Equal(t, validEC, gjson.GetBytes(body, "input.0.encrypted_content").String(), "合法签名应保留")
	require.Equal(t, "valid reasoning", gjson.GetBytes(body, "input.0.summary.0.text").String())
	require.False(t, gjson.GetBytes(body, "input.1.encrypted_content").Exists(), "非法签名应被移除")
	require.Equal(t, "reasoning", gjson.GetBytes(body, "input.1.type").String(), "reasoning item 本身应保留")
	require.Equal(t, "invalid reasoning", gjson.GetBytes(body, "input.1.summary.0.text").String(), "summary 应保留")
	require.Equal(t, "input_text", gjson.GetBytes(body, "input.2.type").String())
}

func TestPresanitizeInvalidEncryptedReasoningItems_AllValidNoChange(t *testing.T) {
	validEC := "gAAAAAAAAAAACQoLDA0ODxAREhMUFRYXGBkaGxwdHh8gISIjJCUmJygpKissLS4vMDEyMzQ1Njc4OTo7PD0-P0BBQkNERUZHSA"
	reqBody := map[string]any{
		"model": "gpt-5.1",
		"input": []any{
			map[string]any{
				"type":              "reasoning",
				"encrypted_content": validEC,
			},
			map[string]any{"type": "input_text", "text": "hello"},
		},
	}

	require.False(t, presanitizeInvalidEncryptedReasoningItems(reqBody, "test-account"), "全部合法时不应有修改")

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)
	require.Equal(t, validEC, gjson.GetBytes(body, "input.0.encrypted_content").String())
}

func TestPresanitizeInvalidEncryptedReasoningItems_NonStringType(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.1",
		"input": []any{
			map[string]any{
				"type":              "reasoning",
				"encrypted_content": 12345,
			},
		},
	}

	require.True(t, presanitizeInvalidEncryptedReasoningItems(reqBody, "test-account"))

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(body, "input.0.encrypted_content").Exists(), "非字符串 encrypted_content 应被移除")
}

func TestPresanitizeInvalidEncryptedReasoningItems_NestedInMessages(t *testing.T) {
	validEC := "gAAAAAAAAAAACQoLDA0ODxAREhMUFRYXGBkaGxwdHh8gISIjJCUmJygpKissLS4vMDEyMzQ1Njc4OTo7PD0-P0BBQkNERUZHSA"
	reqBody := map[string]any{
		"model": "gpt-5.1",
		"messages": []any{
			map[string]any{
				"role": "assistant",
				"content": []any{
					map[string]any{
						"type":              "reasoning",
						"encrypted_content": "gAAA",
						"summary": []any{
							map[string]any{"type": "summary_text", "text": "invalid"},
						},
					},
					map[string]any{
						"type":              "reasoning",
						"encrypted_content": validEC,
					},
				},
			},
		},
	}

	require.True(t, presanitizeInvalidEncryptedReasoningItems(reqBody, "test-account"))

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(body, "messages.0.content.0.encrypted_content").Exists(), "嵌套非法签名应被移除")
	require.Equal(t, "reasoning", gjson.GetBytes(body, "messages.0.content.0.type").String(), "reasoning item 本身应保留")
	require.Equal(t, "invalid", gjson.GetBytes(body, "messages.0.content.0.summary.0.text").String(), "summary 应保留")
	require.Equal(t, validEC, gjson.GetBytes(body, "messages.0.content.1.encrypted_content").String(), "嵌套合法签名应保留")
}

func TestPresanitizeInvalidEncryptedReasoningItems_CompactionWithFunctionCallOutput(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.5",
		"input": []any{
			map[string]any{
				"type":              "compaction",
				"encrypted_content": "- **当前交接**：请继续完成回归点复测）",
			},
			map[string]any{
				"type":    "function_call_output",
				"call_id": "call_123",
				"output":  "ok",
			},
		},
	}

	require.True(t, presanitizeInvalidEncryptedReasoningItems(reqBody, "test-account"))

	body, err := json.Marshal(reqBody)
	require.NoError(t, err)
	inputArr := gjson.GetBytes(body, "input").Array()
	require.Len(t, inputArr, 1, "非法 compaction 只剩 type 一个字段，整个节点应被移除")
	require.Equal(t, "function_call_output", inputArr[0].Get("type").String())
	require.Equal(t, "call_123", inputArr[0].Get("call_id").String())
}

func TestOpenAIGatewayService_Forward_HTTPIngressRetriesWrappedInvalidEncryptedContentOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wsFallbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer wsFallbackServer.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "custom-client/1.0")
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	upstream := &httpUpstreamSequenceRecorder{
		responses: []*http.Response{
			{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(
					`{"error":{"code":null,"message":"{\"error\":{\"message\":\"The encrypted content could not be verified.\",\"type\":\"invalid_request_error\",\"param\":null,\"code\":\"invalid_encrypted_content\"}}（traceid: fb7ad1dbc7699c18f8a02f258f1af5ab）","param":null,"type":"invalid_request_error"}}`,
				)),
			},
			{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
					"x-request-id": []string{"req_http_retry_wrapped_ok"},
				},
				Body: io.NopCloser(strings.NewReader(
					`{"id":"resp_http_retry_wrapped_ok","usage":{"input_tokens":1,"output_tokens":2,"input_tokens_details":{"cached_tokens":0}}}`,
				)),
			},
		},
	}

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
	}

	account := &Account{
		ID:          103,
		Name:        "openai-apikey-wrapped",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": wsFallbackServer.URL,
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	body := []byte(`{"model":"gpt-5.1","stream":false,"previous_response_id":"resp_http_retry_wrapped","input":[{"type":"reasoning","encrypted_content":"gAAAAAAAAAAACQoLDA0ODxAREhMUFRYXGBkaGxwdHh8gISIjJCUmJygpKissLS4vMDEyMzQ1Njc4OTo7PD0-P0BBQkNERUZHSA","summary":[{"type":"summary_text","text":"keep me too"}]},{"type":"input_text","text":"hello"}]}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.OpenAIWSMode, "HTTP 入站应保持 HTTP 转发")
	require.Equal(t, 2, upstream.callCount, "wrapped invalid_encrypted_content 也应只在 HTTP 路径重试一次")
	require.Len(t, upstream.bodies, 2)

	firstBody := upstream.bodies[0]
	secondBody := upstream.bodies[1]
	require.True(t, gjson.GetBytes(firstBody, "input.0.encrypted_content").Exists(), "首次请求不应做发送前预清理")
	require.False(t, gjson.GetBytes(secondBody, "input.0.encrypted_content").Exists(), "wrapped exact retry 应移除 reasoning.encrypted_content")
	require.Equal(t, "keep me too", gjson.GetBytes(secondBody, "input.0.summary.0.text").String(), "wrapped exact retry 应保留有效 reasoning summary")

	decision, _ := c.Get("openai_ws_transport_decision")
	reason, _ := c.Get("openai_ws_transport_reason")
	require.Equal(t, string(OpenAIUpstreamTransportHTTPSSE), decision)
	require.Equal(t, "client_protocol_http", reason)
}

func TestOpenAIGatewayService_Forward_HTTPIngressRetriesInvalidEncryptedContentWithoutOAuthSessionAnchor(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "custom-client/1.0")
	c.Request.Header.Set("session_id", "client-session")
	c.Request.Header.Set("conversation_id", "client-conversation")
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	upstream := &httpUpstreamSequenceRecorder{
		responses: []*http.Response{
			{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(
					`{"error":{"code":"invalid_encrypted_content","type":"invalid_request_error","message":"The encrypted content could not be verified."}}`,
				)),
			},
			{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(
					`{"id":"resp_http_session_anchor_retry_ok","usage":{"input_tokens":1,"output_tokens":2,"input_tokens_details":{"cached_tokens":0}}}`,
				)),
			},
		},
	}

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
	}

	account := &Account{
		ID:          105,
		Name:        "openai-oauth-session-anchor",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "oauth-token",
		},
	}

	body := []byte(`{"model":"gpt-5.4-mini","stream":false,"prompt_cache_key":"cache-session","input":[{"type":"input_text","text":"hello"}]}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.OpenAIWSMode, "HTTP 入站应保持 HTTP 转发")
	require.Equal(t, 2, upstream.callCount, "缺少 encrypted_content 时 OAuth 会话锚点兜底应只重试一次")
	require.Len(t, upstream.bodies, 2)
	require.Len(t, upstream.reqs, 2)

	firstBody := upstream.bodies[0]
	secondBody := upstream.bodies[1]
	require.Equal(t, "cache-session", gjson.GetBytes(firstBody, "prompt_cache_key").String(), "首次请求应保留原始 prompt_cache_key")
	require.False(t, gjson.GetBytes(secondBody, "prompt_cache_key").Exists(), "会话锚点兜底重试应移除 prompt_cache_key")

	firstSessionID := upstream.reqs[0].Header.Get("session_id")
	firstConversationID := upstream.reqs[0].Header.Get("conversation_id")
	require.NotEmpty(t, firstSessionID, "首次请求应带隔离后的 session_id")
	require.NotEqual(t, "client-session", firstSessionID, "OAuth 上游 session_id 应隔离")
	require.NotEmpty(t, firstConversationID, "首次请求应带隔离后的 conversation_id")
	require.Empty(t, upstream.reqs[1].Header.Get("session_id"), "重试请求不应继续携带 session_id")
	require.Empty(t, upstream.reqs[1].Header.Get("conversation_id"), "重试请求不应继续携带 conversation_id")

	decision, _ := c.Get("openai_ws_transport_decision")
	reason, _ := c.Get("openai_ws_transport_reason")
	require.Equal(t, string(OpenAIUpstreamTransportHTTPSSE), decision)
	require.Equal(t, "client_protocol_http", reason)
}

func TestOpenAIGatewayService_Forward_RemovePreviousResponseIDWhenWSDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wsFallbackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer wsFallbackServer.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "custom-client/1.0")

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"usage":{"input_tokens":1,"output_tokens":2,"input_tokens_details":{"cached_tokens":0}}}`,
			)),
		},
	}

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = false
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
	}

	account := &Account{
		ID:          1,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": wsFallbackServer.URL,
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	body := []byte(`{"model":"gpt-5.1","stream":false,"previous_response_id":"resp_123","input":[{"type":"input_text","text":"hello"}]}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, gjson.GetBytes(upstream.lastBody, "previous_response_id").Exists())
}

func TestOpenAIGatewayService_Forward_WSv2Dial426FallbackHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ws426Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUpgradeRequired)
		_, _ = w.Write([]byte(`upgrade required`))
	}))
	defer ws426Server.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "custom-client/1.0")

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"usage":{"input_tokens":8,"output_tokens":9,"input_tokens_details":{"cached_tokens":1}}}`,
			)),
		},
	}

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.FallbackCooldownSeconds = 1

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
	}

	account := &Account{
		ID:          12,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": ws426Server.URL,
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	body := []byte(`{"model":"gpt-5.1","stream":false,"previous_response_id":"resp_426","input":[{"type":"input_text","text":"hello"}]}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "upgrade_required")
	require.Nil(t, upstream.lastReq, "WS 模式下不应再回退 HTTP")
	require.Equal(t, http.StatusUpgradeRequired, rec.Code)
	require.Contains(t, rec.Body.String(), "426")
}

func TestOpenAIGatewayService_Forward_WSv2FallbackCoolingSkipWS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer wsServer.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "custom-client/1.0")

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"usage":{"input_tokens":2,"output_tokens":3,"input_tokens_details":{"cached_tokens":0}}}`,
			)),
		},
	}

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.FallbackCooldownSeconds = 30

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
	}

	account := &Account{
		ID:          21,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": wsServer.URL,
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	svc.markOpenAIWSFallbackCooling(account.ID, "upgrade_required")
	body := []byte(`{"model":"gpt-5.1","stream":false,"previous_response_id":"resp_cooling","input":[{"type":"input_text","text":"hello"}]}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.Error(t, err)
	require.Nil(t, result)
	require.Nil(t, upstream.lastReq, "WS 模式下不应再回退 HTTP")

	_, ok := c.Get("openai_ws_fallback_cooling")
	require.False(t, ok, "已移除 fallback cooling 快捷回退路径")
}

func TestOpenAIGatewayService_Forward_ReturnErrorWhenOnlyWSv1Enabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "custom-client/1.0")

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"usage":{"input_tokens":1,"output_tokens":2,"input_tokens_details":{"cached_tokens":0}}}`,
			)),
		},
	}

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsockets = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = false

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
	}

	account := &Account{
		ID:          31,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://api.openai.com/v1/responses",
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	body := []byte(`{"model":"gpt-5.1","stream":false,"previous_response_id":"resp_v1","input":[{"type":"input_text","text":"hello"}]}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "ws v1")
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "WSv1")
	require.Nil(t, upstream.lastReq, "WSv1 不支持时不应触发 HTTP 上游请求")
}

func TestNewOpenAIGatewayService_InitializesOpenAIWSResolver(t *testing.T) {
	cfg := &config.Config{}
	svc := NewOpenAIGatewayService(
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		cfg,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	decision := svc.getOpenAIWSProtocolResolver().Resolve(nil)
	require.Equal(t, OpenAIUpstreamTransportHTTPSSE, decision.Transport)
	require.Equal(t, "account_missing", decision.Reason)
}

func TestOpenAIGatewayService_Forward_WSv2FallbackWhenResponseAlreadyWrittenReturnsWSError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ws426Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUpgradeRequired)
		_, _ = w.Write([]byte(`upgrade required`))
	}))
	defer ws426Server.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "custom-client/1.0")
	c.String(http.StatusAccepted, "already-written")

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"usage":{"input_tokens":1,"output_tokens":1}}`)),
		},
	}

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.FallbackCooldownSeconds = 1

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
	}

	account := &Account{
		ID:          41,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": ws426Server.URL,
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	body := []byte(`{"model":"gpt-5.1","stream":false,"input":[{"type":"input_text","text":"hello"}]}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "ws fallback")
	require.Nil(t, upstream.lastReq, "已写下游响应时，不应再回退 HTTP")
}

func TestOpenAIGatewayService_Forward_WSv2StreamEarlyCloseFallbackHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket failed: %v", err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()

		var req map[string]any
		if err := conn.ReadJSON(&req); err != nil {
			t.Errorf("read ws request failed: %v", err)
			return
		}

		// 仅发送 response.created（非 token 事件）后立即关闭，
		// 模拟线上“上游早期内部错误断连”的场景。
		if err := conn.WriteJSON(map[string]any{
			"type": "response.created",
			"response": map[string]any{
				"id":    "resp_ws_created_only",
				"model": "gpt-5.3-codex",
			},
		}); err != nil {
			t.Errorf("write response.created failed: %v", err)
			return
		}
		closePayload := websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "")
		_ = conn.WriteControl(websocket.CloseMessage, closePayload, time.Now().Add(time.Second))
	}))
	defer wsServer.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "custom-client/1.0")

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body: io.NopCloser(strings.NewReader(
				"data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n" +
					"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_http_fallback\",\"usage\":{\"input_tokens\":2,\"output_tokens\":1}}}\n\n" +
					"data: [DONE]\n\n",
			)),
		},
	}

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.FallbackCooldownSeconds = 1

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
	}

	account := &Account{
		ID:          88,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": wsServer.URL,
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	body := []byte(`{"model":"gpt-5.3-codex","stream":true,"input":[{"type":"input_text","text":"hello"}]}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.Error(t, err)
	require.Nil(t, result)
	require.Nil(t, upstream.lastReq, "WS 早期断连后不应再回退 HTTP")
	require.Empty(t, rec.Body.String(), "未产出 token 前上游断连时不应写入下游半截流")
}

func TestOpenAIGatewayService_Forward_WSv2RetryFiveTimesThenFallbackHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var wsAttempts atomic.Int32
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wsAttempts.Add(1)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket failed: %v", err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()

		var req map[string]any
		if err := conn.ReadJSON(&req); err != nil {
			t.Errorf("read ws request failed: %v", err)
			return
		}
		closePayload := websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "")
		_ = conn.WriteControl(websocket.CloseMessage, closePayload, time.Now().Add(time.Second))
	}))
	defer wsServer.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "custom-client/1.0")

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body: io.NopCloser(strings.NewReader(
				"data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n" +
					"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_retry_http_fallback\",\"usage\":{\"input_tokens\":2,\"output_tokens\":1}}}\n\n" +
					"data: [DONE]\n\n",
			)),
		},
	}

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.FallbackCooldownSeconds = 1

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
	}

	account := &Account{
		ID:          89,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": wsServer.URL,
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	body := []byte(`{"model":"gpt-5.3-codex","stream":true,"input":[{"type":"input_text","text":"hello"}]}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.Error(t, err)
	require.Nil(t, result)
	require.Nil(t, upstream.lastReq, "WS 重连耗尽后不应再回退 HTTP")
	require.Equal(t, int32(openAIWSReconnectRetryLimit+1), wsAttempts.Load())
}

func TestOpenAIGatewayService_Forward_WSv2PolicyViolationFastFallbackHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var wsAttempts atomic.Int32
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wsAttempts.Add(1)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket failed: %v", err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()

		var req map[string]any
		if err := conn.ReadJSON(&req); err != nil {
			t.Errorf("read ws request failed: %v", err)
			return
		}
		closePayload := websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "")
		_ = conn.WriteControl(websocket.CloseMessage, closePayload, time.Now().Add(time.Second))
	}))
	defer wsServer.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "custom-client/1.0")

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_policy_fallback","usage":{"input_tokens":1,"output_tokens":1}}`)),
		},
	}

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.FallbackCooldownSeconds = 1
	cfg.Gateway.OpenAIWS.RetryBackoffInitialMS = 1
	cfg.Gateway.OpenAIWS.RetryBackoffMaxMS = 2
	cfg.Gateway.OpenAIWS.RetryJitterRatio = 0

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
	}

	account := &Account{
		ID:          8901,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": wsServer.URL,
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	body := []byte(`{"model":"gpt-5.3-codex","stream":false,"input":[{"type":"input_text","text":"hello"}]}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.Error(t, err)
	require.Nil(t, result)
	require.Nil(t, upstream.lastReq, "策略违规关闭后不应回退 HTTP")
	require.Equal(t, int32(1), wsAttempts.Load(), "策略违规不应进行 WS 重试")
}

func TestOpenAIGatewayService_Forward_WSv2ConnectionLimitReachedRetryThenFallbackHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var wsAttempts atomic.Int32
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wsAttempts.Add(1)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket failed: %v", err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()

		var req map[string]any
		if err := conn.ReadJSON(&req); err != nil {
			t.Errorf("read ws request failed: %v", err)
			return
		}
		_ = conn.WriteJSON(map[string]any{
			"type": "error",
			"error": map[string]any{
				"code":    "websocket_connection_limit_reached",
				"type":    "server_error",
				"message": "websocket connection limit reached",
			},
		})
	}))
	defer wsServer.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "custom-client/1.0")

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_http_retry_limit","usage":{"input_tokens":1,"output_tokens":1}}`)),
		},
	}

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.FallbackCooldownSeconds = 1

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
	}

	account := &Account{
		ID:          90,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": wsServer.URL,
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	body := []byte(`{"model":"gpt-5.3-codex","stream":false,"input":[{"type":"input_text","text":"hello"}]}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.Error(t, err)
	require.Nil(t, result)
	require.Nil(t, upstream.lastReq, "触发 websocket_connection_limit_reached 后不应回退 HTTP")
	require.Equal(t, int32(openAIWSReconnectRetryLimit+1), wsAttempts.Load())
}

func TestOpenAIGatewayService_Forward_WSv2PreviousResponseNotFoundRecoversByDroppingPreviousResponseID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var wsAttempts atomic.Int32
	var wsRequestPayloads [][]byte
	var wsRequestMu sync.Mutex
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := wsAttempts.Add(1)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket failed: %v", err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()

		var req map[string]any
		if err := conn.ReadJSON(&req); err != nil {
			t.Errorf("read ws request failed: %v", err)
			return
		}
		reqRaw, _ := json.Marshal(req)
		wsRequestMu.Lock()
		wsRequestPayloads = append(wsRequestPayloads, reqRaw)
		wsRequestMu.Unlock()
		if attempt == 1 {
			_ = conn.WriteJSON(map[string]any{
				"type": "error",
				"error": map[string]any{
					"code":    "previous_response_not_found",
					"type":    "invalid_request_error",
					"message": "previous response not found",
				},
			})
			return
		}
		_ = conn.WriteJSON(map[string]any{
			"type": "response.completed",
			"response": map[string]any{
				"id":    "resp_ws_prev_recover_ok",
				"model": "gpt-5.3-codex",
				"usage": map[string]any{
					"input_tokens":  1,
					"output_tokens": 1,
					"input_tokens_details": map[string]any{
						"cached_tokens": 0,
					},
				},
			},
		})
	}))
	defer wsServer.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "custom-client/1.0")

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_http_drop_prev","usage":{"input_tokens":1,"output_tokens":1}}`)),
		},
	}

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.FallbackCooldownSeconds = 1

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
	}

	account := &Account{
		ID:          91,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": wsServer.URL,
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	body := []byte(`{"model":"gpt-5.3-codex","stream":false,"previous_response_id":"resp_prev_missing","input":[{"type":"input_text","text":"hello"}]}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "resp_ws_prev_recover_ok", result.RequestID)
	require.Nil(t, upstream.lastReq, "previous_response_not_found 不应回退 HTTP")
	require.Equal(t, int32(2), wsAttempts.Load(), "previous_response_not_found 应触发一次去掉 previous_response_id 的恢复重试")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "resp_ws_prev_recover_ok", gjson.Get(rec.Body.String(), "id").String())

	wsRequestMu.Lock()
	requests := append([][]byte(nil), wsRequestPayloads...)
	wsRequestMu.Unlock()
	require.Len(t, requests, 2)
	require.True(t, gjson.GetBytes(requests[0], "previous_response_id").Exists(), "首轮请求应保留 previous_response_id")
	require.False(t, gjson.GetBytes(requests[1], "previous_response_id").Exists(), "恢复重试应移除 previous_response_id")
}

func TestOpenAIGatewayService_Forward_WSv2PreviousResponseNotFoundSkipsRecoveryForFunctionCallOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var wsAttempts atomic.Int32
	var wsRequestPayloads [][]byte
	var wsRequestMu sync.Mutex
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wsAttempts.Add(1)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket failed: %v", err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()

		var req map[string]any
		if err := conn.ReadJSON(&req); err != nil {
			t.Errorf("read ws request failed: %v", err)
			return
		}
		reqRaw, _ := json.Marshal(req)
		wsRequestMu.Lock()
		wsRequestPayloads = append(wsRequestPayloads, reqRaw)
		wsRequestMu.Unlock()
		_ = conn.WriteJSON(map[string]any{
			"type": "error",
			"error": map[string]any{
				"code":    "previous_response_not_found",
				"type":    "invalid_request_error",
				"message": "previous response not found",
			},
		})
	}))
	defer wsServer.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "custom-client/1.0")

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_http_drop_prev","usage":{"input_tokens":1,"output_tokens":1}}`)),
		},
	}

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.FallbackCooldownSeconds = 1

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
	}

	account := &Account{
		ID:          92,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": wsServer.URL,
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	body := []byte(`{"model":"gpt-5.3-codex","stream":false,"previous_response_id":"resp_prev_missing","input":[{"type":"function_call_output","call_id":"call_1","output":"ok"}]}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.Error(t, err)
	require.Nil(t, result)
	require.Nil(t, upstream.lastReq, "previous_response_not_found 不应回退 HTTP")
	require.Equal(t, int32(1), wsAttempts.Load(), "function_call_output 场景应跳过 previous_response_not_found 自动恢复")
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, strings.ToLower(rec.Body.String()), "previous response not found")

	wsRequestMu.Lock()
	requests := append([][]byte(nil), wsRequestPayloads...)
	wsRequestMu.Unlock()
	require.Len(t, requests, 1)
	require.True(t, gjson.GetBytes(requests[0], "previous_response_id").Exists())
}

func TestOpenAIGatewayService_Forward_WSv2PreviousResponseNotFoundSkipsRecoveryWithoutPreviousResponseID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var wsAttempts atomic.Int32
	var wsRequestPayloads [][]byte
	var wsRequestMu sync.Mutex
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wsAttempts.Add(1)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket failed: %v", err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()

		var req map[string]any
		if err := conn.ReadJSON(&req); err != nil {
			t.Errorf("read ws request failed: %v", err)
			return
		}
		reqRaw, _ := json.Marshal(req)
		wsRequestMu.Lock()
		wsRequestPayloads = append(wsRequestPayloads, reqRaw)
		wsRequestMu.Unlock()
		_ = conn.WriteJSON(map[string]any{
			"type": "error",
			"error": map[string]any{
				"code":    "previous_response_not_found",
				"type":    "invalid_request_error",
				"message": "previous response not found",
			},
		})
	}))
	defer wsServer.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "custom-client/1.0")

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_http_drop_prev","usage":{"input_tokens":1,"output_tokens":1}}`)),
		},
	}

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.FallbackCooldownSeconds = 1

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
	}

	account := &Account{
		ID:          93,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": wsServer.URL,
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	body := []byte(`{"model":"gpt-5.3-codex","stream":false,"input":[{"type":"input_text","text":"hello"}]}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.Error(t, err)
	require.Nil(t, result)
	require.Nil(t, upstream.lastReq, "WS 模式下 previous_response_not_found 不应回退 HTTP")
	require.Equal(t, int32(1), wsAttempts.Load(), "缺少 previous_response_id 时应跳过自动恢复重试")
	require.Equal(t, http.StatusBadRequest, rec.Code)

	wsRequestMu.Lock()
	requests := append([][]byte(nil), wsRequestPayloads...)
	wsRequestMu.Unlock()
	require.Len(t, requests, 1)
	require.False(t, gjson.GetBytes(requests[0], "previous_response_id").Exists())
}

func TestOpenAIGatewayService_Forward_WSv2PreviousResponseNotFoundOnlyRecoversOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var wsAttempts atomic.Int32
	var wsRequestPayloads [][]byte
	var wsRequestMu sync.Mutex
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wsAttempts.Add(1)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket failed: %v", err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()

		var req map[string]any
		if err := conn.ReadJSON(&req); err != nil {
			t.Errorf("read ws request failed: %v", err)
			return
		}
		reqRaw, _ := json.Marshal(req)
		wsRequestMu.Lock()
		wsRequestPayloads = append(wsRequestPayloads, reqRaw)
		wsRequestMu.Unlock()
		_ = conn.WriteJSON(map[string]any{
			"type": "error",
			"error": map[string]any{
				"code":    "previous_response_not_found",
				"type":    "invalid_request_error",
				"message": "previous response not found",
			},
		})
	}))
	defer wsServer.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "custom-client/1.0")

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_http_drop_prev","usage":{"input_tokens":1,"output_tokens":1}}`)),
		},
	}

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.FallbackCooldownSeconds = 1

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
	}

	account := &Account{
		ID:          94,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": wsServer.URL,
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	body := []byte(`{"model":"gpt-5.3-codex","stream":false,"previous_response_id":"resp_prev_missing","input":[{"type":"input_text","text":"hello"}]}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.Error(t, err)
	require.Nil(t, result)
	require.Nil(t, upstream.lastReq, "WS 模式下 previous_response_not_found 不应回退 HTTP")
	require.Equal(t, int32(2), wsAttempts.Load(), "应只允许一次自动恢复重试")
	require.Equal(t, http.StatusBadRequest, rec.Code)

	wsRequestMu.Lock()
	requests := append([][]byte(nil), wsRequestPayloads...)
	wsRequestMu.Unlock()
	require.Len(t, requests, 2)
	require.True(t, gjson.GetBytes(requests[0], "previous_response_id").Exists(), "首轮请求应包含 previous_response_id")
	require.False(t, gjson.GetBytes(requests[1], "previous_response_id").Exists(), "恢复重试应移除 previous_response_id")
}

func TestOpenAIGatewayService_Forward_WSv2InvalidEncryptedContentRecoversOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var wsAttempts atomic.Int32
	var wsRequestPayloads [][]byte
	var wsRequestMu sync.Mutex
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := wsAttempts.Add(1)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket failed: %v", err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()

		var req map[string]any
		if err := conn.ReadJSON(&req); err != nil {
			t.Errorf("read ws request failed: %v", err)
			return
		}
		reqRaw, _ := json.Marshal(req)
		wsRequestMu.Lock()
		wsRequestPayloads = append(wsRequestPayloads, reqRaw)
		wsRequestMu.Unlock()
		if attempt == 1 {
			_ = conn.WriteJSON(map[string]any{
				"type": "error",
				"error": map[string]any{
					"code":    "invalid_encrypted_content",
					"type":    "invalid_request_error",
					"message": "The encrypted content could not be verified.",
				},
			})
			return
		}
		_ = conn.WriteJSON(map[string]any{
			"type": "response.completed",
			"response": map[string]any{
				"id":    "resp_ws_invalid_encrypted_content_recover_ok",
				"model": "gpt-5.3-codex",
				"usage": map[string]any{
					"input_tokens":  1,
					"output_tokens": 1,
					"input_tokens_details": map[string]any{
						"cached_tokens": 0,
					},
				},
			},
		})
	}))
	defer wsServer.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "custom-client/1.0")

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_http_drop_reasoning","usage":{"input_tokens":1,"output_tokens":1}}`)),
		},
	}

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.FallbackCooldownSeconds = 1

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
	}

	account := &Account{
		ID:          95,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": wsServer.URL,
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	body := []byte(`{"model":"gpt-5.3-codex","stream":false,"previous_response_id":"resp_prev_encrypted","input":[{"type":"reasoning","encrypted_content":"gAAAAAAAAAAACQoLDA0ODxAREhMUFRYXGBkaGxwdHh8gISIjJCUmJygpKissLS4vMDEyMzQ1Njc4OTo7PD0-P0BBQkNERUZHSA"},{"type":"input_text","text":"hello"}]}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "resp_ws_invalid_encrypted_content_recover_ok", result.RequestID)
	require.Nil(t, upstream.lastReq, "invalid_encrypted_content 不应回退 HTTP")
	require.Equal(t, int32(2), wsAttempts.Load(), "invalid_encrypted_content 应触发一次清洗后重试")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "resp_ws_invalid_encrypted_content_recover_ok", gjson.Get(rec.Body.String(), "id").String())

	wsRequestMu.Lock()
	requests := append([][]byte(nil), wsRequestPayloads...)
	wsRequestMu.Unlock()
	require.Len(t, requests, 2)
	require.True(t, gjson.GetBytes(requests[0], "previous_response_id").Exists(), "首轮请求应保留 previous_response_id")
	require.True(t, gjson.GetBytes(requests[0], `input.0.encrypted_content`).Exists(), "首轮请求应保留 encrypted reasoning")
	require.False(t, gjson.GetBytes(requests[1], "previous_response_id").Exists(), "恢复重试应移除 previous_response_id")
	require.False(t, gjson.GetBytes(requests[1], `input.0.encrypted_content`).Exists(), "恢复重试应移除 encrypted reasoning item")
	require.Equal(t, "input_text", gjson.GetBytes(requests[1], `input.0.type`).String())
}

func TestOpenAIGatewayService_Forward_WSv2InvalidEncryptedContentSkipsRecoveryWithoutReasoningItem(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var wsAttempts atomic.Int32
	var wsRequestPayloads [][]byte
	var wsRequestMu sync.Mutex
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wsAttempts.Add(1)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket failed: %v", err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()

		var req map[string]any
		if err := conn.ReadJSON(&req); err != nil {
			t.Errorf("read ws request failed: %v", err)
			return
		}
		reqRaw, _ := json.Marshal(req)
		wsRequestMu.Lock()
		wsRequestPayloads = append(wsRequestPayloads, reqRaw)
		wsRequestMu.Unlock()
		_ = conn.WriteJSON(map[string]any{
			"type": "error",
			"error": map[string]any{
				"code":    "invalid_encrypted_content",
				"type":    "invalid_request_error",
				"message": "The encrypted content could not be verified.",
			},
		})
	}))
	defer wsServer.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "custom-client/1.0")

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_http_drop_reasoning","usage":{"input_tokens":1,"output_tokens":1}}`)),
		},
	}

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.FallbackCooldownSeconds = 1

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
	}

	account := &Account{
		ID:          96,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": wsServer.URL,
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	body := []byte(`{"model":"gpt-5.3-codex","stream":false,"previous_response_id":"resp_prev_encrypted","input":[{"type":"input_text","text":"hello"}]}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.Error(t, err)
	require.Nil(t, result)
	require.Nil(t, upstream.lastReq, "invalid_encrypted_content 不应回退 HTTP")
	require.Equal(t, int32(1), wsAttempts.Load(), "缺少 reasoning encrypted item 时应跳过自动恢复重试")
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, strings.ToLower(rec.Body.String()), "encrypted content")

	wsRequestMu.Lock()
	requests := append([][]byte(nil), wsRequestPayloads...)
	wsRequestMu.Unlock()
	require.Len(t, requests, 1)
	require.True(t, gjson.GetBytes(requests[0], "previous_response_id").Exists())
	require.False(t, gjson.GetBytes(requests[0], `input.0.encrypted_content`).Exists())
}

func TestOpenAIGatewayService_Forward_WSv2InvalidEncryptedContentRecoversSingleObjectInputAndKeepsSummary(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var wsAttempts atomic.Int32
	var wsRequestPayloads [][]byte
	var wsRequestMu sync.Mutex
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := wsAttempts.Add(1)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket failed: %v", err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()

		var req map[string]any
		if err := conn.ReadJSON(&req); err != nil {
			t.Errorf("read ws request failed: %v", err)
			return
		}
		reqRaw, _ := json.Marshal(req)
		wsRequestMu.Lock()
		wsRequestPayloads = append(wsRequestPayloads, reqRaw)
		wsRequestMu.Unlock()
		if attempt == 1 {
			_ = conn.WriteJSON(map[string]any{
				"type": "error",
				"error": map[string]any{
					"code":    "invalid_encrypted_content",
					"type":    "invalid_request_error",
					"message": "The encrypted content could not be verified.",
				},
			})
			return
		}
		_ = conn.WriteJSON(map[string]any{
			"type": "response.completed",
			"response": map[string]any{
				"id":    "resp_ws_invalid_encrypted_content_object_ok",
				"model": "gpt-5.3-codex",
				"usage": map[string]any{
					"input_tokens":  1,
					"output_tokens": 1,
					"input_tokens_details": map[string]any{
						"cached_tokens": 0,
					},
				},
			},
		})
	}))
	defer wsServer.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "custom-client/1.0")

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_http_drop_reasoning","usage":{"input_tokens":1,"output_tokens":1}}`)),
		},
	}

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.FallbackCooldownSeconds = 1

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
	}

	account := &Account{
		ID:          97,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": wsServer.URL,
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	body := []byte(`{"model":"gpt-5.3-codex","stream":false,"previous_response_id":"resp_prev_encrypted","input":{"type":"reasoning","encrypted_content":"gAAAAAAAAAAACQoLDA0ODxAREhMUFRYXGBkaGxwdHh8gISIjJCUmJygpKissLS4vMDEyMzQ1Njc4OTo7PD0-P0BBQkNERUZHSA","summary":[{"type":"summary_text","text":"keep me"}]}}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "resp_ws_invalid_encrypted_content_object_ok", result.RequestID)
	require.Nil(t, upstream.lastReq, "invalid_encrypted_content 单对象 input 不应回退 HTTP")
	require.Equal(t, int32(2), wsAttempts.Load(), "单对象 reasoning input 也应触发一次清洗后重试")

	wsRequestMu.Lock()
	requests := append([][]byte(nil), wsRequestPayloads...)
	wsRequestMu.Unlock()
	require.Len(t, requests, 2)
	require.True(t, gjson.GetBytes(requests[0], `input.encrypted_content`).Exists(), "首轮单对象应保留 encrypted_content")
	require.True(t, gjson.GetBytes(requests[1], `input.summary.0.text`).Exists(), "恢复重试应保留 reasoning summary")
	require.False(t, gjson.GetBytes(requests[1], `input.encrypted_content`).Exists(), "恢复重试只应移除 encrypted_content")
	require.Equal(t, "reasoning", gjson.GetBytes(requests[1], `input.type`).String())
	require.False(t, gjson.GetBytes(requests[1], `previous_response_id`).Exists(), "恢复重试应移除 previous_response_id")
}

func TestOpenAIGatewayService_Forward_WSv2InvalidEncryptedContentKeepsPreviousResponseIDForFunctionCallOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var wsAttempts atomic.Int32
	var wsRequestPayloads [][]byte
	var wsRequestMu sync.Mutex
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := wsAttempts.Add(1)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket failed: %v", err)
			return
		}
		defer func() {
			_ = conn.Close()
		}()

		var req map[string]any
		if err := conn.ReadJSON(&req); err != nil {
			t.Errorf("read ws request failed: %v", err)
			return
		}
		reqRaw, _ := json.Marshal(req)
		wsRequestMu.Lock()
		wsRequestPayloads = append(wsRequestPayloads, reqRaw)
		wsRequestMu.Unlock()
		if attempt == 1 {
			_ = conn.WriteJSON(map[string]any{
				"type": "error",
				"error": map[string]any{
					"code":    "invalid_encrypted_content",
					"type":    "invalid_request_error",
					"message": "The encrypted content could not be verified.",
				},
			})
			return
		}
		_ = conn.WriteJSON(map[string]any{
			"type": "response.completed",
			"response": map[string]any{
				"id":    "resp_ws_invalid_encrypted_content_function_call_output_ok",
				"model": "gpt-5.3-codex",
				"usage": map[string]any{
					"input_tokens":  1,
					"output_tokens": 1,
					"input_tokens_details": map[string]any{
						"cached_tokens": 0,
					},
				},
			},
		})
	}))
	defer wsServer.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "custom-client/1.0")

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"resp_http_drop_reasoning","usage":{"input_tokens":1,"output_tokens":1}}`)),
		},
	}

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.FallbackCooldownSeconds = 1

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
	}

	account := &Account{
		ID:          98,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": wsServer.URL,
		},
		Extra: map[string]any{
			"responses_websockets_v2_enabled": true,
		},
	}

	body := []byte(`{"model":"gpt-5.3-codex","stream":false,"previous_response_id":"resp_prev_function_call","input":[{"type":"reasoning","encrypted_content":"gAAAAAAAAAAACQoLDA0ODxAREhMUFRYXGBkaGxwdHh8gISIjJCUmJygpKissLS4vMDEyMzQ1Njc4OTo7PD0-P0BBQkNERUZHSA"},{"type":"function_call_output","call_id":"call_123","output":"ok"}]}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "resp_ws_invalid_encrypted_content_function_call_output_ok", result.RequestID)
	require.Nil(t, upstream.lastReq, "function_call_output + invalid_encrypted_content 不应回退 HTTP")
	require.Equal(t, int32(2), wsAttempts.Load(), "应只做一次保锚点的清洗后重试")

	wsRequestMu.Lock()
	requests := append([][]byte(nil), wsRequestPayloads...)
	wsRequestMu.Unlock()
	require.Len(t, requests, 2)
	require.True(t, gjson.GetBytes(requests[0], "previous_response_id").Exists(), "首轮请求应保留 previous_response_id")
	require.True(t, gjson.GetBytes(requests[1], "previous_response_id").Exists(), "function_call_output 恢复重试不应移除 previous_response_id")
	require.False(t, gjson.GetBytes(requests[1], `input.0.encrypted_content`).Exists(), "恢复重试应移除 reasoning encrypted_content")
	require.Equal(t, "function_call_output", gjson.GetBytes(requests[1], `input.0.type`).String(), "清洗后应保留 function_call_output 作为首个输入项")
	require.Equal(t, "call_123", gjson.GetBytes(requests[1], `input.0.call_id`).String())
	require.Equal(t, "ok", gjson.GetBytes(requests[1], `input.0.output`).String())
	require.Equal(t, "resp_prev_function_call", gjson.GetBytes(requests[1], "previous_response_id").String())
}

func TestIsNoToolOutputFoundError(t *testing.T) {
	require.True(t, isNoToolOutputFoundError(400, "No tool output found for function call call_WxwK96bNaNbnbwAWnXrAwS4V."))
	require.True(t, isNoToolOutputFoundError(400, "No tool output found for function call call_abc123."))
	require.False(t, isNoToolOutputFoundError(400, "The encrypted content could not be verified."))
	require.False(t, isNoToolOutputFoundError(500, "No tool output found for function call call_abc."))
	require.False(t, isNoToolOutputFoundError(400, ""))
}

func TestRecoverNoToolOutputFound(t *testing.T) {
	t.Run("移除 function_call_output 并清掉 previous_response_id", func(t *testing.T) {
		reqBody := map[string]any{
			"model":                "gpt-5.5",
			"previous_response_id": "resp_prev",
			"input": []any{
				map[string]any{"type": "input_text", "text": "hello"},
				map[string]any{"type": "function_call_output", "call_id": "call_1", "output": "result"},
			},
		}
		ok := recoverNoToolOutputFound(reqBody)
		require.True(t, ok)
		_, hasPrevResponseID := reqBody["previous_response_id"]
		require.False(t, hasPrevResponseID, "previous_response_id 应被清除")

		input := reqBody["input"].([]any)
		require.Len(t, input, 1)
		item := input[0].(map[string]any)
		require.Equal(t, "input_text", item["type"])
	})

	t.Run("无 function_call_output 时返回 false", func(t *testing.T) {
		reqBody := map[string]any{
			"model":                "gpt-5.5",
			"previous_response_id": "resp_prev",
			"input": []any{
				map[string]any{"type": "input_text", "text": "hello"},
			},
		}
		ok := recoverNoToolOutputFound(reqBody)
		require.False(t, ok)
		_, hasPrevResponseID := reqBody["previous_response_id"]
		require.True(t, hasPrevResponseID, "previous_response_id 不应被修改")
	})

	t.Run("input 为空时返回 false", func(t *testing.T) {
		reqBody := map[string]any{"model": "gpt-5.5"}
		require.False(t, recoverNoToolOutputFound(reqBody))
	})

	t.Run("多个 function_call_output 全部移除", func(t *testing.T) {
		reqBody := map[string]any{
			"model":                "gpt-5.5",
			"previous_response_id": "resp_prev",
			"input": []any{
				map[string]any{"type": "function_call_output", "call_id": "call_1", "output": "r1"},
				map[string]any{"type": "input_text", "text": "continue"},
				map[string]any{"type": "function_call_output", "call_id": "call_2", "output": "r2"},
			},
		}
		ok := recoverNoToolOutputFound(reqBody)
		require.True(t, ok)
		input := reqBody["input"].([]any)
		require.Len(t, input, 1)
		require.Equal(t, "input_text", input[0].(map[string]any)["type"])
	})
}

func TestOpenAIGatewayService_Forward_HTTPRetriesNoToolOutputFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "custom-client/1.0")
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	upstream := &httpUpstreamSequenceRecorder{
		responses: []*http.Response{
			{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(
					`{"error":{"code":null,"message":"No tool output found for function call call_abc123.","param":"input","type":"invalid_request_error"}}`,
				)),
			},
			{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(
					`{"id":"resp_no_tool_retry_ok","usage":{"input_tokens":1,"output_tokens":2,"input_tokens_details":{"cached_tokens":0}}}`,
				)),
			},
		},
	}

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
	}

	account := &Account{
		ID:          201,
		Name:        "openai-apikey-no-tool",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key": "sk-test-no-tool",
		},
	}

	body := []byte(`{"model":"gpt-5.5","stream":false,"previous_response_id":"resp_prev_no_tool","input":[{"type":"function_call_output","call_id":"call_abc123","output":"tool result"},{"type":"input_text","text":"continue"}]}`)
	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 2, upstream.callCount, "no_tool_output_found 应重试一次")
	require.Len(t, upstream.bodies, 2)

	firstBody := upstream.bodies[0]
	secondBody := upstream.bodies[1]
	require.Equal(t, "function_call_output", gjson.GetBytes(firstBody, "input.0.type").String(), "首次请求应包含 function_call_output")

	for _, item := range gjson.GetBytes(secondBody, "input").Array() {
		require.NotEqual(t, "function_call_output", item.Get("type").String(), "重试请求不应包含 function_call_output")
	}
	require.Equal(t, "input_text", gjson.GetBytes(secondBody, "input.0.type").String(), "重试请求应保留 input_text")
}
