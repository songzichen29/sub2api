//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

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
		body = `{"id":"msg_test","content":[{"type":"text","text":"hi"}],"usage":{"input_tokens":5,"output_tokens":3}}`
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

	// 验证发往 upstream 的请求构造正确
	require.Equal(t, "http://grok2api.local:8000/v1/messages", upstream.capturedURL)
	require.Equal(t, "Bearer sk-grok-test", upstream.capturedAuth)
	require.Equal(t, "sk-grok-test", upstream.capturedAPIKey)
	require.Equal(t, "2023-06-01", upstream.capturedAnthropicVersion)

	// 验证 usage 正确解析
	require.Equal(t, 5, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)

	// 验证响应已透传到 client
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "msg_test")
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
	require.Equal(t, "http://localhost:8000/v1/messages", upstream.capturedURL)
}

func TestGrokGatewayService_ForwardUpstream_TransparentlyForwardsErrorStatus(t *testing.T) {
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
	require.NoError(t, err) // 上游错误透传不算 forward 失败
	require.NotNil(t, result)
	require.Equal(t, "grok-3", result.Model)

	// 客户端应该看到上游的 401 状态和原始 body
	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	require.Contains(t, recorder.Body.String(), "bad api key")
}
