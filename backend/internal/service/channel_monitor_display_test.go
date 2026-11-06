package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type monitorDisplaySettingsStub struct {
	SettingRepository
	value  string
	err    error
	writes int
}

func (r *monitorDisplaySettingsStub) GetValue(context.Context, string) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	if r.value == "" {
		return "", ErrSettingNotFound
	}
	return r.value, nil
}
func (r *monitorDisplaySettingsStub) Set(_ context.Context, key, value string) error {
	if key != SettingKeyChannelMonitorV1Display {
		panic("unexpected setting key")
	}
	if r.err != nil {
		return r.err
	}
	r.value = value
	r.writes++
	return nil
}

type monitorDisplayGroupStub struct{ err error }

func (r monitorDisplayGroupStub) GetByID(_ context.Context, id int64) (*Group, error) {
	if r.err != nil {
		return nil, r.err
	}
	return &Group{ID: id}, nil
}

func TestChannelMonitorDisplayConfigRoundTripAndOrdering(t *testing.T) {
	ctx := context.Background()
	repo := &monitorDisplaySettingsStub{}
	svc := &SettingService{settingRepo: repo, defaultSubGroupReader: monitorDisplayGroupStub{}}
	empty, err := svc.GetChannelMonitorDisplayConfig(ctx)
	require.NoError(t, err)
	require.Empty(t, empty.ProviderOrder)
	cfg := ChannelMonitorDisplayConfig{
		ProviderOrder: []string{" grok ", "openai", "anthropic"},
		MonitorOrder:  []int64{3, 1, 2},
		GroupMappings: []ChannelMonitorGroupMapping{{MonitorID: 3, GroupID: 8}},
	}
	saved, err := svc.UpdateChannelMonitorDisplayConfig(ctx, cfg)
	require.NoError(t, err)
	loaded, err := svc.GetChannelMonitorDisplayConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, saved, loaded)
	views := []*UserMonitorView{
		{ID: 5, Provider: "gemini"}, {ID: 1, Provider: "openai"}, {ID: 9, Provider: "openai"},
		{ID: 2, Provider: "anthropic"}, {ID: 4, Provider: "grok"}, {ID: 3, Provider: "openai"},
	}
	loaded.SortUserViews(views)
	ids := []int64{}
	for _, view := range views {
		ids = append(ids, view.ID)
	}
	require.Equal(t, []int64{4, 3, 1, 9, 2, 5}, ids)
	_, err = svc.UpdateChannelMonitorDisplayConfig(ctx, ChannelMonitorDisplayConfig{})
	require.NoError(t, err)
	cleared, err := svc.GetChannelMonitorDisplayConfig(ctx)
	require.NoError(t, err)
	require.Empty(t, cleared.GroupMappings)
}

func TestChannelMonitorDisplayConfigRejectsInvalidWithoutWriting(t *testing.T) {
	for _, cfg := range []ChannelMonitorDisplayConfig{
		{ProviderOrder: []string{"not-a-provider"}},
		{ProviderOrder: []string{"openai", "OPENAI"}},
		{MonitorOrder: []int64{1, 1}}, {MonitorOrder: []int64{-1}},
		{GroupMappings: []ChannelMonitorGroupMapping{{MonitorID: 1, GroupID: 0}}},
		{GroupMappings: []ChannelMonitorGroupMapping{{MonitorID: 1, GroupID: 2}, {MonitorID: 1, GroupID: 3}}},
	} {
		repo := &monitorDisplaySettingsStub{}
		svc := &SettingService{settingRepo: repo}
		_, err := svc.UpdateChannelMonitorDisplayConfig(context.Background(), cfg)
		require.Error(t, err)
		require.Zero(t, repo.writes)
	}
	repo := &monitorDisplaySettingsStub{}
	svc := &SettingService{settingRepo: repo, defaultSubGroupReader: monitorDisplayGroupStub{err: ErrGroupNotFound}}
	_, err := svc.UpdateChannelMonitorDisplayConfig(context.Background(), ChannelMonitorDisplayConfig{GroupMappings: []ChannelMonitorGroupMapping{{MonitorID: 1, GroupID: 2}}})
	require.ErrorIs(t, err, ErrGroupNotFound)
	require.Zero(t, repo.writes)
	repo.err = errors.New("settings unavailable")
	_, err = svc.GetChannelMonitorDisplayConfig(context.Background())
	require.Error(t, err)
}
