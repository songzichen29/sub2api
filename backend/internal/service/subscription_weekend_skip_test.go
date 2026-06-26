package service

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

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
