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
	windowStart time.Time
	windowEnd   time.Time
}

func TestAccountMetricRecoveryServiceRejectsEnableWithoutOps(t *testing.T) {
	svc := NewAccountMetricRecoveryService(&accountMetricRecoveryRepoStub{}, nil, time.Minute)
	_, err := svc.SaveConfig(context.Background(), 7, 0, accountMetricConfigForTest(), nil, time.Now())
	require.ErrorIs(t, err, ErrInvalidAccountMetricConfig)
}

func TestAccountMetricRecoveryServiceDoesNotRecoverWhenAutomaticRecoveryOff(t *testing.T) {
	config := accountMetricConfigForTest()
	off := false
	config.AutoRecover = &off
	started := time.Now().Add(-time.Minute)
	repo := &accountMetricRecoveryRepoStub{monitors: []*AccountPerformanceMonitor{{AccountID: 7, Config: config, Paused: true, TrialStartedAt: &started, TrialRequests: 3}}}
	svc := NewAccountMetricRecoveryService(repo, &OpsService{}, time.Minute)
	require.NoError(t, svc.RunEvaluation(context.Background(), time.Now()))
	require.Zero(t, repo.recovered)
	require.Zero(t, repo.ended)
}

func (r *accountMetricRecoveryRepoStub) ListAccountPerformanceMonitors(context.Context) ([]*AccountPerformanceMonitor, error) {
	return r.monitors, r.listErr
}

func (r *accountMetricRecoveryRepoStub) GetAccountMetricSnapshot(_ context.Context, _ int64, start, end time.Time) (*AccountMetricSnapshot, error) {
	r.windowStart, r.windowEnd = start, end
	return r.snapshot, r.snapshotErr
}

func (r *accountMetricRecoveryRepoStub) GetAccountPerformanceMonitor(_ context.Context, id int64) (*AccountPerformanceMonitor, error) {
	for _, monitor := range r.monitors {
		if monitor.AccountID == id {
			return monitor, nil
		}
	}
	return &AccountPerformanceMonitor{AccountID: id}, nil
}

func (r *accountMetricRecoveryRepoStub) GetAccountPerformanceMonitors(ctx context.Context, ids []int64) (map[int64]*AccountPerformanceMonitor, error) {
	result := make(map[int64]*AccountPerformanceMonitor, len(ids))
	for _, id := range ids {
		result[id], _ = r.GetAccountPerformanceMonitor(ctx, id)
	}
	return result, nil
}

func (r *accountMetricRecoveryRepoStub) GetAccountMetricSnapshots(_ context.Context, ids []int64, start, end time.Time) (map[int64]*AccountMetricSnapshot, error) {
	r.windowStart, r.windowEnd = start, end
	result := make(map[int64]*AccountMetricSnapshot, len(ids))
	for _, id := range ids {
		result[id] = r.snapshot
	}
	return result, r.snapshotErr
}

func TestAccountMetricRecoveryStatusUsesSeparateListAndEvaluationWindows(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	configuredAt, trialAt := now.Add(-10*time.Minute), now.Add(-time.Minute)
	monitor := &AccountPerformanceMonitor{AccountID: 7, MetricsSince: &configuredAt}
	repo := &accountMetricRecoveryRepoStub{monitors: []*AccountPerformanceMonitor{monitor}}
	svc := NewAccountMetricRecoveryService(repo, &OpsService{}, time.Minute)
	ctx := context.Background()

	_, err := svc.GetStatus(ctx, 7, now)
	require.NoError(t, err)
	require.Equal(t, configuredAt, repo.windowStart)
	monitor.Paused, monitor.TrialStartedAt = true, &trialAt
	_, err = svc.GetStatus(ctx, 7, now)
	require.NoError(t, err)
	require.Equal(t, trialAt, repo.windowStart)
	statuses, err := svc.GetStatuses(ctx, []int64{7}, now)
	require.NoError(t, err)
	require.True(t, statuses[7].MetricsAvailable)
	require.Equal(t, now.Add(-time.Hour), repo.windowStart)
	require.Equal(t, now, repo.windowEnd)

	monitor.MetricsSince = &now
	status, err := svc.GetStatus(ctx, 7, now)
	require.NoError(t, err)
	require.True(t, status.MetricsAvailable, "an empty fresh window does not mean Ops is disabled")
	require.Nil(t, status.Metrics)
}

func (r *accountMetricRecoveryRepoStub) PauseAccountForMetricBreach(_ context.Context, _ int64, _ AccountMetricEvaluationToken, reason string, _ []AccountMetricRuleEvidence, _ time.Time) (bool, error) {
	r.paused++
	r.lastReason = reason
	return true, nil
}

func (r *accountMetricRecoveryRepoStub) RecoverAccountFromMetricTrial(context.Context, int64, AccountMetricEvaluationToken, []AccountMetricRuleEvidence, time.Time) (bool, error) {
	r.recovered++
	return true, nil
}

func (r *accountMetricRecoveryRepoStub) EndAccountMetricTrial(_ context.Context, _ int64, _ AccountMetricEvaluationToken, outcome string, _ []AccountMetricRuleEvidence, _ time.Time) (bool, error) {
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
