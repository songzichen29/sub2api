package service

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestQQChannelCardTrendKeepsHealthAndHidesThroughput(t *testing.T) {
	points := []ChannelMonitorV2TrendPoint{{BucketStart: time.Now(), Metrics: ChannelMonitorV2Metric{RequestCount: 42, RPM: 21, TPM: 12000, ErrorRate: .01}, Health: ChannelMonitorV2Health{Overall: "warning"}}}
	hidden := qqBotChannelStatusPoints(points, false, 5)
	require.Len(t, hidden, 1)
	require.Equal(t, points[0].BucketStart, hidden[0].At)
	require.Equal(t, "warning", hidden[0].Health)
	require.True(t, hidden[0].Metrics.HasRequests)
	require.Zero(t, hidden[0].Metrics.RPM)
	require.Zero(t, hidden[0].Metrics.TokensPerSecond)
	shown := qqBotChannelStatusPoints(points, true, 5)
	require.Equal(t, 21.0, shown[0].Metrics.RPM)
	require.Equal(t, 200.0, shown[0].Metrics.TokensPerSecond)
}
