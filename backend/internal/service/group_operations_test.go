package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

type groupOperationsRepoStub struct {
	GroupRepository
	groups map[int64]*Group
}

func (r *groupOperationsRepoStub) GetByIDLite(_ context.Context, id int64) (*Group, error) {
	return r.groups[id], nil
}

func TestGroupOperationsValidation(t *testing.T) {
	id := int64(2)
	repo := &groupOperationsRepoStub{groups: map[int64]*Group{2: {ID: 2, Platform: PlatformOpenAI, Status: StatusActive}}}
	svc := &adminServiceImpl{groupRepo: repo}
	ctx := context.Background()
	got, err := svc.validateGroupOperations(ctx, 1, PlatformOpenAI, 2, &id)
	require.NoError(t, err)
	require.Equal(t, id, *got)
	_, err = svc.validateGroupOperations(ctx, 2, PlatformOpenAI, 2, &id)
	require.Error(t, err)
	_, err = svc.validateGroupOperations(ctx, 1, PlatformAnthropic, 2, &id)
	require.Error(t, err)
	_, err = svc.validateGroupOperations(ctx, 1, PlatformOpenAI, -1, nil)
	require.Error(t, err)
	back := int64(1)
	repo.groups[2].FallbackGroupIDOnUnavailable = &back
	_, err = svc.validateGroupOperations(ctx, 1, PlatformOpenAI, 2, &id)
	require.Error(t, err)
	zero := int64(0)
	got, err = svc.validateGroupOperations(ctx, 1, PlatformOpenAI, 0, &zero)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestGroupOperationsAuthSnapshotIsolation(t *testing.T) {
	target := int64(8)
	key := &APIKey{ID: 1, UserID: 2, User: &User{ID: 2}, Group: &Group{ID: 7, Hydrated: true, UserConcurrencyLimit: 3, FallbackGroupIDOnUnavailable: &target}}
	svc := &APIKeyService{}
	snapshot := svc.snapshotFromAPIKey(context.Background(), key)
	target = 99
	require.EqualValues(t, 8, *snapshot.Group.FallbackGroupIDOnUnavailable)
	data, err := json.Marshal(snapshot)
	require.NoError(t, err)
	var decoded APIKeyAuthSnapshot
	require.NoError(t, json.Unmarshal(data, &decoded))
	first := svc.snapshotToAPIKey("test", &decoded)
	second := svc.snapshotToAPIKey("test", &decoded)
	require.Equal(t, 3, first.Group.UserConcurrencyLimit)
	*first.Group.FallbackGroupIDOnUnavailable = 88
	require.EqualValues(t, 8, *second.Group.FallbackGroupIDOnUnavailable)
	require.EqualValues(t, 8, *decoded.Group.FallbackGroupIDOnUnavailable)
}
