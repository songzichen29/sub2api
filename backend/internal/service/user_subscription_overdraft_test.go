package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUserSubscription_DailyOverdraftBorrowedDays_ExcludesNormalUsageDays(t *testing.T) {
	daily := 10.0
	startsAt := time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)
	group := &Group{
		SubscriptionType:    SubscriptionTypeSubscription,
		DailyLimitUSD:       &daily,
		AllowDailyOverdraft: true,
	}
	sub := &UserSubscription{
		StartsAt:            startsAt,
		ExpiresAt:           startsAt.Add(5 * 24 * time.Hour),
		WeeklyUsageUSD:      30,
		AllowDailyOverdraft: true,
	}

	require.Equal(t, 0, sub.DailyOverdraftBorrowedDays(group, time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)))

	sub.WeeklyUsageUSD = 35
	require.Equal(t, 1, sub.DailyOverdraftBorrowedDays(group, time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)))

	sub.WeeklyUsageUSD = 60
	require.Equal(t, 2, sub.DailyOverdraftBorrowedDays(group, time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)))

	sub.WeeklyUsageUSD = 60
	require.Equal(t, 0, sub.DailyOverdraftBorrowedDays(group, time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)))
}

func TestUserSubscription_DailyOverdraftUsedUSD_ReturnsActualUsage(t *testing.T) {
	daily := 10.0
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	startsAt := time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)
	group := &Group{
		SubscriptionType:    SubscriptionTypeSubscription,
		DailyLimitUSD:       &daily,
		AllowDailyOverdraft: true,
	}
	sub := &UserSubscription{
		StartsAt:            startsAt,
		ExpiresAt:           startsAt.Add(5 * 24 * time.Hour),
		WeeklyUsageUSD:      12,
		AllowDailyOverdraft: true,
	}

	require.InDelta(t, 12.0, sub.DailyOverdraftConsumedUSD(group, now), 0.0001)
	require.InDelta(t, 12.0, sub.DailyOverdraftUsedUSD(group), 0.0001)
}
