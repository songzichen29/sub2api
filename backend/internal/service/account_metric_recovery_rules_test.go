package service

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func accountMetricConfigForTest() AccountMetricRecoveryConfig {
	return AccountMetricRecoveryConfig{
		Enabled:         true,
		RecoverySamples: 3,
		SuccessRate: &AccountMetricThreshold{
			PauseThreshold:    0.8,
			RecoveryThreshold: metricValue(0.9),
			MinSamples:        10,
		},
		TTFTMs: &AccountMetricThreshold{
			PauseThreshold:    1200,
			RecoveryThreshold: metricValue(800),
			MinSamples:        5,
		},
	}
}

func metricValue(value float64) *float64 { return &value }

func TestEvaluateAccountMetricPauseThresholdBoundariesAndInsufficientSamples(t *testing.T) {
	config := accountMetricConfigForTest()
	snapshot := &AccountMetricSnapshot{
		SuccessRate:       metricValue(0.8),
		SuccessSamples:    8,
		ErrorSamples:      2,
		AverageTTFTMs:     metricValue(1300),
		TTFTSamples:       4,
		AverageDurationMs: metricValue(10),
	}
	evaluation, err := EvaluateAccountMetricPause(config, snapshot)
	require.NoError(t, err)
	require.False(t, evaluation.Triggered, "equal boundary is not a breach and TTFT lacks its minimum sample count")
	require.Len(t, evaluation.Evidence, 1)
	require.False(t, evaluation.Evidence[0].Breached)

	snapshot.TTFTSamples = 5
	evaluation, err = EvaluateAccountMetricPause(config, snapshot)
	require.NoError(t, err)
	require.True(t, evaluation.Triggered)
	require.Equal(t, "ttft_ms", evaluation.Evidence[1].Metric)
	require.True(t, evaluation.Evidence[1].Breached)
	require.Contains(t, AccountMetricPauseReason(evaluation.Evidence), "above")
}

func TestEvaluateAccountMetricPauseDisabledAndInvalidObservations(t *testing.T) {
	config := accountMetricConfigForTest()
	config.Enabled = false
	evaluation, err := EvaluateAccountMetricPause(config, &AccountMetricSnapshot{SuccessRate: metricValue(0), SuccessSamples: 100})
	require.NoError(t, err)
	require.False(t, evaluation.Triggered)

	config.Enabled = true
	snapshot := &AccountMetricSnapshot{SuccessRate: metricValue(math.NaN()), SuccessSamples: 100, AverageTTFTMs: metricValue(math.Inf(1)), TTFTSamples: 100}
	evaluation, err = EvaluateAccountMetricPause(config, snapshot)
	require.NoError(t, err)
	require.False(t, evaluation.Triggered, "non-finite observations are not evidence")
	require.Empty(t, evaluation.Evidence)
}

func TestNormalizeAccountMetricRecoveryConfigDefaultsAndValidatesHysteresis(t *testing.T) {
	config := AccountMetricRecoveryConfig{Enabled: true, SuccessRate: &AccountMetricThreshold{PauseThreshold: 0.75}}
	normalized, err := NormalizeAccountMetricRecoveryConfig(config)
	require.NoError(t, err)
	require.Equal(t, AccountMetricDefaultCooldownSeconds, normalized.CooldownSeconds)
	require.Equal(t, AccountMetricDefaultRecoverySamples, normalized.RecoverySamples)
	require.Equal(t, AccountMetricDefaultMinSamples, normalized.SuccessRate.MinSamples)
	require.NotNil(t, normalized.SuccessRate.RecoveryThreshold)
	require.Equal(t, 0.75, *normalized.SuccessRate.RecoveryThreshold)

	bad := config
	bad.SuccessRate = &AccountMetricThreshold{PauseThreshold: 0.8, RecoveryThreshold: metricValue(0.7)}
	_, err = NormalizeAccountMetricRecoveryConfig(bad)
	require.ErrorIs(t, err, ErrInvalidAccountMetricConfig)

	bad = config
	bad.SuccessRate = &AccountMetricThreshold{PauseThreshold: math.Inf(1)}
	_, err = NormalizeAccountMetricRecoveryConfig(bad)
	require.ErrorIs(t, err, ErrInvalidAccountMetricConfig)

	_, err = NormalizeAccountMetricRecoveryConfig(AccountMetricRecoveryConfig{Enabled: true})
	require.ErrorIs(t, err, ErrInvalidAccountMetricConfig)
}

func TestEvaluateAccountMetricRecoveryRequiresEveryRuleAndEnoughSamples(t *testing.T) {
	config := accountMetricConfigForTest()
	snapshot := &AccountMetricSnapshot{
		SuccessRate:       metricValue(0.95),
		SuccessSamples:    5,
		AverageTTFTMs:     metricValue(700),
		TTFTSamples:       3,
		AverageDurationMs: metricValue(500),
		DurationSamples:   3,
	}
	evaluation, err := EvaluateAccountMetricRecovery(config, snapshot)
	require.NoError(t, err)
	require.True(t, evaluation.Triggered, "recovery uses its own sample limit and must fit within the bounded trial budget")

	snapshot.SuccessSamples = 10
	snapshot.TTFTSamples = 5
	evaluation, err = EvaluateAccountMetricRecovery(config, snapshot)
	require.NoError(t, err)
	require.True(t, evaluation.Triggered)
	require.Len(t, evaluation.Evidence, 2)

	snapshot.AverageTTFTMs = metricValue(801)
	evaluation, err = EvaluateAccountMetricRecovery(config, snapshot)
	require.NoError(t, err)
	require.False(t, evaluation.Triggered, "recovery uses a stricter hysteresis bound than pause")
}
