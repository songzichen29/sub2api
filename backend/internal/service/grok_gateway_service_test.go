//go:build unit

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const grokChatCompletionResponse = `{"id":"chatcmpl_test","object":"chat.completion","model":"grok-3","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`

// grokOKUpstream 是 grok 测试专用的 HTTPUpstream stub，记录请求并返回固定 200 响应。
type grokOKUpstream struct {
	capturedURL              string
	capturedAuth             string
	capturedAPIKey           string
	capturedAnthropicVersion string
	capturedBody             string
	response                 string
	statusCode               int
}

func (s *grokOKUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	s.capturedURL = req.URL.String()
	s.capturedAuth = req.Header.Get("Authorization")
	s.capturedAPIKey = req.Header.Get("x-api-key")
	s.capturedAnthropicVersion = req.Header.Get("anthropic-version")
	if req.Body != nil {
		buf, _ := io.ReadAll(req.Body)
		s.capturedBody = string(buf)
	}

	statusCode := s.statusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	body := s.response
	if body == "" {
		body = grokChatCompletionResponse
	}
	header := http.Header{"Content-Type": []string{"application/json"}}
	return &http.Response{
		StatusCode: statusCode,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}

func (s *grokOKUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

// makeTestGinContext 创建一个最小可用的 gin.Context，含 Request 用于 GetHeader 调用。
func makeTestGinContext() (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/grok/v1/messages", strings.NewReader(""))
	return c, recorder
}

func TestGrokGatewayService_Forward_RejectsNonUpstreamAccount(t *testing.T) {
	s := &GrokGatewayService{}
	c, _ := makeTestGinContext()

	cases := []struct {
		name     string
		acctType string
	}{
		{"oauth", AccountTypeOAuth},
		{"apikey", AccountTypeAPIKey},
		{"setup-token", AccountTypeSetupToken},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			account := &Account{
				Platform: PlatformGrok,
				Type:     tc.acctType,
			}
			body := []byte(`{"model":"grok-3","messages":[]}`)
			_, err := s.Forward(context.Background(), c, account, body)
			require.Error(t, err)
			require.Contains(t, err.Error(), "only supports type=upstream")
		})
	}
}

func TestGrokGatewayService_ForwardUpstream_MissingCredentials(t *testing.T) {
	s := &GrokGatewayService{}
	body := []byte(`{"model":"grok-3","messages":[]}`)

	t.Run("missing base_url", func(t *testing.T) {
		c, _ := makeTestGinContext()
		account := &Account{
			Platform:    PlatformGrok,
			Type:        AccountTypeUpstream,
			Credentials: map[string]any{"api_key": "sk-test"},
		}
		_, err := s.Forward(context.Background(), c, account, body)
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing base_url or api_key")
	})

	t.Run("missing api_key", func(t *testing.T) {
		c, _ := makeTestGinContext()
		account := &Account{
			Platform:    PlatformGrok,
			Type:        AccountTypeUpstream,
			Credentials: map[string]any{"base_url": "http://localhost:8000"},
		}
		_, err := s.Forward(context.Background(), c, account, body)
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing base_url or api_key")
	})

	t.Run("both empty", func(t *testing.T) {
		c, _ := makeTestGinContext()
		account := &Account{
			Platform:    PlatformGrok,
			Type:        AccountTypeUpstream,
			Credentials: map[string]any{},
		}
		_, err := s.Forward(context.Background(), c, account, body)
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing base_url or api_key")
	})

	t.Run("nil credentials", func(t *testing.T) {
		c, _ := makeTestGinContext()
		account := &Account{
			Platform: PlatformGrok,
			Type:     AccountTypeUpstream,
		}
		_, err := s.Forward(context.Background(), c, account, body)
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing base_url or api_key")
	})
}

func TestGrokGatewayService_ForwardUpstream_RejectsInvalidPayload(t *testing.T) {
	upstream := &grokOKUpstream{}
	s := &GrokGatewayService{httpUpstream: upstream}

	t.Run("invalid json", func(t *testing.T) {
		c, _ := makeTestGinContext()
		account := &Account{
			Platform: PlatformGrok,
			Type:     AccountTypeUpstream,
			Credentials: map[string]any{
				"base_url": "http://localhost:8000",
				"api_key":  "sk-x",
			},
		}
		_, err := s.Forward(context.Background(), c, account, []byte("not-json"))
		require.Error(t, err)
		require.Contains(t, err.Error(), "parse claude request")
	})

	t.Run("missing model", func(t *testing.T) {
		c, _ := makeTestGinContext()
		account := &Account{
			Platform: PlatformGrok,
			Type:     AccountTypeUpstream,
			Credentials: map[string]any{
				"base_url": "http://localhost:8000",
				"api_key":  "sk-x",
			},
		}
		_, err := s.Forward(context.Background(), c, account, []byte(`{"messages":[]}`))
		require.Error(t, err)
		require.Contains(t, err.Error(), "missing model")
	})
}

