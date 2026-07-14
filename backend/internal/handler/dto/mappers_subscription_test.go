package dto

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUserSubscriptionFromServiceAdmin_IncludesTotalPoolWhenUserOverdraftDisabled(t *testing.T) {
	t.Parallel()

	dailyLimit := 40.0
	startsAt := time.Now().Add(-24 * time.Hour)
	sub := &service.UserSubscription{
		ID:                  1,
		UserID:              2,
		GroupID:             3,
		StartsAt:            startsAt,
		ExpiresAt:           startsAt.Add(5 * 24 * time.Hour),
		Status:              service.SubscriptionStatusActive,
		WeeklyUsageUSD:      160,
		AllowDailyOverdraft: false,
		Group: &service.Group{
			ID:                  3,
			SubscriptionType:    service.SubscriptionTypeSubscription,
			DailyLimitUSD:       &dailyLimit,
			AllowDailyOverdraft: true,
			DefaultValidityDays: 5,
		},
	}

	userDTO := UserSubscriptionFromService(sub)
	require.NotNil(t, userDTO)
	require.Zero(t, userDTO.OverdraftLimitUSD)
	require.Zero(t, userDTO.OverdraftUsedUSD)

	adminDTO := UserSubscriptionFromServiceAdmin(sub)
	require.NotNil(t, adminDTO)
	require.InDelta(t, 200.0, adminDTO.OverdraftLimitUSD, 0.0001)
	require.InDelta(t, 160.0, adminDTO.OverdraftUsedUSD, 0.0001)
	require.Zero(t, adminDTO.OverdraftDays)
}
