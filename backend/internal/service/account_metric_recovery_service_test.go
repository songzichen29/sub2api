package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type accountMetricRecoveryRepoStub struct {
	AccountPerformanceRepository
	monitors    []*AccountPerformanceMonitor
	snapshot    *AccountMetricSnapshot
	listErr     error
	snapshotErr error
	paused      int
	recovered   int
	ended       int
	lastReason  string
	lastOutcome string
}

func (r *accountMetricRecoveryRepoStub) ListAccountPerformanceMonitors(context.Context) ([]*AccountPerformanceMonitor, error) {
	return r.monitors, r.listErr
}

func (r *accountMetricRecoveryRepoStub) GetAccountMetricSnapshot(context.Context, int64, time.Time, time.Time) (*AccountMetricSnapshot, error) {
	return r.snapshot, r.snapshotErr
}

func (r *accountMetricRecoveryRepoStub) PauseAccountForMetricBreach(_ context.Context, _ int64, _ int64, reason string, _ []AccountMetricRuleEvidence, _ time.Time) (bool, error) {
	r.paused++
	r.lastReason = reason
	return true, nil
}

func (r *accountMetricRecoveryRepoStub) RecoverAccountFromMetricTrial(context.Context, int64, int64, []AccountMetricRuleEvidence, time.Time) (bool, error) {
	r.recovered++
	return true, nil
}

func (r *accountMetricRecoveryRepoStub) EndAccountMetricTrial(_ context.Context, _ int64, _ int64, outcome string, _ []AccountMetricRuleEvidence, _ time.Time) (bool, error) {
	r.ended++
	r.lastOutcome = outcome
	return true, nil
}

func TestAccountMetricRecoveryServicePausesOnlyConfiguredBreaches(t *testing.T) {
	config := AccountMetricRecoveryConfig{
		Enabled: true,
		SuccessRate: &AccountMetricThreshold{
			PauseThreshold:    0.8,
			RecoveryThreshold: metricValue(0.9),
			MinSamples:        5,
		},
	}
	configured := &AccountPerformanceMonitor{AccountID: 7, Config: config, Revision: 3}
	unconfigured := &AccountPerformanceMonitor{AccountID: 8, Config: AccountMetricRecoveryConfig{Enabled: false}}
	repo := &accountMetricRecoveryRepoStub{
		monitors: []*AccountPerformanceMonitor{configured, unconfigured},
		snapshot: &AccountMetricSnapshot{SuccessRate: metricValue(0.4), SuccessSamples: 2, ErrorSamples: 3},
	}
	service := NewAccountMetricRecoveryService(repo, &OpsService{}, time.Minute)
	serviceNow := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)

	require.NoError(t, service.RunEvaluation(context.Background(), serviceNow))
	require.Equal(t, 1, repo.paused)
	require.Contains(t, repo.lastReason, "success_rate")
}

func TestAccountMetricRecoveryServiceRequiresSuccessfulTrialObservations(t *testing.T) {
	startedAt := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC).Add(-time.Minute)
	config := AccountMetricRecoveryConfig{
		Enabled:         true,
		RecoverySamples: 3,
		SuccessRate: &AccountMetricThreshold{
			PauseThreshold:    0.8,
			RecoveryThreshold: metricValue(0.9),
			MinSamples:        10,
		},
	}
	repo := &accountMetricRecoveryRepoStub{
		monitors: []*AccountPerformanceMonitor{{AccountID: 7, Config: config, Revision: 4, Paused: true, TrialStartedAt: &startedAt, TrialRequests: 3}},
		snapshot: &AccountMetricSnapshot{SuccessRate: metricValue(0.95), SuccessSamples: 3},
	}
	service := NewAccountMetricRecoveryService(repo, &OpsService{}, time.Minute)

	require.NoError(t, service.RunEvaluation(context.Background(), startedAt.Add(time.Minute)))
	require.Equal(t, 1, repo.recovered)
	require.Zero(t, repo.ended)
}

func TestAccountMetricRecoveryServiceEndsExhaustedTrialAfterTelemetryGrace(t *testing.T) {
	startedAt := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC).Add(-time.Minute)
	nextTrialAt := startedAt.Add(30 * time.Second)
	config := AccountMetricRecoveryConfig{
		Enabled:         true,
		RecoverySamples: 3,
		SuccessRate:     &AccountMetricThreshold{PauseThreshold: 0.8, RecoveryThreshold: metricValue(0.9), MinSamples: 10},
	}
	repo := &accountMetricRecoveryRepoStub{
		monitors: []*AccountPerformanceMonitor{{AccountID: 7, Config: config, Revision: 2, Paused: true, TrialStartedAt: &startedAt, TrialRequests: 9, NextTrialAt: &nextTrialAt}},
		snapshot: &AccountMetricSnapshot{SuccessRate: metricValue(0.85), SuccessSamples: 2},
	}
	service := NewAccountMetricRecoveryService(repo, &OpsService{}, time.Minute)

	require.NoError(t, service.RunEvaluation(context.Background(), startedAt.Add(time.Minute)))
	require.Equal(t, 1, repo.ended)
	require.Equal(t, "budget_exhausted", repo.lastOutcome)
}

func TestAccountMetricRecoveryServiceRespectsOpsMonitoringSwitch(t *testing.T) {
	settings := &opsRuntimeSettingsSnapshot{monitoringEnabled: false}
	ops := &OpsService{}
	ops.runtimeSettings.Store(settings)
	repo := &accountMetricRecoveryRepoStub{monitors: []*AccountPerformanceMonitor{{AccountID: 7, Config: AccountMetricRecoveryConfig{Enabled: true}}}}
	service := NewAccountMetricRecoveryService(repo, ops, time.Minute)

	require.NoError(t, service.RunEvaluation(context.Background(), time.Now()))
	require.Nil(t, repo.monitors[0].PausedAt)
	require.Zero(t, repo.paused)
}