func TestGrokGatewayService_ForwardUpstream_HappyPath(t *testing.T) {
	upstream := &grokOKUpstream{}
	s := &GrokGatewayService{httpUpstream: upstream}

	c, recorder := makeTestGinContext()
	c.Request.Header.Set("anthropic-version", "2023-06-01")

	account := &Account{
		ID:       42,
		Name:     "test-grok",
		Platform: PlatformGrok,
		Type:     AccountTypeUpstream,
		Credentials: map[string]any{
			"base_url": "http://grok2api.local:8000",
			"api_key":  "sk-grok-test",
		},
	}
	body := []byte(`{"model":"grok-3","messages":[{"role":"user","content":"hi"}],"max_tokens":50}`)

	result, err := s.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)

	// 验证 ForwardResult 字段
	require.Equal(t, "grok-3", result.Model)
	require.False(t, result.Stream)

	// 验证发往 upstream 的请求构造正确：Anthropic 请求会转换成 Chat Completions
	require.Equal(t, "http://grok2api.local:8000/v1/chat/completions", upstream.capturedURL)
	require.Equal(t, "Bearer sk-grok-test", upstream.capturedAuth)
	require.Empty(t, upstream.capturedAPIKey)
	require.Empty(t, upstream.capturedAnthropicVersion)

	var sent map[string]any
	require.NoError(t, json.Unmarshal([]byte(upstream.capturedBody), &sent))
	require.Equal(t, "grok-3", sent["model"])
	require.NotContains(t, sent, "stream")
	require.Contains(t, upstream.capturedBody, `"messages"`)

	// 验证 usage 正确解析
	require.Equal(t, 5, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)

	// 验证响应已透传到 client
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"type":"message"`)
	require.Contains(t, recorder.Body.String(), `"text":"hi"`)
}

func TestGrokGatewayService_ForwardUpstream_TrimsTrailingSlash(t *testing.T) {
	upstream := &grokOKUpstream{}
	s := &GrokGatewayService{httpUpstream: upstream}

	c, _ := makeTestGinContext()
	account := &Account{
		ID:       1,
		Name:     "trim-test",
		Platform: PlatformGrok,
		Type:     AccountTypeUpstream,
		Credentials: map[string]any{
			"base_url": "http://localhost:8000/",
			"api_key":  "sk-x",
		},
	}
	body := []byte(`{"model":"grok-3","messages":[]}`)
	_, err := s.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.Equal(t, "http://localhost:8000/v1/chat/completions", upstream.capturedURL)
}

func TestGrokGatewayService_ForwardAsCC_FailoverOnUnauthorized(t *testing.T) {
	upstream := &grokOKUpstream{
		statusCode: http.StatusUnauthorized,
		response:   `{"error":{"type":"authentication_error","message":"bad api key"}}`,
	}
	s := &GrokGatewayService{httpUpstream: upstream}

	c, recorder := makeTestGinContext()
	account := &Account{
		ID:       1,
		Name:     "auth-fail-test",
		Platform: PlatformGrok,
		Type:     AccountTypeUpstream,
		Credentials: map[string]any{
			"base_url": "http://localhost:8000",
			"api_key":  "wrong-key",
		},
	}
	body := []byte(`{"model":"grok-3","messages":[]}`)

	result, err := s.Forward(context.Background(), c, account, body)
	require.Error(t, err)
	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusUnauthorized, failoverErr.StatusCode)

	// failover 错误不在 service 层提前写响应，交给 handler 切账号或兜底输出。
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Empty(t, recorder.Body.String())
}

func TestGrokGatewayService_ForwardAsResponses_ConvertsRequestToChatCompletions(t *testing.T) {
	upstream := &grokOKUpstream{}
	s := &GrokGatewayService{httpUpstream: upstream}

	c, recorder := makeTestGinContext()
	account := &Account{
		ID:       7,
		Name:     "responses-test",
		Platform: PlatformGrok,
		Type:     AccountTypeUpstream,
		Credentials: map[string]any{
			"base_url": "http://grok2api.local:8000",
			"api_key":  "sk-grok-test",
			"model_mapping": map[string]any{
				"grok-client": "grok-upstream",
			},
		},
	}
	body := []byte(`{"model":"grok-client","input":"hello","max_output_tokens":50,"stream":false,"reasoning":{"effort":"high"}}`)

	result, err := s.ForwardAsResponses(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "grok-client", result.Model)
	require.Equal(t, "grok-upstream", result.UpstreamModel)
	require.NotNil(t, result.ReasoningEffort)
	require.Equal(t, "high", *result.ReasoningEffort)
	require.Equal(t, 5, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)

	require.Equal(t, "http://grok2api.local:8000/v1/chat/completions", upstream.capturedURL)
	require.Equal(t, "Bearer sk-grok-test", upstream.capturedAuth)

	var sent map[string]any
	require.NoError(t, json.Unmarshal([]byte(upstream.capturedBody), &sent))
	require.Equal(t, "grok-upstream", sent["model"])
	require.Contains(t, sent, "messages")
	require.NotContains(t, sent, "input")
	require.NotContains(t, sent, "stream")
	require.Equal(t, "high", sent["reasoning_effort"])

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"object":"response"`)
	require.Contains(t, recorder.Body.String(), `"model":"grok-client"`)
	require.Contains(t, recorder.Body.String(), `"output_text"`)
}

