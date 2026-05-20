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

func TestUserSubscription_DailyOverdraftUsedUSDAt_CountsExpiredDailyQuota(t *testing.T) {
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
		DailyWindowStart:    ptrOverdraftTime(startsAt.Add(3 * 24 * time.Hour)),
		DailyUsageUSD:       4,
		WeeklyUsageUSD:      12,
		AllowDailyOverdraft: true,
	}

	// 2026-05-08 10:00 -> 2026-05-11 12:00 has 3 full elapsed daily cards.
	// Even if cumulative actual usage is only $12, the first 3 days' $30 quota
	// has expired. The current day's real usage is added on top, so effective
	// usage is $34.
	require.InDelta(t, 34.0, sub.DailyOverdraftConsumedUSD(group, now), 0.0001)
	require.InDelta(t, 34.0, sub.DailyOverdraftUsedUSDAt(group, now), 0.0001)

	sub.WeeklyUsageUSD = 35
	require.InDelta(t, 35.0, sub.DailyOverdraftUsedUSDAt(group, now), 0.0001)
}

func TestUserSubscription_DailyOverdraftUsedUSDAt_WeekMonthUseActualUsage(t *testing.T) {
	daily := 10.0
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	startsAt := time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)
	group := &Group{
		SubscriptionType:    SubscriptionTypeSubscription,
		DailyLimitUSD:       &daily,
		AllowDailyOverdraft: true,
	}
	dailyStart := startsAt.Add(3 * 24 * time.Hour)
	for _, unit := range []string{"week", "month"} {
		sub := &UserSubscription{
			StartsAt:            startsAt,
			ExpiresAt:           startsAt.Add(30 * 24 * time.Hour),
			ValidityUnit:        unit,
			DailyWindowStart:    &dailyStart,
			DailyUsageUSD:       4,
			WeeklyUsageUSD:      12,
			AllowDailyOverdraft: true,
		}

		require.InDelta(t, 12.0, sub.DailyOverdraftUsedUSDAt(group, now), 0.0001, unit)
		require.InDelta(t, 12.0, sub.DailyOverdraftConsumedUSD(group, now), 0.0001, unit)
		require.Equal(t, 0, sub.DailyOverdraftBorrowedDays(group, now), unit)
	}
}

func TestUserSubscription_CheckDailyLimitAfterDisablingOverdraftRepaysDebt(t *testing.T) {
	daily := 40.0
	startsAt := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	now := startsAt.Add(29*24*time.Hour + time.Hour)
	dailyStart := startsAt.Add(29 * 24 * time.Hour)
	group := &Group{
		SubscriptionType:    SubscriptionTypeSubscription,
		DailyLimitUSD:       &daily,
		AllowDailyOverdraft: true,
	}
	sub := &UserSubscription{
		StartsAt:            startsAt,
		ExpiresAt:           startsAt.Add(30 * 24 * time.Hour),
		DailyWindowStart:    &dailyStart,
		DailyUsageUSD:       0,
		WeeklyUsageUSD:      1160 + 15,
		AllowDailyOverdraft: false,
	}

	require.InDelta(t, 15.0, sub.DailyOverdraftDebtUSD(group, now), 0.0001)
	require.True(t, sub.checkDailyLimitAt(group, 25, now))
	require.False(t, sub.checkDailyLimitAt(group, 25.01, now))

	sub.DailyUsageUSD = 10
	sub.WeeklyUsageUSD = 1160 + 15 + 10
	require.True(t, sub.checkDailyLimitAt(group, 15, now))
	require.False(t, sub.checkDailyLimitAt(group, 15.01, now))
}

func TestUserSubscription_CheckDailyLimitAfterDisablingOverdraft_WeekMonthNoElapsedDayDebt(t *testing.T) {
	daily := 40.0
	startsAt := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	now := startsAt.Add(29*24*time.Hour + time.Hour)
	dailyStart := startsAt.Add(29 * 24 * time.Hour)
	group := &Group{
		SubscriptionType:    SubscriptionTypeSubscription,
		DailyLimitUSD:       &daily,
		AllowDailyOverdraft: true,
	}
	sub := &UserSubscription{
		StartsAt:            startsAt,
		ExpiresAt:           startsAt.Add(30 * 24 * time.Hour),
		ValidityUnit:        "month",
		DailyWindowStart:    &dailyStart,
		DailyUsageUSD:       0,
		WeeklyUsageUSD:      1160 + 15,
		AllowDailyOverdraft: false,
	}

	require.Zero(t, sub.DailyOverdraftDebtUSD(group, now))
	require.True(t, sub.checkDailyLimitAt(group, 40, now))
	require.False(t, sub.checkDailyLimitAt(group, 40.01, now))
}

func ptrOverdraftTime(t time.Time) *time.Time {
	return &t
}
