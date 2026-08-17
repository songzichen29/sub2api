package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

type weekendSkipSubscriptionRepoStub struct {
	userSubRepoNoop
	sub *UserSubscription
}

func (r *weekendSkipSubscriptionRepoStub) GetByID(context.Context, int64) (*UserSubscription, error) {
	return r.sub, nil
}

func (r *weekendSkipSubscriptionRepoStub) Update(_ context.Context, sub *UserSubscription) error {
	r.sub = sub
	return nil
}

func TestWeekendSkippedDurationBetween_RoundTripsToNaturalDuration(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	start := time.Date(2026, 6, 26, 13, 0, 0, 0, time.UTC) // Friday
	naturalExpires := start.Add(5 * 24 * time.Hour)
	weekendSkippedExpires := addWeekendSkippedDuration(start, naturalExpires.Sub(start))

	remainingUsable := weekendSkippedDurationBetween(start, weekendSkippedExpires)
	require.Equal(t, 5*24*time.Hour, remainingUsable)
	require.Equal(t, naturalExpires, start.Add(remainingUsable))
}

func TestWeekendSkippedDurationBetween_IgnoresWeekendTime(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	start := time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC) // Saturday
	end := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)   // Tuesday

	require.Equal(t, 34*time.Hour, weekendSkippedDurationBetween(start, end))
}

func TestAdminSetWeekendSkip_DisablePreservesOriginalQuotaDays(t *testing.T) {
	require.NoError(t, timezone.Init("UTC"))
	previousNow := weekendSkipNow
	t.Cleanup(func() { weekendSkipNow = previousNow })

	daily := 40.0
	startsAt := time.Date(2026, 8, 8, 22, 52, 15, 0, time.UTC) // Saturday
	originalExpiresAt := startsAt.Add(5 * 24 * time.Hour)
	userEnabledAt := time.Date(2026, 8, 8, 23, 8, 35, 0, time.UTC)
	skippedExpiresAt := addWeekendSkippedDuration(userEnabledAt, originalExpiresAt.Sub(userEnabledAt))
	adminDisabledAt := time.Date(2026, 8, 9, 13, 45, 27, 0, time.UTC) // Sunday
	weekendSkipNow = func() time.Time { return adminDisabledAt }

	repo := &weekendSkipSubscriptionRepoStub{sub: &UserSubscription{
		ID:                           183,
		UserID:                       444,
		GroupID:                      38,
		StartsAt:                     startsAt,
		ExpiresAt:                    skippedExpiresAt,
		Status:                       SubscriptionStatusActive,
		AllowDailyOverdraft:          true,
		SkipWeekends:                 true,
		WeekendSkipUserChangedAt:     &userEnabledAt,
		WeekendSkipOriginalExpiresAt: &originalExpiresAt,
		Group: &Group{
			SubscriptionType:    SubscriptionTypeSubscription,
			DailyLimitUSD:       &daily,
			AllowDailyOverdraft: true,
		},
	}}
	svc := NewSubscriptionService(nil, repo, nil, nil, nil)

	updated, err := svc.AdminSetWeekendSkip(context.Background(), 1, 183, false)
	require.NoError(t, err)
	require.False(t, updated.SkipWeekends)
	require.WithinDuration(t, time.Date(2026, 8, 14, 13, 29, 7, 0, time.UTC), updated.ExpiresAt, time.Second)
	require.Equal(t, 6, updated.EffectiveValidityDays())
	require.Equal(t, 5, updated.OverdraftValidityDays())
	limit, ok := updated.DailyOverdraftLimitUSD(updated.Group)
	require.True(t, ok)
	require.InDelta(t, 200.0, limit, 0.0001)
}
