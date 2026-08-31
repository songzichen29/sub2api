package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAccountTestServiceCompletionIncludesTimingMetrics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/1/test", nil)
	c.Set(accountTestMetricsContextKey, &accountTestMetrics{
		startedAt: time.Now().Add(-1500 * time.Millisecond),
	})

	markAccountTestConnected(c)
	svc := &AccountTestService{}
	svc.sendEvent(c, TestEvent{Type: "content", Text: "ok"})
	svc.sendEvent(c, TestEvent{Type: "test_complete", Success: true})

	var completion TestEvent
	for _, line := range strings.Split(recorder.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event TestEvent
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event))
		if event.Type == "test_complete" {
			completion = event
		}
	}

	require.Equal(t, "test_complete", completion.Type)
	require.GreaterOrEqual(t, completion.ElapsedMs, int64(1500))
	require.GreaterOrEqual(t, completion.ConnectMs, int64(1500))
	require.GreaterOrEqual(t, completion.FirstResponseMs, int64(1500))
}
