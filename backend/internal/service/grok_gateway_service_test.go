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

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
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
	capturedBodies           []string
	response                 string
	statusCode               int
	contentType              string
}

func (s *grokOKUpstream) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	s.capturedURL = req.URL.String()
	s.capturedAuth = req.Header.Get("Authorization")
	s.capturedAPIKey = req.Header.Get("x-api-key")
	s.capturedAnthropicVersion = req.Header.Get("anthropic-version")
	if req.Body != nil {
		buf, _ := io.ReadAll(req.Body)
		s.capturedBody = string(buf)
		s.capturedBodies = append(s.capturedBodies, s.capturedBody)
	}

	statusCode := s.statusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	body := s.response
	if body == "" {
		body = grokChatCompletionResponse
	}
	contentType := s.contentType
	if contentType == "" {
		contentType = "application/json"
	}
	header := http.Header{"Content-Type": []string{contentType}}
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
	require.Equal(t, false, sent["stream"])
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
	require.Equal(t, false, sent["stream"])
	require.Equal(t, "high", sent["reasoning_effort"])

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"object":"response"`)
	require.Contains(t, recorder.Body.String(), `"model":"grok-client"`)
	require.Contains(t, recorder.Body.String(), `"output_text"`)

	var responsesResp apicompat.ResponsesResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &responsesResp))
	require.True(t, strings.HasPrefix(responsesResp.ID, "resp_"), "Grok Responses id must be resp_*, got %q", responsesResp.ID)
	require.NotEqual(t, "chatcmpl_test", responsesResp.ID)
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

func TestGrokGatewayService_ForwardAsChatCompletions_NonStreamExtractsResponsesStyleUsage(t *testing.T) {
	upstream := &grokOKUpstream{
		response: `{"id":"chatcmpl_test","object":"chat.completion","model":"grok-3","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"input_tokens":11,"output_tokens":4,"input_tokens_details":{"cached_tokens":3}}}`,
	}
	s := &GrokGatewayService{httpUpstream: upstream}

	c, recorder := makeTestGinContext()
	account := &Account{
		ID:       12,
		Name:     "chat-completions-usage-test",
		Platform: PlatformGrok,
		Type:     AccountTypeUpstream,
		Credentials: map[string]any{
			"base_url": "http://grok2api.local:8000",
			"api_key":  "sk-grok-test",
		},
	}
	body := []byte(`{"model":"grok-3","messages":[{"role":"user","content":"hello"}],"stream":false}`)

	result, err := s.ForwardAsChatCompletions(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Stream)
	require.Equal(t, 8, result.Usage.InputTokens)
	require.Equal(t, 4, result.Usage.OutputTokens)
	require.Equal(t, 3, result.Usage.CacheReadInputTokens)

	require.Equal(t, "http://grok2api.local:8000/v1/chat/completions", upstream.capturedURL)
	var sent map[string]any
	require.NoError(t, json.Unmarshal([]byte(upstream.capturedBody), &sent))
	require.Equal(t, false, sent["stream"])
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"content":"hi"`)
}

func TestGrokGatewayService_CachedUsageDoesNotDoubleBillInput(t *testing.T) {
	var usage ClaudeUsage
	s := &GrokGatewayService{}
	require.True(t, s.extractCCUsage([]byte(`{
		"usage": {
			"prompt_tokens": 69743,
			"completion_tokens": 1157,
			"prompt_tokens_details": {"cached_tokens": 69248}
		}
	}`), &usage))
	require.Equal(t, 495, usage.InputTokens)
	require.Equal(t, 69248, usage.CacheReadInputTokens)

	cost := (&BillingService{}).computeTokenBreakdown(&ModelPricing{
		InputPricePerToken:     2e-6,
		OutputPricePerToken:    6e-6,
		CacheReadPricePerToken: 0.5e-6,
	}, UsageTokens{
		InputTokens:     usage.InputTokens,
		OutputTokens:    usage.OutputTokens,
		CacheReadTokens: usage.CacheReadInputTokens,
	}, 0.2, "", false)

	require.InDelta(t, 0.00099, cost.InputCost, 1e-12)
	require.InDelta(t, 0.042556, cost.TotalCost, 1e-12)
	require.InDelta(t, 0.0085112, cost.ActualCost, 1e-12)
}

func TestGrokGatewayService_ForwardAsChatCompletions_NonStreamWithoutStreamFieldForcesFalse(t *testing.T) {
	upstream := &grokOKUpstream{}
	s := &GrokGatewayService{httpUpstream: upstream}

	c, _ := makeTestGinContext()
	account := &Account{
		ID:       17,
		Name:     "chat-completions-stream-default-test",
		Platform: PlatformGrok,
		Type:     AccountTypeUpstream,
		Credentials: map[string]any{
			"base_url": "http://grok2api.local:8000",
			"api_key":  "sk-grok-test",
		},
	}
	body := []byte(`{"model":"grok-3","messages":[{"role":"user","content":"hello"}]}`)

	result, err := s.ForwardAsChatCompletions(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Stream)

	var sent map[string]any
	require.NoError(t, json.Unmarshal([]byte(upstream.capturedBody), &sent))
	require.Equal(t, false, sent["stream"])
}

func TestGrokGatewayService_ForwardAsChatCompletions_StreamInjectsIncludeUsageAndExtractsUsage(t *testing.T) {
	upstream := &grokOKUpstream{
		response: strings.Join([]string{
			`data:{"id":"chatcmpl_test","object":"chat.completion.chunk","model":"grok-3","choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":null}]}`,
			"",
			`data:{"id":"chatcmpl_test","object":"chat.completion.chunk","model":"grok-3","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":2,"prompt_tokens_details":{"cached_tokens":5}}}`,
			"",
			`data:[DONE]`,
			"",
		}, "\n"),
	}
	s := &GrokGatewayService{httpUpstream: upstream}

	c, recorder := makeTestGinContext()
	account := &Account{
		ID:       13,
		Name:     "chat-completions-stream-usage-test",
		Platform: PlatformGrok,
		Type:     AccountTypeUpstream,
		Credentials: map[string]any{
			"base_url": "http://grok2api.local:8000",
			"api_key":  "sk-grok-test",
		},
	}
	body := []byte(`{"model":"grok-3","messages":[{"role":"user","content":"hello"}],"stream":true}`)

	result, err := s.ForwardAsChatCompletions(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)
	require.Equal(t, 5, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
	require.Equal(t, 5, result.Usage.CacheReadInputTokens)

	var sent map[string]any
	require.NoError(t, json.Unmarshal([]byte(upstream.capturedBody), &sent))
	streamOptions, ok := sent["stream_options"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, streamOptions["include_usage"])

	out := recorder.Body.String()
	require.Contains(t, out, "data:{")
	require.Contains(t, out, `"content":"hello"`)
	require.Contains(t, out, `"usage":{"prompt_tokens":10,"completion_tokens":2`)
	require.Contains(t, out, "\n\n")
}

func TestGrokGatewayService_ForwardAsChatCompletions_NonStreamCollectsSSEBody(t *testing.T) {
	upstream := &grokOKUpstream{
		contentType: "text/event-stream",
		response: strings.Join([]string{
			`data: {"id":"chatcmpl_sse","object":"chat.completion.chunk","created":123,"model":"grok-3","choices":[{"index":0,"delta":{"content":"O"},"finish_reason":null}]}`,
			"",
			`data: {"id":"chatcmpl_sse","object":"chat.completion.chunk","created":123,"model":"grok-3","choices":[{"index":0,"delta":{"content":"K"},"finish_reason":"stop"}]}`,
			"",
			`data: {"id":"chatcmpl_sse","object":"chat.completion.chunk","created":123,"model":"grok-3","choices":[],"usage":{"prompt_tokens":7,"completion_tokens":2,"total_tokens":9,"prompt_tokens_details":{"cached_tokens":4}}}`,
			"",
			`data: [DONE]`,
			"",
		}, "\n"),
	}
	s := &GrokGatewayService{httpUpstream: upstream}

	c, recorder := makeTestGinContext()
	account := &Account{
		ID:       14,
		Name:     "chat-completions-nonstream-sse-test",
		Platform: PlatformGrok,
		Type:     AccountTypeUpstream,
		Credentials: map[string]any{
			"base_url": "http://grok2api.local:8000",
			"api_key":  "sk-grok-test",
		},
	}
	body := []byte(`{"model":"grok-3","messages":[{"role":"user","content":"hello"}],"stream":false}`)

	result, err := s.ForwardAsChatCompletions(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 3, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
	require.Equal(t, 4, result.Usage.CacheReadInputTokens)
	require.Equal(t, "application/json", recorder.Header().Get("Content-Type"))

	var out apicompat.ChatCompletionsResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &out))
	require.Equal(t, "chatcmpl_sse", out.ID)
	require.Len(t, out.Choices, 1)
	require.JSONEq(t, `"OK"`, string(out.Choices[0].Message.Content))
	require.NotNil(t, out.Usage)
	require.Equal(t, 7, out.Usage.PromptTokens)
	require.Equal(t, 2, out.Usage.CompletionTokens)
	require.Equal(t, 9, out.Usage.TotalTokens)
	require.NotNil(t, out.Usage.PromptTokensDetails)
	require.Equal(t, 4, out.Usage.PromptTokensDetails.CachedTokens)
}

func TestGrokGatewayService_ForwardAsChatCompletions_SSEErrorReturnsFailover(t *testing.T) {
	upstream := &grokOKUpstream{
		contentType: "text/event-stream",
		response: strings.Join([]string{
			`event: error`,
			`data: {"error":{"message":"Console API returned 429","type":"upstream_error","code":"upstream_error"}}`,
			"",
		}, "\n"),
	}
	s := &GrokGatewayService{httpUpstream: upstream}

	c, recorder := makeTestGinContext()
	account := &Account{
		ID:       15,
		Name:     "chat-completions-sse-error-test",
		Platform: PlatformGrok,
		Type:     AccountTypeUpstream,
		Credentials: map[string]any{
			"base_url": "http://grok2api.local:8000",
			"api_key":  "sk-grok-test",
		},
	}
	body := []byte(`{"model":"grok-3","messages":[{"role":"user","content":"hello"}],"stream":false}`)

	result, err := s.ForwardAsChatCompletions(context.Background(), c, account, body)
	require.Error(t, err)
	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
	require.Contains(t, string(failoverErr.ResponseBody), "Console API returned 429")
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Empty(t, recorder.Body.String())
}

func TestGrokGatewayService_GrokUpstreamSupportsPoolModeAndCustomErrorCodes(t *testing.T) {
	account := &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeUpstream,
		Credentials: map[string]any{
			"pool_mode":                  true,
			"pool_mode_retry_count":      4,
			"custom_error_codes_enabled": true,
			"custom_error_codes":         []any{float64(429), "503"},
		},
	}

	require.True(t, account.SupportsPoolModeAndCustomErrors())
	require.True(t, account.IsPoolMode())
	require.Equal(t, 4, account.GetPoolModeRetryCount())
	require.True(t, account.IsCustomErrorCodesEnabled())
	require.ElementsMatch(t, []int{429, 503}, account.GetCustomErrorCodes())
	require.True(t, account.ShouldHandleErrorCode(429))
	require.False(t, account.ShouldHandleErrorCode(403))
}

func TestGrokGatewayService_ForwardAsChatCompletions_PoolModeMarksFailoverRetryable(t *testing.T) {
	upstream := &grokOKUpstream{
		statusCode: http.StatusTooManyRequests,
		response:   `{"error":{"message":"rate limited"}}`,
	}
	s := &GrokGatewayService{httpUpstream: upstream}

	c, _ := makeTestGinContext()
	account := &Account{
		ID:       18,
		Name:     "grok-pool-mode-retryable-test",
		Platform: PlatformGrok,
		Type:     AccountTypeUpstream,
		Credentials: map[string]any{
			"base_url":  "http://grok2api.local:8000",
			"api_key":   "sk-grok-test",
			"pool_mode": true,
		},
	}
	body := []byte(`{"model":"grok-3","messages":[{"role":"user","content":"hello"}],"stream":false}`)

	result, err := s.ForwardAsChatCompletions(context.Background(), c, account, body)
	require.Error(t, err)
	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
	require.True(t, failoverErr.RetryableOnSameAccount)
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
	require.Contains(t, out, `"id":"resp_`)
	require.NotContains(t, out, `"id":"chatcmpl_test"`)
	require.Contains(t, out, "event: response.output_item.added\n")
	require.Contains(t, out, `"content":[{"text":"","type":"output_text"}]`)
	require.Contains(t, out, "event: response.output_text.delta\n")
	require.Contains(t, out, `"delta":"hello"`)
	require.Contains(t, out, "event: response.completed\n")
	require.Contains(t, out, `"usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}`)
	require.Contains(t, out, "data: [DONE]\n\n")
}

