package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPIKeyAuthSnapshotDailyOverdraftRoundtrip(t *testing.T) {
	dailyLimit := 40.0
	groupID := int64(38)
	apiKey := &APIKey{
		ID:      647,
		UserID:  444,
		GroupID: &groupID,
		Key:     "sk-overdraft-roundtrip",
		Status:  StatusActive,
		User:    &User{ID: 444, Status: StatusActive, Role: RoleUser},
		Group: &Group{
			ID:                  groupID,
			Name:                "daily-overdraft",
			Platform:            PlatformOpenAI,
			Status:              StatusActive,
			SubscriptionType:    SubscriptionTypeSubscription,
			DailyLimitUSD:       &dailyLimit,
			AllowDailyOverdraft: true,
		},
	}

	svc := &APIKeyService{}
	snapshot := svc.snapshotFromAPIKey(context.Background(), apiKey)
	require.NotNil(t, snapshot)
	require.Equal(t, apiKeyAuthSnapshotVersion, snapshot.Version)
	require.NotNil(t, snapshot.Group)
	require.True(t, snapshot.Group.AllowDailyOverdraft)

	payload, err := json.Marshal(&APIKeyAuthCacheEntry{Snapshot: snapshot})
	require.NoError(t, err)
	var cached APIKeyAuthCacheEntry
	require.NoError(t, json.Unmarshal(payload, &cached))

	materialized, used, err := svc.applyAuthCacheEntry(apiKey.Key, &cached)
	require.NoError(t, err)
	require.True(t, used)
	require.NotNil(t, materialized.Group)
	require.True(t, materialized.Group.AllowDailyOverdraft)
	require.True(t, materialized.Group.AllowsDailyOverdraft())
}
