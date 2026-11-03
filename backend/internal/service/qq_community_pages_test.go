package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/qqcommunity"
	"github.com/stretchr/testify/require"
)

type qqPageRuntime struct{ enabled bool }

func (r qqPageRuntime) GetChannelMonitorRuntime(context.Context) ChannelMonitorRuntime {
	return ChannelMonitorRuntime{Enabled: r.enabled, Mode: ChannelMonitorModeV1, ShowQuota: true}
}

type qqPageActive struct{}

func (qqPageActive) ListUserView(context.Context) ([]*UserMonitorView, error) {
	return []*UserMonitorView{{ID: 1, Name: "实际监控", PrimaryModel: "test-model", PrimaryStatus: "operational", Availability7d: 99, Timeline: []UserMonitorTimelinePoint{{Status: "operational", CheckedAt: time.Now()}}}}, nil
}

type qqPageQuality struct{ t *testing.T }

func (q qqPageQuality) List(_ context.Context, uid int64, admin bool) ([]*ModelQualityView, error) {
	require.EqualValues(q.t, 42, uid)
	require.False(q.t, admin)
	return []*ModelQualityView{{ModelQualityPlan: &ModelQualityPlan{ID: "plan", Name: "公开检测", GroupID: 1, Config: ModelQualityConfig{BaseURL: "secret-endpoint", APIKeyID: 999}}, AdminConfig: &ModelQualityConfig{BaseURL: "secret-admin"}, Target: ModelQualityTarget{Model: "test-model", ChannelName: "secret-channel"}, Recent: []*ModelQualityRun{{Reason: "secret-error"}}, Batches: []ModelQualityBatch{{ID: "batch", Verdict: "normal", Count: 1}}}}, nil
}

func TestQQMonitorPageUsesActiveModeAndSanitizedUserData(t *testing.T) {
	p := &qqBotMonitorPages{active: qqPageActive{}, quality: qqPageQuality{t}, status: &qqBotChannelStatus{settings: qqPageRuntime{true}}}
	data, err := p.MonitorPage(context.Background(), 42)
	require.NoError(t, err)
	raw, err := json.Marshal(data)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"primary_model":"test-model"`)
	require.Contains(t, string(raw), `"verdict":"normal"`)
	for _, private := range []string{"secret-endpoint", "secret-admin", "secret-channel", "secret-error", "api_key_id", "latest_quota"} {
		require.NotContains(t, string(raw), private)
	}
	p.status.settings = qqPageRuntime{false}
	_, err = p.MonitorPage(context.Background(), 42)
	require.ErrorIs(t, err, qqcommunity.ErrChannelStatusDisabled)
}