func TestGrokGatewayService_ForwardAsResponses_NonStreamSSEErrorReturnsFailover(t *testing.T) {
	upstream := &grokOKUpstream{
		contentType: "text/event-stream",
		response: strings.Join([]string{
			`event: error`,
			`data: {"error":{"message":"Console API returned 429","type":"upstream_error","code":"upstream_error"}}`,
			"",
		}, "\n"),
	}
	s := &GrokGatewayService{httpUpstream: upstream}

	c, recorder := makeTestGinContext()
	account := &Account{
		ID:       16,
		Name:     "responses-sse-error-test",
		Platform: PlatformGrok,
		Type:     AccountTypeUpstream,
		Credentials: map[string]any{
			"base_url": "http://grok2api.local:8000",
			"api_key":  "sk-grok-test",
		},
	}
	body := []byte(`{"model":"grok-3","input":"hello","stream":false}`)

	result, err := s.ForwardAsResponses(context.Background(), c, account, body)
	require.Error(t, err)
	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusTooManyRequests, failoverErr.StatusCode)
	require.Contains(t, string(failoverErr.ResponseBody), "Console API returned 429")
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Empty(t, recorder.Body.String())
}

func TestGrokGatewayService_ForwardAsResponses_PreviousResponseIDReplaysLocalConversation(t *testing.T) {
	upstream := &grokOKUpstream{}
	s := &GrokGatewayService{httpUpstream: upstream}
	account := &Account{
		ID:       10,
		Name:     "responses-continuation-test",
		Platform: PlatformGrok,
		Type:     AccountTypeUpstream,
		Credentials: map[string]any{
			"base_url": "http://grok2api.local:8000",
			"api_key":  "sk-grok-test",
		},
	}

	c1, recorder1 := makeTestGinContext()
	firstBody := []byte(`{"model":"grok-3","input":"hello","stream":false}`)
	_, err := s.ForwardAsResponses(context.Background(), c1, account, firstBody)
	require.NoError(t, err)

	var firstResp apicompat.ResponsesResponse
	require.NoError(t, json.Unmarshal(recorder1.Body.Bytes(), &firstResp))
	require.True(t, strings.HasPrefix(firstResp.ID, "resp_"))

	c2, _ := makeTestGinContext()
	secondBody := []byte(`{"model":"grok-3","previous_response_id":"` + firstResp.ID + `","input":"again","stream":false}`)
	_, err = s.ForwardAsResponses(context.Background(), c2, account, secondBody)
	require.NoError(t, err)
	require.Len(t, upstream.capturedBodies, 2)

	var sent struct {
		Messages []apicompat.ChatMessage `json:"messages"`
	}
	require.NoError(t, json.Unmarshal([]byte(upstream.capturedBodies[1]), &sent))
	require.Len(t, sent.Messages, 3)
	require.Equal(t, "user", sent.Messages[0].Role)
	require.JSONEq(t, `"hello"`, string(sent.Messages[0].Content))
	require.Equal(t, "assistant", sent.Messages[1].Role)
	require.JSONEq(t, `"hi"`, string(sent.Messages[1].Content))
	require.Equal(t, "user", sent.Messages[2].Role)
	require.JSONEq(t, `"again"`, string(sent.Messages[2].Content))
}

func TestGrokGatewayService_ForwardAsResponses_UnknownPreviousResponseIDReturnsClientError(t *testing.T) {
	upstream := &grokOKUpstream{}
	s := &GrokGatewayService{httpUpstream: upstream}
	account := &Account{
		ID:       11,
		Name:     "responses-missing-continuation-test",
		Platform: PlatformGrok,
		Type:     AccountTypeUpstream,
		Credentials: map[string]any{
			"base_url": "http://grok2api.local:8000",
			"api_key":  "sk-grok-test",
		},
	}

	c, _ := makeTestGinContext()
	body := []byte(`{"model":"grok-3","previous_response_id":"resp_missing","input":"again","stream":false}`)
	result, err := s.ForwardAsResponses(context.Background(), c, account, body)

	require.Error(t, err)
	require.Nil(t, result)
	var clientErr *GrokResponsesClientError
	require.ErrorAs(t, err, &clientErr)
	require.Equal(t, http.StatusBadRequest, clientErr.StatusCode)
	require.Contains(t, clientErr.Message, "previous_response_id")
	require.Empty(t, upstream.capturedBody)
}
