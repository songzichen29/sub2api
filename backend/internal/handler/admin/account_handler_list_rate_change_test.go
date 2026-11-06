package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountHandlerListPreservesUpstreamRateHistory(t *testing.T) {
	for _, mode := range []string{"0", "1"} {
		t.Run("lite="+mode, func(t *testing.T) {
			router, adminSvc := setupAccountListRouter()
			change := &service.UpstreamBillingRateChange{
				PreviousRateMultiplier: 0.8, CurrentRateMultiplier: 1.2,
				DetectedAt: time.Date(2026, time.November, 7, 2, 0, 0, 0, time.UTC),
			}
			adminSvc.accounts = []service.Account{{
				ID: 501, Name: "rate-change", Platform: service.PlatformOpenAI,
				Type: service.AccountTypeAPIKey, Status: service.StatusActive,
				Extra: map[string]any{service.UpstreamBillingProbeExtraKey: &service.UpstreamBillingProbeSnapshot{
					Status: service.UpstreamBillingProbeStatusOK, RateChange: change,
				}},
			}}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts?lite="+mode, nil))
			require.Equal(t, http.StatusOK, rec.Code)
			var payload struct {
				Data struct {
					Items []struct {
						Extra struct {
							Probe service.UpstreamBillingProbeSnapshot `json:"upstream_billing_probe"`
						} `json:"extra"`
					} `json:"items"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
			require.Len(t, payload.Data.Items, 1)
			require.Equal(t, change, payload.Data.Items[0].Extra.Probe.RateChange)
		})
	}
}
