package service

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/qqcommunity"
)

type qqBotMonitorPages struct {
	active interface {
		ListUserView(context.Context) ([]*UserMonitorView, error)
	}
	quality interface {
		List(context.Context, int64, bool) ([]*ModelQualityView, error)
	}
	status *qqBotChannelStatus
}

// Use the same user projections and visibility gates as /monitor. Never return
// the stored monitor (which contains endpoints/credentials) or admin config.
func (p *qqBotMonitorPages) MonitorPage(ctx context.Context, uid int64) (any, error) {
	runtime := p.status.settings.GetChannelMonitorRuntime(ctx)
	if !runtime.Enabled {
		return nil, qqcommunity.ErrChannelStatusDisabled
	}
	if runtime.Mode != ChannelMonitorModeV1 {
		r, err := p.status.ChannelStatus(ctx)
		return map[string]any{"mode": "v2", "report": r}, err
	}
	views, err := p.active.ListUserView(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(views))
	for _, v := range views {
		extras := make([]map[string]any, 0, len(v.ExtraModels))
		for _, e := range v.ExtraModels {
			extras = append(extras, map[string]any{"model": e.Model, "status": e.Status, "latency_ms": e.LatencyMs})
		}
		// Quota snapshots describe operator/upstream balances, so omit them in
		// shared group reports even if the authenticated site UI shows them.
		items = append(items, map[string]any{"id": v.ID, "name": v.Name, "provider": v.Provider, "group_name": v.GroupName, "primary_model": v.PrimaryModel, "primary_status": v.PrimaryStatus, "primary_latency_ms": v.PrimaryLatencyMs, "primary_ping_latency_ms": v.PrimaryPingLatencyMs, "availability_7d": v.Availability7d, "timeline": v.Timeline, "extra_models": extras})
	}
	plans := make([]map[string]any, 0)
	if uid > 0 && p.quality != nil {
		quality, err := p.quality.List(ctx, uid, false)
		if err != nil {
			return nil, fmt.Errorf("load visible quality status: %w", err)
		}
		for _, v := range quality {
			// The strip only needs verdict histories, never generated HTML,
			// answers, raw errors, credential references or target channels.
			batches := make([]map[string]any, 0, len(v.Batches))
			for _, b := range v.Batches {
				batches = append(batches, map[string]any{"id": b.ID, "plan_id": b.PlanID, "started_at": b.StartedAt, "finished_at": b.FinishedAt, "verdict": b.Verdict, "count": b.Count, "stats": b.Stats})
			}
			plans = append(plans, map[string]any{"id": v.ID, "group_id": v.GroupID, "name": v.Name, "enabled": v.Enabled, "public_visible": v.PublicVisible, "interval_minutes": v.IntervalMinutes, "monitor_ids": v.MonitorIDs, "stats": v.Stats, "stats_24h": v.Stats24H, "configs": v.Configs, "target": map[string]any{"model": v.Target.Model, "benchmark": v.Target.Benchmark, "group_name": v.Target.GroupName}, "batches": batches, "recent": []any{}})
		}
	}
	return map[string]any{"mode": "v1", "items": items, "quality_plans": plans}, nil
}
