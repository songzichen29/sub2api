package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type monitorBillingStub struct {
	groups []service.Group
	rates  map[int64]float64
	err    error
	userID int64
}

func (r *monitorBillingStub) GetAvailableGroups(_ context.Context, userID int64) ([]service.Group, error) {
	r.userID = userID
	return r.groups, r.err
}
func (r *monitorBillingStub) GetUserGroupRates(_ context.Context, userID int64) (map[int64]float64, error) {
	r.userID = userID
	return r.rates, r.err
}

func TestChannelMonitorBillingRatesRespectViewerAndZeroOverride(t *testing.T) {
	reader := &monitorBillingStub{
		groups: []service.Group{{ID: 7, Name: "Public", RateMultiplier: 0.5}, {ID: 8, Name: "Custom", RateMultiplier: 1}},
		rates:  map[int64]float64{8: 0},
	}
	h := &ChannelMonitorUserHandler{groupReader: reader}
	items := []channelMonitorUserListItem{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}}
	cfg := &service.ChannelMonitorDisplayConfig{GroupMappings: []service.ChannelMonitorGroupMapping{
		{MonitorID: 1, GroupID: 7}, {MonitorID: 2, GroupID: 8}, {MonitorID: 3, GroupID: 99},
	}}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/channel-monitors", nil)
	ctx.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
	require.True(t, h.addBillingRates(ctx, items, cfg))
	require.Equal(t, int64(42), reader.userID)
	require.Equal(t, 0.5, *items[0].RateMultiplier)
	require.Equal(t, 0.0, *items[1].RateMultiplier)
	require.Nil(t, items[2].RateMultiplier)
	require.Empty(t, items[2].BillingGroupName)
	require.Nil(t, items[3].RateMultiplier)
	raw, err := json.Marshal(items)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"rate_multiplier":0`)
	require.NotContains(t, string(raw), `group_id`)
}

func TestChannelMonitorBillingFailureDoesNotPublishFallbackPrices(t *testing.T) {
	h := &ChannelMonitorUserHandler{groupReader: &monitorBillingStub{err: errors.New("group lookup unavailable")}}
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/channel-monitors", nil)
	ctx.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
	items := []channelMonitorUserListItem{{ID: 1}}
	cfg := &service.ChannelMonitorDisplayConfig{GroupMappings: []service.ChannelMonitorGroupMapping{{MonitorID: 1, GroupID: 7}}}
	require.False(t, h.addBillingRates(ctx, items, cfg))
	require.Equal(t, http.StatusInternalServerError, response.Code)
	require.Nil(t, items[0].RateMultiplier)
}

type monitorDisplayStore struct {
	service.SettingRepository
	raw string
}

func (s *monitorDisplayStore) GetValue(_ context.Context, key string) (string, error) {
	if key != service.SettingKeyChannelMonitorV1Display || s.raw == "" {
		return "", service.ErrSettingNotFound
	}
	return s.raw, nil
}
func (s *monitorDisplayStore) Set(_ context.Context, key, value string) error {
	s.raw = value
	return nil
}
func (s *monitorDisplayStore) GetMultiple(_ context.Context, _ []string) (map[string]string, error) {
	return map[string]string{service.SettingKeyChannelMonitorEnabled: "true", service.SettingKeyChannelMonitorMode: "v1"}, nil
}
func (s *monitorDisplayStore) GetByID(_ context.Context, id int64) (*service.Group, error) {
	return &service.Group{ID: id}, nil
}

type monitorDisplayRepo struct {
	service.ChannelMonitorRepository
}

func (*monitorDisplayRepo) ListEnabled(context.Context) ([]*service.ChannelMonitor, error) {
	return []*service.ChannelMonitor{{ID: 1, Provider: "openai", PrimaryModel: "model"}, {ID: 2, Provider: "openai", PrimaryModel: "model"}, {ID: 3, Provider: "grok", PrimaryModel: "model"}}, nil
}
func (*monitorDisplayRepo) ListLatestForMonitorIDs(context.Context, []int64) (map[int64][]*service.ChannelMonitorLatest, error) {
	return nil, nil
}
func (*monitorDisplayRepo) ComputeAvailabilityForMonitors(context.Context, []int64, int) (map[int64][]*service.ChannelMonitorAvailability, error) {
	return nil, nil
}
func (*monitorDisplayRepo) ListRecentHistoryForMonitors(context.Context, []int64, map[int64]string, int) (map[int64][]*service.ChannelMonitorHistoryEntry, error) {
	return nil, nil
}

func TestChannelMonitorDisplayAdminSaveChangesNextUserResponse(t *testing.T) {
	store := &monitorDisplayStore{}
	settings := service.NewSettingService(store, nil)
	settings.SetDefaultSubscriptionGroupReader(store)
	adminHandler := admin.NewSettingHandler(settings, nil, nil, nil, nil, nil, nil)
	userHandler := NewChannelMonitorUserHandler(service.NewChannelMonitorService(&monitorDisplayRepo{}, nil), settings, nil)
	userHandler.groupReader = &monitorBillingStub{groups: []service.Group{{ID: 7, Name: "Billing", RateMultiplier: 0.5}}}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
		c.Next()
	})
	router.PUT("/admin/channel-monitors/display-config", adminHandler.UpdateChannelMonitorDisplayConfig)
	router.GET("/admin/channel-monitors/display-config", adminHandler.GetChannelMonitorDisplayConfig)
	router.GET("/channel-monitors", userHandler.List)
	save := func(payload string) {
		req := httptest.NewRequest(http.MethodPut, "/admin/channel-monitors/display-config", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	}
	read := func() []channelMonitorUserListItem {
		res := httptest.NewRecorder()
		router.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/channel-monitors", nil))
		require.Equal(t, http.StatusOK, res.Code, res.Body.String())
		var body struct {
			Data struct {
				Items []channelMonitorUserListItem `json:"items"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
		return body.Data.Items
	}
	save(`{"provider_order":["grok","openai"],"monitor_order":[2,1,3],"group_mappings":[{"monitor_id":2,"group_id":7}]}`)
	items := read()
	require.Equal(t, []int64{3, 2, 1}, []int64{items[0].ID, items[1].ID, items[2].ID})
	require.Equal(t, 0.5, *items[1].RateMultiplier)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/admin/channel-monitors/display-config", nil))
	require.Equal(t, http.StatusOK, res.Code)
	require.Contains(t, res.Body.String(), `"provider_order":["grok","openai"]`)
	save(`{"provider_order":["openai","grok"],"monitor_order":[1,2,3],"group_mappings":[]}`)
	items = read()
	require.Equal(t, []int64{1, 2, 3}, []int64{items[0].ID, items[1].ID, items[2].ID})
	require.Nil(t, items[1].RateMultiplier)
}
