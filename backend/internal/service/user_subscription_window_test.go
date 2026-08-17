package service

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurrentWindowStart_AnchoredToSubscriptionStart(t *testing.T) {
	anchor := time.Date(2026, 5, 10, 16, 30, 0, 0, time.FixedZone("CST", 8*3600))
	now := anchor.Add(50 * time.Hour)

	got := currentWindowStart(anchor, now, dailyWindowDuration)

	want := anchor.Add(48 * time.Hour)
	assert.Equal(t, want, got)
}

func TestWindowNeedsReset_UsesStoredWindowBoundary(t *testing.T) {
	anchor := time.Date(2026, 5, 10, 16, 30, 0, 0, time.FixedZone("CST", 8*3600))
	now := anchor.Add(50 * time.Hour)
	stale := anchor

	require.True(t, windowNeedsReset(anchor, &stale, now, dailyWindowDuration))

	fresh := anchor.Add(27 * time.Hour)
	require.False(t, windowNeedsReset(anchor, &fresh, now, dailyWindowDuration))
}

func TestDailyResetTime_UsesConfiguredTimezoneDayBoundary(t *testing.T) {
	anchor := time.Date(2026, 5, 10, 16, 30, 0, 0, time.FixedZone("CST", 8*3600))
	current := anchor.Add(27 * time.Hour)
	sub := &UserSubscription{
		StartsAt:         anchor,
		DailyWindowStart: &current,
	}

	got := sub.DailyResetTime()

	require.NotNil(t, got)
	assert.Equal(t, timezone.StartOfDay(current).AddDate(0, 0, 1), *got)
}