func TestGrokGatewayService_ForwardAsResponses_VersionedBaseURL(t *testing.T) {
	upstream := &grokOKUpstream{}
	s := &GrokGatewayService{httpUpstream: upstream}

	c, _ := makeTestGinContext()
	account := &Account{
		ID:       9,
		Name:     "responses-versioned-base-url-test",
		Platform: PlatformGrok,
		Type:     AccountTypeUpstream,
		Credentials: map[string]any{
			"base_url": "https://api.dwai.cloud/v1",
			"api_key":  "sk-grok-test",
		},
	}
	body := []byte(`{"model":"grok-3","input":"hello","stream":false}`)

	_, err := s.ForwardAsResponses(context.Background(), c, account, body)
	require.NoError(t, err)
	require.Equal(t, "https://api.dwai.cloud/v1/chat/completions", upstream.capturedURL)
}

func TestGrokGatewayService_ForwardAsResponses_StreamEmitsResponsesSSEEvents(t *testing.T) {
	upstream := &grokOKUpstream{
		response: strings.Join([]string{
			`data: {"id":"chatcmpl_test","object":"chat.completion.chunk","model":"grok-3","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
			"",
			`data: {"id":"chatcmpl_test","object":"chat.completion.chunk","model":"grok-3","choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":null}]}`,
			"",
			`data: {"id":"chatcmpl_test","object":"chat.completion.chunk","model":"grok-3","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			"",
			`data: {"id":"chatcmpl_test","object":"chat.completion.chunk","model":"grok-3","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`,
			"",
			`data: [DONE]`,
			"",
		}, "\n"),
	}
	s := &GrokGatewayService{httpUpstream: upstream}

	c, recorder := makeTestGinContext()
	account := &Account{
		ID:       8,
		Name:     "responses-stream-test",
		Platform: PlatformGrok,
		Type:     AccountTypeUpstream,
		Credentials: map[string]any{
			"base_url": "http://grok2api.local:8000",
			"api_key":  "sk-grok-test",
		},
	}
	body := []byte(`{"model":"grok-3","input":"hello","stream":true}`)

	result, err := s.ForwardAsResponses(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)
	require.Equal(t, 10, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)

	require.Equal(t, "http://grok2api.local:8000/v1/chat/completions", upstream.capturedURL)
	var sent map[string]any
	require.NoError(t, json.Unmarshal([]byte(upstream.capturedBody), &sent))
	require.Equal(t, true, sent["stream"])
	require.Contains(t, sent, "stream_options")

	out := recorder.Body.String()
	require.Contains(t, out, "event: response.created\n")
	require.Contains(t, out, "event: response.output_item.added\n")
	require.Contains(t, out, `"content":[{"type":"output_text","text":""}]`)
	require.Contains(t, out, "event: response.output_text.delta\n")
	require.Contains(t, out, `"delta":"hello"`)
	require.Contains(t, out, "event: response.completed\n")
	require.Contains(t, out, `"usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}`)
	require.Contains(t, out, "data: [DONE]\n\n")
}
