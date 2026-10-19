package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestBuildOpsOutputTPSFromValuesMatchesLinearPercentiles(t *testing.T) {
	stats := buildOpsOutputTPSFromValues([]float64{100, 1, 20, 2}, 123)
	require.NotNil(t, stats)
	require.EqualValues(t, 4, stats.SampleCount)
	require.InDelta(t, 1.15, *stats.P5, 1e-9)
	require.InDelta(t, 1.3, *stats.P10, 1e-9)
	require.InDelta(t, 11, *stats.P50, 1e-9)
	require.InDelta(t, 30.75, *stats.Avg, 1e-9)
}

func TestBuildOpsOutputTPSFromValuesEmpty(t *testing.T) {
	require.Equal(t, &service.OpsOutputTPS{}, buildOpsOutputTPSFromValues(nil, 0))
}
