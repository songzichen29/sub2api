package admin

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type metricRecoveryHandlerRepoStub struct {
	service.AccountPerformanceRepository
	monitor *service.AccountPerformanceMonitor
	events  []service.AccountPerformanceEvent
}

func (s *metricRecoveryHandlerRepoStub) GetAccountPerformanceMonitors(_ context.Context, ids []int64) (map[int64]*service.AccountPerformanceMonitor, error) {
	result := make(map[int64]*service.AccountPerformanceMonitor, len(ids))
	for _, id := range ids {
		result[id] = &service.AccountPerformanceMonitor{AccountID: id}
	}
	return result, nil
}

func (s *metricRecoveryHandlerRepoStub) GetAccountPerformanceMonitor(context.Context, int64) (*service.AccountPerformanceMonitor, error) {
	return s.monitor, nil
}

func (s *metricRecoveryHandlerRepoStub) SaveAccountMetricRecoveryConfig(_ context.Context, _, _ int64, config service.AccountMetricRecoveryConfig, _ *int64, _ time.Time) (*service.AccountPerformanceMonitor, error) {
	s.monitor.Config = config
	s.monitor.Revision++
	return s.monitor, nil
}

func (s *metricRecoveryHandlerRepoStub) ReleaseAccountMetricPause(_ context.Context, _, _, _ int64, _ *int64, _ time.Time) (*service.AccountPerformanceMonitor, error) {
	s.monitor.Paused = false
	s.monitor.Config.Enabled = false
	s.monitor.Revision++
	return s.monitor, nil
}

func (s *metricRecoveryHandlerRepoStub) ListAccountPerformanceEvents(context.Context, int64, int64, int) ([]service.AccountPerformanceEvent, error) {
	return s.events, nil
}

func newMetricRecoveryHandlerRouter(repo service.AccountPerformanceRepository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewAccountHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.SetAccountMetricRecoveryService(service.NewAccountMetricRecoveryService(repo, nil, time.Minute))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 91})
		c.Next()
	})
	router.GET("/accounts/:id/metric-recovery", h.GetAccountMetricRecovery)
	router.POST("/accounts/metric-recovery/batch", h.GetBatchAccountMetricRecovery)
	router.PUT("/accounts/:id/metric-recovery", h.SaveAccountMetricRecovery)
	router.POST("/accounts/:id/metric-recovery/release", h.ReleaseAccountMetricRecovery)
	router.GET("/accounts/:id/metric-recovery/events", h.ListAccountMetricRecoveryEvents)
	return router
}

func TestAccountMetricRecoveryBatchValidationAndUnavailableMetrics(t *testing.T) {
	router := newMetricRecoveryHandlerRouter(&metricRecoveryHandlerRepoStub{})
	for _, tc := range []struct {
		body string
		code int
	}{
		{`{"account_ids":[7,8,7]}`, 200}, {`{"account_ids":[0]}`, 400}, {`{"account_ids":[]}`, 400}, {`{"account_ids":["7"]}`, 400},
		{`{"account_ids":[` + strings.Repeat("7,", 100) + `7]}`, 400},
	} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/accounts/metric-recovery/batch", strings.NewReader(tc.body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(response, request)
		require.Equal(t, tc.code, response.Code, response.Body.String())
		if tc.code == 200 {
			require.Contains(t, response.Body.String(), `"metrics_available":false`)
			require.Contains(t, response.Body.String(), `"account_id":8`)
		}
	}
}

type metricRestrictedAdminStub struct {
	service.AdminService
	service.ScopedAdminAccounts
}

func TestAccountMetricRoutesDenyRestrictedAdminKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &AccountHandler{adminService: &metricRestrictedAdminStub{}}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(adminaccess.WithPrincipal(c.Request.Context(), &adminaccess.Principal{Policy: adminaccess.Policy{Rules: []adminaccess.Rule{{Permissions: []string{"accounts.read"}, GroupIDs: []int64{7}}}}}))
	})
	router.Use(h.ScopedAdminKeyAccounts)
	for _, tc := range []struct{ method, path, request string }{
		{"POST", "/api/v1/admin/accounts/metric-recovery/batch", "/api/v1/admin/accounts/metric-recovery/batch"},
		{"GET", "/api/v1/admin/accounts/:id/metric-recovery", "/api/v1/admin/accounts/8/metric-recovery"},
		{"PUT", "/api/v1/admin/accounts/:id/metric-recovery", "/api/v1/admin/accounts/8/metric-recovery"},
		{"GET", "/api/v1/admin/accounts/:id/metric-recovery/events", "/api/v1/admin/accounts/8/metric-recovery/events"},
		{"POST", "/api/v1/admin/accounts/:id/metric-recovery/release", "/api/v1/admin/accounts/8/metric-recovery/release"},
	} {
		router.Handle(tc.method, tc.path, func(c *gin.Context) { t.Fatal("restricted key reached metric handler") })
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(tc.method, tc.request, strings.NewReader(`{"account_ids":[8]}`)))
		require.Equal(t, 403, response.Code, response.Body.String())
	}
}

func TestAccountMetricRecoveryHandlerReadsAndConfiguresWithRevision(t *testing.T) {
	repo := &metricRecoveryHandlerRepoStub{monitor: &service.AccountPerformanceMonitor{AccountID: 7, Config: service.AccountMetricRecoveryConfig{}, Revision: 2}}
	router := newMetricRecoveryHandlerRouter(repo)

	get := httptest.NewRecorder()
	router.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/accounts/7/metric-recovery", nil))
	require.Equal(t, http.StatusOK, get.Code, get.Body.String())
	require.Contains(t, get.Body.String(), `"revision":2`)

	put := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/accounts/7/metric-recovery", bytes.NewBufferString(`{"expected_revision":2,"config":{"enabled":false}}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(put, request)
	require.Equal(t, http.StatusOK, put.Code, put.Body.String())
	require.Equal(t, int64(3), repo.monitor.Revision)
}

func TestAccountMetricRecoveryHandlerRejectsMalformedRevisionAndInvalidID(t *testing.T) {
	router := newMetricRecoveryHandlerRouter(&metricRecoveryHandlerRepoStub{monitor: &service.AccountPerformanceMonitor{AccountID: 7}})

	badID := httptest.NewRecorder()
	router.ServeHTTP(badID, httptest.NewRequest(http.MethodGet, "/accounts/nope/metric-recovery", nil))
	require.Equal(t, http.StatusBadRequest, badID.Code)

	badRevision := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/accounts/7/metric-recovery", bytes.NewBufferString(`{"expected_revision":-1,"config":{"enabled":false}}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(badRevision, request)
	require.Equal(t, http.StatusBadRequest, badRevision.Code)
}
