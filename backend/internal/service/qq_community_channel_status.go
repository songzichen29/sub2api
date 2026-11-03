package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/qqcommunity"
)

type qqBotChannelStatus struct {
	monitor  *ChannelMonitorV2Service
	settings channelMonitorRuntimeReader
	groups   interface {
		ListActive(context.Context) ([]Group, error)
	}
}

func (p *qqBotChannelStatus) ChannelStatus(ctx context.Context) (*qqcommunity.ChannelStatusReport, error) {
	if p.monitor == nil || p.settings == nil || p.groups == nil {
		return nil, errors.New("channel monitor dependencies unavailable")
	}
	runtime := p.settings.GetChannelMonitorRuntime(ctx)
	if !runtime.Enabled {
		return nil, qqcommunity.ErrChannelStatusDisabled
	}
	if !runtime.PassiveAggregationAllowed() {
		return nil, qqcommunity.ErrChannelStatusUnsupported
	}
	cfg, err := p.monitor.getEnabledConfig(ctx)
	if errors.Is(err, ErrChannelMonitorDisabled) {
		return nil, qqcommunity.ErrChannelStatusDisabled
	}
	if err != nil {
		return nil, err
	}
	filter, err := p.monitor.ParseFilter("90m", nil, nil, nil)
	if err != nil {
		return nil, err
	}
	groups, err := p.groups.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	// Report every active group selected by the operator's monitor configuration.
	// Keep the same group scope for the total and the per-group matrix; a member's
	// personal bindings must not change the station-wide report.
	filter.RestrictGroups = true
	filter.AllowedGroupIDs = make([]int64, 0, len(groups))
	groupNames := make(map[int64]string, len(groups))
	for _, group := range groups {
		if group.ID > 0 && group.IsActive() && !group.IsExclusive {
			filter.AllowedGroupIDs = append(filter.AllowedGroupIDs, group.ID)
			groupNames[group.ID] = group.Name
		}
	}
	report := &qqcommunity.ChannelStatusReport{ShowThroughput: !runtime.HideThroughput}
	if len(filter.AllowedGroupIDs) == 0 {
		return report, nil
	}
	// Read raw aggregate counters only to distinguish missing samples from zero.
	// Map them immediately to a display-only DTO: no volumes, policies, user data
	// or admin diagnostics leave this adapter. Both queries use one config/filter.
	snapshot, err := p.monitor.repo.GetSnapshot(ctx, filter, *cfg, false)
	if err != nil {
		return nil, err
	}
	matrix, err := p.monitor.repo.GetMatrix(ctx, filter, *cfg, ChannelMonitorV2GroupByPlatformGroup, false)
	if err != nil {
		return nil, err
	}
	if snapshot == nil || matrix == nil {
		return nil, errors.New("channel monitor returned an incomplete report")
	}
	report.UpdatedAt = snapshot.Coverage.DataThrough
	if matrix.Coverage.DataThrough.Before(report.UpdatedAt) {
		report.UpdatedAt = matrix.Coverage.DataThrough
	}
	report.PartialCoverage = !snapshot.Coverage.CoverageComplete || !matrix.Coverage.CoverageComplete
	report.Backfilling = (snapshot.Coverage.Bootstrap != nil && snapshot.Coverage.Bootstrap.Active) ||
		(matrix.Coverage.Bootstrap != nil && matrix.Coverage.Bootstrap.Active)
	minimumSample := NormalizeChannelMonitorV2HealthThresholds(cfg.HealthThresholds).MinimumSample
	report.Overall = qqBotChannelStatusMetric(snapshot.Metrics, report.ShowThroughput, minimumSample)
	report.Trend = qqBotChannelStatusPoints(snapshot.Trend, report.ShowThroughput, minimumSample)
	counts := make(map[int64]int, len(matrix.Items))
	for _, row := range matrix.Items {
		if row.GroupID != nil {
			counts[*row.GroupID]++
		}
	}
	for _, row := range matrix.Items {
		if row.GroupID == nil {
			continue
		}
		name, ok := groupNames[*row.GroupID]
		if !ok {
			continue
		}
		if strings.TrimSpace(name) == "" {
			name = fmt.Sprintf("分组 %d", *row.GroupID)
		}
		// A mixed-platform group has separate rows on the web matrix. Preserve
		// those aggregates; averaging their rates or P50s would be incorrect.
		if counts[*row.GroupID] > 1 {
			name += "（" + row.Platform + "）"
		}
		report.Groups = append(report.Groups, qqcommunity.ChannelStatusGroup{Name: name, Metrics: qqBotChannelStatusMetric(row.Metrics, report.ShowThroughput, minimumSample), Buckets: qqBotChannelStatusPoints(row.Buckets, report.ShowThroughput, minimumSample)})
	}
	return report, nil
}

func qqBotChannelStatusPoints(points []ChannelMonitorV2TrendPoint, throughput bool, minimumSample int64) []qqcommunity.ChannelStatusPoint {
	result := make([]qqcommunity.ChannelStatusPoint, 0, len(points))
	for _, point := range points {
		result = append(result, qqcommunity.ChannelStatusPoint{At: point.BucketStart, Metrics: qqBotChannelStatusMetric(point.Metrics, throughput, minimumSample), Health: point.Health.Overall})
	}
	return result
}

func qqBotChannelStatusMetric(metric ChannelMonitorV2Metric, throughput bool, minimumSample int64) qqcommunity.ChannelStatusMetric {
	result := qqcommunity.ChannelStatusMetric{
		HasRequests:         metric.RequestCount > 0,
		InsufficientSamples: metric.RequestCount > 0 && metric.RequestCount < minimumSample,
		ErrorRate:           metric.ErrorRate,
		TTFTP50Ms:           metric.TTFT.P50Ms,
	}
	if metric.CacheRateDenominator > 0 {
		rate := metric.CacheRate
		result.CacheRate = &rate
	}
	if throughput {
		result.RPM = metric.RPM
		result.TokensPerSecond = metric.TPM / 60
	}
	return result
}
