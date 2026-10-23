package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type poolRetryScriptUpstream struct {
	service.HTTPUpstream
	mu       sync.Mutex
	attempts []int64
	reply    func(accountID int64, attempt int) *http.Response
}

func (u *poolRetryScriptUpstream) Do(_ *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.attempts = append(u.attempts, accountID)
	return u.reply(accountID, len(u.attempts)), nil
}

func poolRetryTestResponse(status int, contentType, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestOpenAIResponsesPoolRetryLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, transport := range []string{"http", "response.failed", "error"} {
		for _, outcome := range []string{"retry_success", "switch_success", "exhausted", "partial_output"} {
			if transport == "http" && outcome == "partial_output" {
				continue
			}
			t.Run(transport+"/"+outcome, func(t *testing.T) {
				const status = http.StatusServiceUnavailable
				const failureMessage = "pool-maintenance-diagnostic"
				const successJSON = `{"id":"resp_recovered","object":"response","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"recovered-answer"}]}],"usage":{"input_tokens":1,"output_tokens":1}}`
				successStream := "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"recovered-answer\"}\n\n" +
					"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":" + successJSON + "}\n\n"
				upstream := &poolRetryScriptUpstream{}
				upstream.reply = func(accountID int64, attempt int) *http.Response {
					if (outcome == "retry_success" && attempt == 2) || (outcome == "switch_success" && accountID == 2) {
						if transport == "http" {
							return poolRetryTestResponse(200, "application/json", successJSON)
						}
						return poolRetryTestResponse(200, "text/event-stream", successStream)
					}
					errorJSON := `{"type":"server_error","code":"server_error","status_code":` + strconv.Itoa(status) + `,"message":"` + failureMessage + `"}`
					if transport == "http" {
						return poolRetryTestResponse(status, "application/json", `{"error":`+errorJSON+"}")
					}
					body := ""
					if outcome == "partial_output" {
						body = "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"partial-answer\"}\n\n"
					}
					if transport == "error" {
						body += "event: error\ndata: {\"type\":\"error\",\"error\":" + errorJSON + "}\n\n"
					} else {
						body += "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_failed_attempt\",\"status\":\"failed\",\"error\":" + errorJSON + "}}\n\n"
					}
					return poolRetryTestResponse(200, "text/event-stream", body)
				}
				cfg := &config.Config{RunMode: config.RunModeSimple}
				cfg.Default.RateMultiplier = 1
				cfg.Gateway.MaxAccountSwitches = 1
				accounts := []service.Account{}
				for _, id := range []int64{1, 2} {
					accounts = append(accounts, service.Account{
						ID: id, Name: "pool-" + strconv.FormatInt(id, 10),
						Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
						Status: service.StatusActive, Schedulable: true, Priority: int(id),
						Credentials: map[string]any{
							"api_key": "sk-test", "base_url": "https://api.example.test",
							"pool_mode": true, "pool_mode_retry_count": float64(1),
							"pool_mode_retry_status_codes": []any{float64(status)},
						},
						Extra: map[string]any{"openai_passthrough": true},
					})
				}
				repo := &openAIWSFailoverHandlerAccountRepoStub{accounts: accounts}
				billingCache := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
				t.Cleanup(billingCache.Stop)
				gateway := service.NewOpenAIGatewayService(
					repo, nil, nil, nil, nil, nil, nil, cfg, nil, nil,
					service.NewBillingService(cfg, nil), nil, billingCache, upstream,
					&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil,
				)
				handler := NewOpenAIGatewayHandler(
					gateway, service.NewConcurrencyService(nil), billingCache,
					service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg),
					nil, nil, nil, nil, cfg,
				)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses",
					strings.NewReader(`{"model":"gpt-5.2","input":"hello","stream":`+strconv.FormatBool(transport != "http")+"}"))
				c.Request.Header.Set("Content-Type", "application/json")
				groupID := int64(42)
				c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
					ID: 1, GroupID: &groupID, User: &service.User{ID: 1, Status: service.StatusActive},
					Group: &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive},
				})
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1, Concurrency: 0})

				handler.Responses(c)

				body := rec.Body.String()
				switch outcome {
				case "retry_success", "switch_success":
					expected := []int64{1, 1}
					if outcome == "switch_success" {
						expected = append(expected, 2)
					}
					require.Equal(t, expected, upstream.attempts)
					require.Equal(t, http.StatusOK, rec.Code)
					require.Contains(t, body, "recovered-answer")
					require.NotContains(t, body, failureMessage)
					require.NotContains(t, body, "response.failed")
					require.NotContains(t, body, "resp_failed_attempt")
				case "exhausted":
					require.Equal(t, []int64{1, 1, 2, 2}, upstream.attempts)
					require.Equal(t, http.StatusBadGateway, rec.Code)
					require.Contains(t, body, "upstream_error")
				case "partial_output":
					require.Equal(t, []int64{1}, upstream.attempts, "never replay after semantic output")
					require.Equal(t, http.StatusOK, rec.Code, "stream headers are already committed")
					require.Contains(t, body, "partial-answer")
					require.Contains(t, body, "response.failed")
					require.Contains(t, body, failureMessage)
					require.NotContains(t, body, "Upstream service temporarily unavailable. Please retry.")
				}
				{
					rawEvents, ok := c.Get(service.OpsUpstreamErrorsKey)
					require.True(t, ok)
					events, ok := rawEvents.([]*service.OpsUpstreamErrorEvent)
					require.True(t, ok)
					require.NotEmpty(t, events)
					for _, event := range events {
						require.Equal(t, status, event.UpstreamStatusCode)
						require.Contains(t, event.Message, failureMessage)
					}
				}
			})
		}
	}
}
