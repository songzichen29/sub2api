package service

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
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

func TestUserSubscription_OverdraftValidityDays_CapsAtOriginalExpiresWhenSkippingWeekends(t *testing.T) {
	daily := 10.0
	startsAt := time.Date(2026, 7, 13, 10, 35, 9, 0, time.UTC)
	originalExpires := time.Date(2026, 7, 18, 10, 35, 9, 0, time.UTC) // 5 天自然日
	skippedExpires := time.Date(2026, 7, 20, 10, 35, 9, 0, time.UTC)  // 跳周末后 7 天跨度
	group := &Group{
		SubscriptionType:    SubscriptionTypeSubscription,
		DailyLimitUSD:       &daily,
		AllowDailyOverdraft: true,
	}
	sub := &UserSubscription{
		StartsAt:                     startsAt,
		ExpiresAt:                    skippedExpires,
		SkipWeekends:                 true,
		WeekendSkipOriginalExpiresAt: &originalExpires,
		AllowDailyOverdraft:          true,
	}

	// 日历有效期仍是 7，但透支池必须按原 5 天计划封顶。
	require.Equal(t, 7, sub.EffectiveValidityDays())
	require.Equal(t, 5, sub.OverdraftValidityDays())

	limit, ok := sub.DailyOverdraftLimitUSD(group)
	require.True(t, ok)
	require.InDelta(t, 50.0, limit, 0.0001)

	// 第一天把池子打满：最多只能花 5 日额，不能借到跳周末多出来的 2 天。
	now := startsAt.Add(2 * time.Hour)
	sub.WeeklyUsageUSD = 49.99
	require.True(t, sub.checkDailyLimitAt(group, 0.01, now))
	require.False(t, sub.checkDailyLimitAt(group, 0.02, now))
	sub.WeeklyUsageUSD = 50
	require.False(t, sub.checkDailyLimitAt(group, 0.01, now))
	require.Equal(t, 4, sub.DailyOverdraftBorrowedDays(group, now))
}

func TestUserSubscription_OverdraftValidityDays_IgnoresStaleOriginalExpiresWhenWeekendSkipOff(t *testing.T) {
	daily := 40.0
	startsAt := time.Date(2026, 7, 14, 17, 16, 47, 0, time.UTC)
	expiresAt := startsAt.Add(5 * 24 * time.Hour)
	staleOriginalExpires := time.Date(2026, 7, 18, 10, 35, 9, 0, time.UTC)
	group := &Group{
		SubscriptionType:    SubscriptionTypeSubscription,
		DailyLimitUSD:       &daily,
		AllowDailyOverdraft: true,
	}
	sub := &UserSubscription{
		StartsAt:                     startsAt,
		ExpiresAt:                    expiresAt,
		SkipWeekends:                 false,
		WeekendSkipOriginalExpiresAt: &staleOriginalExpires,
		AllowDailyOverdraft:          true,
	}

	require.Equal(t, 5, sub.EffectiveValidityDays())
	require.Equal(t, 5, sub.OverdraftValidityDays())
	limit, ok := sub.DailyOverdraftLimitUSD(group)
	require.True(t, ok)
	require.InDelta(t, 200.0, limit, 0.0001)
}

func TestUserSubscription_OverdraftValidityDays_IgnoresStaleWeekendSkipOriginal(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))

	daily := 40.0
	startsAt := time.Date(2026, 7, 7, 19, 40, 57, 0, time.UTC)
	staleOriginalExpires := startsAt.AddDate(0, 0, 5)
	group := &Group{
		SubscriptionType:    SubscriptionTypeSubscription,
		DailyLimitUSD:       &daily,
		AllowDailyOverdraft: true,
	}
	sub := &UserSubscription{
		StartsAt:                     startsAt,
		ExpiresAt:                    time.Date(2026, 7, 21, 19, 40, 57, 0, time.UTC),
		SkipWeekends:                 true,
		WeekendSkipOriginalExpiresAt: &staleOriginalExpires,
		AllowDailyOverdraft:          true,
	}

	require.Equal(t, 14, sub.EffectiveValidityDays())
	require.Equal(t, 10, sub.OverdraftValidityDays())
	limit, ok := sub.DailyOverdraftLimitUSD(group)
	require.True(t, ok)
	require.InDelta(t, 400.0, limit, 0.0001)
}

