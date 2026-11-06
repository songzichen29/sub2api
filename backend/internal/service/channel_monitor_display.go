package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const SettingKeyChannelMonitorV1Display = "channel_monitor_v1_display"

// Display settings are independent of probe credentials and V2 aggregation.
// Array order is significant; unconfigured/new monitors sort after configured ones.
type ChannelMonitorDisplayConfig struct {
	ProviderOrder []string                     `json:"provider_order"`
	MonitorOrder  []int64                      `json:"monitor_order"`
	GroupMappings []ChannelMonitorGroupMapping `json:"group_mappings"`
}

type ChannelMonitorGroupMapping struct {
	MonitorID int64 `json:"monitor_id"`
	GroupID   int64 `json:"group_id"`
}

func emptyChannelMonitorDisplayConfig() *ChannelMonitorDisplayConfig {
	return &ChannelMonitorDisplayConfig{ProviderOrder: []string{}, MonitorOrder: []int64{}, GroupMappings: []ChannelMonitorGroupMapping{}}
}

func (s *SettingService) GetChannelMonitorDisplayConfig(ctx context.Context) (*ChannelMonitorDisplayConfig, error) {
	if s == nil || s.settingRepo == nil {
		return emptyChannelMonitorDisplayConfig(), nil
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyChannelMonitorV1Display)
	if errors.Is(err, ErrSettingNotFound) || (err == nil && strings.TrimSpace(raw) == "") {
		return emptyChannelMonitorDisplayConfig(), nil
	}
	if err != nil {
		return nil, err
	}
	cfg := emptyChannelMonitorDisplayConfig()
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		return nil, fmt.Errorf("decode V1 monitor display config: %w", err)
	}
	return cfg, nil
}

func (s *SettingService) UpdateChannelMonitorDisplayConfig(ctx context.Context, cfg ChannelMonitorDisplayConfig) (*ChannelMonitorDisplayConfig, error) {
	if err := normalizeChannelMonitorDisplayConfig(&cfg); err != nil {
		return nil, err
	}
	if s == nil || s.settingRepo == nil {
		return nil, fmt.Errorf("V1 monitor settings store unavailable")
	}
	// Validate explicit billing links, never infer them from the free-text group_name.
	seen := make(map[int64]bool)
	for _, mapping := range cfg.GroupMappings {
		if seen[mapping.GroupID] {
			continue
		}
		if s.defaultSubGroupReader == nil {
			return nil, fmt.Errorf("billing group reader unavailable")
		}
		group, err := s.defaultSubGroupReader.GetByID(ctx, mapping.GroupID)
		if err != nil {
			return nil, err
		}
		if group == nil {
			return nil, invalidMonitorDisplayConfig("billing group does not exist")
		}
		seen[mapping.GroupID] = true
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	if err := s.settingRepo.Set(ctx, SettingKeyChannelMonitorV1Display, string(data)); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func invalidMonitorDisplayConfig(message string) error {
	return infraerrors.BadRequest("INVALID_MONITOR_DISPLAY_CONFIG", message)
}

func normalizeChannelMonitorDisplayConfig(cfg *ChannelMonitorDisplayConfig) error {
	if len(cfg.ProviderOrder) > 100 || len(cfg.MonitorOrder) > 10000 || len(cfg.GroupMappings) > 10000 {
		return invalidMonitorDisplayConfig("too many display entries")
	}
	providers := make(map[string]bool)
	for i, provider := range cfg.ProviderOrder {
		provider = strings.ToLower(strings.TrimSpace(provider))
		// opencode_go is also supported by the monitor UI and API.
		if _, ok := monitorProviders[provider]; (!ok && provider != "opencode_go") || providers[provider] {
			return invalidMonitorDisplayConfig("invalid or duplicate provider")
		}
		providers[provider] = true
		cfg.ProviderOrder[i] = provider
	}
	monitors := make(map[int64]bool)
	for _, id := range cfg.MonitorOrder {
		if id <= 0 || monitors[id] {
			return invalidMonitorDisplayConfig("invalid or duplicate monitor order ID")
		}
		monitors[id] = true
	}
	mappings := make(map[int64]bool)
	for _, mapping := range cfg.GroupMappings {
		if mapping.MonitorID <= 0 || mapping.GroupID <= 0 || mappings[mapping.MonitorID] {
			return invalidMonitorDisplayConfig("invalid or duplicate billing group mapping")
		}
		mappings[mapping.MonitorID] = true
	}
	if cfg.ProviderOrder == nil {
		cfg.ProviderOrder = []string{}
	}
	if cfg.MonitorOrder == nil {
		cfg.MonitorOrder = []int64{}
	}
	if cfg.GroupMappings == nil {
		cfg.GroupMappings = []ChannelMonitorGroupMapping{}
	}
	return nil
}

func (cfg *ChannelMonitorDisplayConfig) SortUserViews(views []*UserMonitorView) {
	providers := make(map[string]int, len(cfg.ProviderOrder))
	for i, provider := range cfg.ProviderOrder {
		providers[provider] = i + 1
	}
	monitors := make(map[int64]int, len(cfg.MonitorOrder))
	for i, id := range cfg.MonitorOrder {
		monitors[id] = i + 1
	}
	rank := func(value, size int) int {
		if value == 0 {
			return size + 1
		}
		return value
	}
	sort.SliceStable(views, func(i, j int) bool {
		a, b := views[i], views[j]
		pa, pb := rank(providers[a.Provider], len(providers)), rank(providers[b.Provider], len(providers))
		if pa != pb {
			return pa < pb
		}
		if a.Provider != b.Provider {
			return a.Provider < b.Provider
		}
		ma, mb := rank(monitors[a.ID], len(monitors)), rank(monitors[b.ID], len(monitors))
		if ma != mb {
			return ma < mb
		}
		return a.ID < b.ID
	})
}
