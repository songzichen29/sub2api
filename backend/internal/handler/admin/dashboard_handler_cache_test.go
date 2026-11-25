package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type dashboardUsageRepoCacheProbe struct {
	service.UsageLogRepository
	trendCalls      atomic.Int32
	modelCalls      atomic.Int32
	groupCalls      atomic.Int32
	queryErr        error
	usersTrendCalls atomic.Int32
}

func (r *dashboardUsageRepoCacheProbe) GetUsageTrendWithFilters(
	ctx context.Context,
	startTime, endTime time.Time,
	granularity string,
	userID, apiKeyID, accountID, groupID int64,
	model string,
	requestType *int16,
	stream *bool,
	billingType *int8,
) ([]usagestats.TrendDataPoint, error) {
	calls := r.trendCalls.Add(1)
	if r.queryErr != nil {
		return nil, r.queryErr
	}
	return []usagestats.TrendDataPoint{{
		Date:        "2026-03-11",
		Requests:    int64(calls),
		TotalTokens: 2,
		Cost:        3,
		ActualCost:  4,
	}}, nil
}

func (r *dashboardUsageRepoCacheProbe) GetUserUsageTrend(
	ctx context.Context,
	startTime, endTime time.Time,
	granularity string,
	limit int,
	metric string,
) ([]usagestats.UserUsageTrendPoint, error) {
	r.usersTrendCalls.Add(1)
	return []usagestats.UserUsageTrendPoint{{
		Date:       "2026-03-11",
		UserID:     1,
		Email:      "cache@test.dev",
		Requests:   2,
		Tokens:     20,
		Cost:       2,
		ActualCost: 1,
	}}, nil
}

func resetDashboardReadCachesForTest() {
	dashboardTrendCache = newSnapshotCache(30 * time.Second)
	dashboardUsersTrendCache = newSnapshotCache(30 * time.Second)
	dashboardAPIKeysTrendCache = newSnapshotCache(30 * time.Second)
	dashboardModelStatsCache = newSnapshotCache(30 * time.Second)
	dashboardGroupStatsCache = newSnapshotCache(30 * time.Second)
	dashboardSnapshotV2Cache = newSnapshotCache(30 * time.Second)
}

func TestDashboardHandler_GetUsageTrend_UsesCache(t *testing.T) {
	t.Cleanup(resetDashboardReadCachesForTest)
	resetDashboardReadCachesForTest()

	gin.SetMode(gin.TestMode)
	repo := &dashboardUsageRepoCacheProbe{}
	dashboardSvc := service.NewDashboardService(repo, nil, nil, nil)
	handler := NewDashboardHandler(dashboardSvc, nil)
	router := gin.New()
	router.GET("/admin/dashboard/trend", handler.GetUsageTrend)

	req1 := httptest.NewRequest(http.MethodGet, "/admin/dashboard/trend?start_date=2026-03-01&end_date=2026-03-07&granularity=day", nil)
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)
	require.Equal(t, http.StatusOK, rec1.Code)
	require.Equal(t, "miss", rec1.Header().Get("X-Snapshot-Cache"))

	req2 := httptest.NewRequest(http.MethodGet, "/admin/dashboard/trend?start_date=2026-03-01&end_date=2026-03-07&granularity=day", nil)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusOK, rec2.Code)
	require.Equal(t, "hit", rec2.Header().Get("X-Snapshot-Cache"))
	require.Equal(t, int32(1), repo.trendCalls.Load())
}

func TestDashboardHandler_GetUserUsageTrend_UsesCache(t *testing.T) {
	t.Cleanup(resetDashboardReadCachesForTest)
	resetDashboardReadCachesForTest()

	gin.SetMode(gin.TestMode)
	repo := &dashboardUsageRepoCacheProbe{}
	dashboardSvc := service.NewDashboardService(repo, nil, nil, nil)
	handler := NewDashboardHandler(dashboardSvc, nil)
	router := gin.New()
	router.GET("/admin/dashboard/users-trend", handler.GetUserUsageTrend)

	req1 := httptest.NewRequest(http.MethodGet, "/admin/dashboard/users-trend?start_date=2026-03-01&end_date=2026-03-07&granularity=day&limit=8", nil)
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)
	require.Equal(t, http.StatusOK, rec1.Code)
	require.Equal(t, "miss", rec1.Header().Get("X-Snapshot-Cache"))

	req2 := httptest.NewRequest(http.MethodGet, "/admin/dashboard/users-trend?start_date=2026-03-01&end_date=2026-03-07&granularity=day&limit=8", nil)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusOK, rec2.Code)
	require.Equal(t, "hit", rec2.Header().Get("X-Snapshot-Cache"))
	require.Equal(t, int32(1), repo.usersTrendCalls.Load())

	for _, tc := range []struct {
		metric, cache string
		calls         int32
	}{
		{"actual_cost", "miss", 2},
		{"actual_cost", "hit", 2},
		{"tokens", "hit", 2},
	} {
		req := httptest.NewRequest(http.MethodGet, "/admin/dashboard/users-trend?start_date=2026-03-01&end_date=2026-03-07&granularity=day&limit=8&metric="+tc.metric, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, tc.cache, rec.Header().Get("X-Snapshot-Cache"))
		require.Equal(t, tc.calls, repo.usersTrendCalls.Load())
	}
}

