package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type qqHostKeysStub struct {
	groups []Group
	err    error
	userID int64
}

func (k *qqHostKeysStub) GetAvailableGroups(_ context.Context, id int64) ([]Group, error) {
	k.userID = id
	return k.groups, k.err
}
func (k *qqHostKeysStub) InvalidateAuthCacheByUserID(context.Context, int64) {}

type qqHostRuntimeStub struct{ enabled bool }

func (k qqHostRuntimeStub) GetChannelMonitorRuntime(context.Context) ChannelMonitorRuntime {
	return ChannelMonitorRuntime{Enabled: k.enabled, Mode: ChannelMonitorModeV2}
}

type qqHostMonitorStub struct {
	filter ChannelMonitorV2Filter
	admin  bool
	called bool
	items  []ChannelMonitorV2MatrixRow
}

func (m *qqHostMonitorStub) ParseFilter(r string, _ []string, _ []string, _ []int64) (ChannelMonitorV2Filter, error) {
	return ChannelMonitorV2Filter{Range: r}, nil
}
func (m *qqHostMonitorStub) Matrix(_ context.Context, f ChannelMonitorV2Filter, _ ChannelMonitorV2GroupBy, admin bool) (*ChannelMonitorV2Matrix, error) {
	m.filter = f
	m.admin = admin
	m.called = true
	return &ChannelMonitorV2Matrix{Items: m.items}, nil
}

func TestQQBotChannelScopeIsDerivedFromBoundAccount(t *testing.T) {
	keys := &qqHostKeysStub{groups: []Group{{ID: 3}, {ID: 7}}}
	delay := int64(200)
	monitor := &qqHostMonitorStub{items: []ChannelMonitorV2MatrixRow{{GroupName: "visible", Platform: "openai", Metrics: ChannelMonitorV2Metric{SuccessRate: 0.95, TTFT: ChannelMonitorV2Latency{P50Ms: &delay}}, Health: ChannelMonitorV2Health{Overall: "healthy"}}}}
	host := &qqBotHost{keys: keys, monitor: monitor, settings: qqHostRuntimeStub{true}}
	reply, err := host.ChannelStatus(context.Background(), 42)
	require.NoError(t, err)
	require.EqualValues(t, 42, keys.userID)
	require.True(t, monitor.filter.RestrictGroups)
	require.Equal(t, []int64{3, 7}, monitor.filter.AllowedGroupIDs)
	require.Equal(t, "1h", monitor.filter.Range)
	require.False(t, monitor.admin)
	require.Contains(t, reply, "95.0%")
	require.Contains(t, reply, "200ms")
	keys.groups = nil
	monitor.called = false
	_, err = host.ChannelStatus(context.Background(), 42)
	require.NoError(t, err)
	require.False(t, monitor.called)
	keys.err = errors.New("authorization unavailable")
	_, err = host.ChannelStatus(context.Background(), 42)
	require.Error(t, err)
	require.False(t, monitor.called)
}