func TestUserSubscription_OverdraftValidityDays_WithoutOriginalUsesWorkingDays(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))

	daily := 10.0
	startsAt := time.Date(2026, 7, 13, 10, 35, 9, 0, time.UTC) // Monday
	skippedExpires := time.Date(2026, 7, 20, 10, 35, 9, 0, time.UTC)
	group := &Group{
		SubscriptionType:    SubscriptionTypeSubscription,
		DailyLimitUSD:       &daily,
		AllowDailyOverdraft: true,
	}
	sub := &UserSubscription{
		StartsAt:            startsAt,
		ExpiresAt:           skippedExpires,
		SkipWeekends:        true,
		AllowDailyOverdraft: true,
	}

	// 无 original 备份时，按工作日可用时长折算，不能按 7 天墙钟跨度发额度。
	require.Equal(t, 7, sub.EffectiveValidityDays())
	require.Equal(t, 5, sub.OverdraftValidityDays())
	limit, ok := sub.DailyOverdraftLimitUSD(group)
	require.True(t, ok)
	require.InDelta(t, 50.0, limit, 0.0001)
}

func TestUserSubscription_DailyOverdraftElapsedFullDays_IgnoresWeekendWhenSkipping(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))

	daily := 10.0
	// 周五 10:00 开始；下周一 12:00 时，墙钟已过 3 天，但跳周末后可用时长约 26h。
	startsAt := time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC) // Friday
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)      // Monday
	originalExpires := startsAt.Add(5 * 24 * time.Hour)
	group := &Group{
		SubscriptionType:    SubscriptionTypeSubscription,
		DailyLimitUSD:       &daily,
		AllowDailyOverdraft: true,
	}
	sub := &UserSubscription{
		StartsAt:                     startsAt,
		ExpiresAt:                    addWeekendSkippedDuration(startsAt, 5*24*time.Hour),
		SkipWeekends:                 true,
		WeekendSkipOriginalExpiresAt: &originalExpires,
		WeeklyUsageUSD:               3,
		AllowDailyOverdraft:          true,
	}

	// 可用完整日卡=1（约 26h 可用时长），不能把周末两天当已过日卡（墙钟会得到 3）。
	require.Equal(t, 1, sub.dailyOverdraftElapsedFullDays(now))
	// 无当前日窗口时，有效已用至少覆盖已过完整日卡 10；实际累计 3 更小，取 10。
	require.InDelta(t, 10.0, sub.DailyOverdraftUsedUSDAt(group, now), 0.0001)
	limit, ok := sub.DailyOverdraftLimitUSD(group)
	require.True(t, ok)
	require.InDelta(t, 50.0, limit, 0.0001)

	// 对照：若按墙钟 elapsed=3，错误已用会到 30。
	wallElapsed := int(now.Sub(startsAt) / (24 * time.Hour))
	require.Equal(t, 3, wallElapsed)
}

func TestUserSubscription_DailyOverdraftElapsedFullDays_DoesNotRetroactivelySkipWeekends(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))

	startsAt := time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC)      // Friday
	skipEnabledAt := time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC) // Monday, after the first weekend
	now := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)           // Next Monday
	sub := &UserSubscription{
		StartsAt:                 startsAt,
		ExpiresAt:                time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC),
		SkipWeekends:             true,
		WeekendSkipUserChangedAt: &skipEnabledAt,
		AllowDailyOverdraft:      true,
	}

	// The first weekend happened before the user enabled skipping, so it still
	// consumes two daily cards. Only the later weekend is skipped: 3 + 5 = 8.
	require.Equal(t, 8, sub.dailyOverdraftElapsedFullDays(now))
	require.Equal(t, 9, sub.dailyOverdraftNormalDays(now))
}

func ptrOverdraftTime(t time.Time) *time.Time {
	return &t
}