func (r *dashboardUsageRepoCacheProbe) GetModelStatsWithUsageFiltersBySource(
	ctx context.Context, startTime, endTime time.Time, filters usagestats.UsageLogFilters, source string,
) ([]usagestats.ModelStat, error) {
	calls := r.modelCalls.Add(1)
	if r.queryErr != nil {
		return nil, r.queryErr
	}
	return []usagestats.ModelStat{{Model: source, Requests: int64(calls)}}, nil
}

func (r *dashboardUsageRepoCacheProbe) GetGroupStatsWithUsageFilters(
	ctx context.Context, startTime, endTime time.Time, filters usagestats.UsageLogFilters,
) ([]usagestats.GroupStat, error) {
	calls := r.groupCalls.Add(1)
	return []usagestats.GroupStat{{Requests: int64(calls)}}, nil
}

func TestDashboardHandler_GetModelStats_BypassCache(t *testing.T) {
	for _, source := range []string{"requested", "upstream", "mapping"} {
		t.Run(source, func(t *testing.T) {
			resetDashboardReadCachesForTest()
			t.Cleanup(resetDashboardReadCachesForTest)
			gin.SetMode(gin.TestMode)
			repo := &dashboardUsageRepoCacheProbe{}
			handler := NewDashboardHandler(service.NewDashboardService(repo, nil, nil, nil), nil)
			router := gin.New()
			router.GET("/models", handler.GetModelStats)
			url := "/models?start_date=2026-03-01&end_date=2026-03-07&model_source=" + source
			for _, tc := range []struct {
				query, cache    string
				calls, requests int32
			}{
				{"", "miss", 1, 1},
				{"", "hit", 1, 1},
				{"&nocache=1", "bypass", 2, 2},
				{"&nocache=true", "bypass", 3, 3},
				{"", "hit", 3, 1},
			} {
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url+tc.query, nil))
				require.Equal(t, http.StatusOK, rec.Code)
				require.Equal(t, tc.cache, rec.Header().Get("X-Snapshot-Cache"))
				require.Equal(t, tc.calls, repo.modelCalls.Load())
				var body struct {
					Data struct {
						Models []usagestats.ModelStat `json:"models"`
					} `json:"data"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
				require.Len(t, body.Data.Models, 1)
				require.Equal(t, int64(tc.requests), body.Data.Models[0].Requests)
				require.Equal(t, source, body.Data.Models[0].Model)
			}
			repo.queryErr = errors.New("query failed")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url+"&nocache=1", nil))
			require.Equal(t, http.StatusInternalServerError, rec.Code)
		})
	}
}

func TestDashboardHandler_GetSnapshotV2_BypassesNestedCaches(t *testing.T) {
	resetDashboardReadCachesForTest()
	t.Cleanup(resetDashboardReadCachesForTest)
	gin.SetMode(gin.TestMode)
	repo := &dashboardUsageRepoCacheProbe{}
	handler := NewDashboardHandler(service.NewDashboardService(repo, nil, nil, nil), nil)
	router := gin.New()
	router.GET("/snapshot", handler.GetSnapshotV2)
	url := "/snapshot?start_date=2026-03-01&end_date=2026-03-07&include_stats=false&include_group_stats=true&include_users_trend=true"
	etag := ""
	for _, tc := range []struct {
		query, cache    string
		calls, requests int32
	}{
		{"", "miss", 1, 1},
		{"", "hit", 1, 1},
		{"&nocache=1", "bypass", 2, 2},
		{"&nocache=1", "bypass", 3, 3},
		{"", "hit", 3, 1},
	} {
		req := httptest.NewRequest(http.MethodGet, url+tc.query, nil)
		if tc.query != "" {
			req.Header.Set("If-None-Match", etag)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, tc.cache, rec.Header().Get("X-Snapshot-Cache"))
		require.Equal(t, tc.calls, repo.trendCalls.Load())
		require.Equal(t, tc.calls, repo.modelCalls.Load())
		require.Equal(t, tc.calls, repo.groupCalls.Load())
		require.Equal(t, tc.calls, repo.usersTrendCalls.Load())
		var body struct {
			Data dashboardSnapshotV2Response `json:"data"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		require.Len(t, body.Data.Trend, 1)
		require.Len(t, body.Data.Models, 1)
		require.Len(t, body.Data.Groups, 1)
		require.Equal(t, int64(tc.requests), body.Data.Trend[0].Requests)
		require.Equal(t, int64(tc.requests), body.Data.Models[0].Requests)
		require.Equal(t, int64(tc.requests), body.Data.Groups[0].Requests)
		etag = rec.Header().Get("ETag")
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("If-None-Match", etag)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotModified, rec.Code)

	repo.queryErr = errors.New("query failed")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url+"&nocache=1", nil))
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}
