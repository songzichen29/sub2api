package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type metricOptInRepo struct {
	AccountPerformanceRepository
	monitors   []*AccountPerformanceMonitor
	listErr    error
	listHook   func() ([]*AccountPerformanceMonitor, error)
	writeHook  func()
	writeErr   error
	admitErr   error
	allowed    bool
	admitCalls atomic.Int64
}

func (r *metricOptInRepo) ListAccountPerformanceMonitors(context.Context) ([]*AccountPerformanceMonitor, error) {
	if r.listHook != nil {
		return r.listHook()
	}
	return r.monitors, r.listErr
}

func (r *metricOptInRepo) AdmitAccountPerformanceRequest(context.Context, int64, time.Time) (bool, error) {
	r.admitCalls.Add(1)
	return r.allowed, r.admitErr
}

func (r *metricOptInRepo) SaveAccountMetricRecoveryConfig(_ context.Context, accountID, expectedRevision int64, config AccountMetricRecoveryConfig, _ *int64, _ time.Time) (*AccountPerformanceMonitor, error) {
	if r.writeHook != nil {
		r.writeHook()
	}
	if r.writeErr != nil {
		return nil, r.writeErr
	}
	monitor := &AccountPerformanceMonitor{AccountID: accountID, Revision: expectedRevision + 1, Config: config}
	r.monitors = []*AccountPerformanceMonitor{monitor}
	return monitor, nil
}

func (r *metricOptInRepo) ReleaseAccountMetricPause(_ context.Context, accountID, revision, generation int64, _ *int64, _ time.Time) (*AccountPerformanceMonitor, error) {
	if r.writeErr != nil {
		return nil, r.writeErr
	}
	monitor := &AccountPerformanceMonitor{AccountID: accountID, Revision: revision + 1, PauseGeneration: generation + 1}
	r.monitors = []*AccountPerformanceMonitor{monitor}
	return monitor, nil
}

func newMetricOptInTestService(t *testing.T, repo *metricOptInRepo) (*AccountMetricAdmissionService, *AccountMetricRecoveryService) {
	t.Helper()
	gate := NewAccountMetricAdmissionService(repo)
	_, err := gate.Refresh(context.Background())
	require.NoError(t, err)
	return gate, NewAccountMetricRecoveryService(repo, nil, time.Minute, gate)
}

func TestAccountMetricOptInUnconfiguredAndDisabledDoNotQueryDatabase(t *testing.T) {
	repo := &metricOptInRepo{
		monitors: []*AccountPerformanceMonitor{{AccountID: 7, Config: AccountMetricRecoveryConfig{Enabled: true}}, {AccountID: 8}},
		admitErr: errors.New("metric database unavailable"),
	}
	gate, _ := newMetricOptInTestService(t, repo)
	for _, id := range []int64{8, 9} {
		for _, limit := range []int{0, 2} {
			cache := &accountMetricGuardCache{acquired: true}
			result, err := NewConcurrencyService(cache, gate).AcquireAccountSlot(context.Background(), id, limit)
			require.NoError(t, err)
			require.True(t, result.Acquired)
			result.ReleaseFunc()
			if limit > 0 {
				require.Equal(t, 1, cache.releases)
			} else {
				require.Zero(t, cache.releases)
			}
		}
	}
	require.Zero(t, repo.admitCalls.Load(), "ordinary accounts must never touch the metric database on request admission")
	cache := &accountMetricGuardCache{acquired: true}
	result, err := NewConcurrencyService(cache, gate).AcquireAccountSlot(context.Background(), 7, 2)
	require.Nil(t, result)
	require.ErrorContains(t, err, "metric database unavailable")
	require.Equal(t, int64(1), repo.admitCalls.Load())
	require.Equal(t, 1, cache.releases)
}

func TestAccountMetricOptInRefreshFailureRetainsProtection(t *testing.T) {
	repo := &metricOptInRepo{monitors: []*AccountPerformanceMonitor{{AccountID: 7, Config: AccountMetricRecoveryConfig{Enabled: true}}}}
	gate, _ := newMetricOptInTestService(t, repo)
	repo.listErr = errors.New("database down")
	repo.admitErr = repo.listErr
	_, err := gate.Refresh(context.Background())
	require.Error(t, err)
	allowed, err := gate.AdmitAccountPerformanceRequest(context.Background(), 7, time.Now())
	require.False(t, allowed)
	require.Error(t, err)
	allowed, err = gate.AdmitAccountPerformanceRequest(context.Background(), 9, time.Now())
	require.True(t, allowed)
	require.NoError(t, err)
	require.Equal(t, int64(1), repo.admitCalls.Load())
}

func TestAccountMetricOptInColdStartDoesNotAssumeEmptyConfiguration(t *testing.T) {
	repo := &metricOptInRepo{listErr: errors.New("bootstrap database unavailable")}
	gate, err := ProvideAccountMetricAdmissionService(repo)
	require.Nil(t, gate)
	require.ErrorContains(t, err, "initialize account metric admission")
	allowed, err := NewAccountMetricAdmissionService(repo).AdmitAccountPerformanceRequest(context.Background(), 7, time.Now())
	require.False(t, allowed)
	require.ErrorContains(t, err, "not been initialized")
	require.Zero(t, repo.admitCalls.Load())
}

func TestAccountMetricOptInSaveProtectsBeforeCommitAndDisableRemovesAfterSuccess(t *testing.T) {
	repo := &metricOptInRepo{}
	gate, svc := newMetricOptInTestService(t, repo)
	repo.writeHook = func() {
		allowed, err := gate.AdmitAccountPerformanceRequest(context.Background(), 7, time.Now())
		require.NoError(t, err)
		require.False(t, allowed, "enabling must be guarded before SQL commit")
	}
	_, err := svc.SaveConfig(context.Background(), 7, 0, accountMetricConfigForTest(), nil, time.Now())
	require.NoError(t, err)
	require.Equal(t, int64(1), repo.admitCalls.Load())
	repo.writeHook = nil
	_, err = svc.SaveConfig(context.Background(), 7, 1, AccountMetricRecoveryConfig{}, nil, time.Now())
	require.NoError(t, err)
	repo.admitErr = errors.New("database down")
	allowed, err := gate.AdmitAccountPerformanceRequest(context.Background(), 7, time.Now())
	require.NoError(t, err)
	require.True(t, allowed)
	require.Equal(t, int64(1), repo.admitCalls.Load())
}

func TestAccountMetricOptInUncertainEnableAndFailedReleaseStayProtected(t *testing.T) {
	repo := &metricOptInRepo{writeErr: errors.New("commit outcome unknown"), admitErr: errors.New("database down")}
	gate, svc := newMetricOptInTestService(t, repo)
	_, err := svc.SaveConfig(context.Background(), 7, 0, accountMetricConfigForTest(), nil, time.Now())
	require.Error(t, err)
	_, err = svc.Release(context.Background(), 7, 1, 1, nil, time.Now())
	require.Error(t, err)
	allowed, err := gate.AdmitAccountPerformanceRequest(context.Background(), 7, time.Now())
	require.False(t, allowed)
	require.ErrorContains(t, err, "database down")
	repo.writeErr = nil
	_, err = svc.Release(context.Background(), 7, 1, 1, nil, time.Now())
	require.NoError(t, err)
	allowed, err = gate.AdmitAccountPerformanceRequest(context.Background(), 7, time.Now())
	require.True(t, allowed)
	require.NoError(t, err)
}

func TestAccountMetricOptInRefreshRunsWhenOpsDisabled(t *testing.T) {
	repo := &metricOptInRepo{}
	gate, svc := newMetricOptInTestService(t, repo)
	// Simulate a config enabled by another instance; Ops is nil (disabled).
	repo.monitors = []*AccountPerformanceMonitor{{AccountID: 7, Config: AccountMetricRecoveryConfig{Enabled: true}}}
	require.NoError(t, svc.RunEvaluation(context.Background(), time.Now()))
	allowed, err := gate.AdmitAccountPerformanceRequest(context.Background(), 7, time.Now())
	require.NoError(t, err)
	require.False(t, allowed)
	require.Equal(t, int64(1), repo.admitCalls.Load())
}

func TestAccountMetricOptInPausedRowStaysProtectedEvenWithDisabledConfig(t *testing.T) {
	repo := &metricOptInRepo{monitors: []*AccountPerformanceMonitor{{AccountID: 7, Paused: true}}}
	gate, _ := newMetricOptInTestService(t, repo)
	allowed, err := gate.AdmitAccountPerformanceRequest(context.Background(), 7, time.Now())
	require.NoError(t, err)
	require.False(t, allowed)
	require.Equal(t, int64(1), repo.admitCalls.Load())
}

func TestAccountMetricOptInOtherInstanceEnableRequiresSuccessfulRefresh(t *testing.T) {
	repo := &metricOptInRepo{}
	local, svc := newMetricOptInTestService(t, repo)
	other, _ := newMetricOptInTestService(t, repo)
	_, err := svc.SaveConfig(context.Background(), 7, 0, accountMetricConfigForTest(), nil, time.Now())
	require.NoError(t, err)
	allowed, err := local.AdmitAccountPerformanceRequest(context.Background(), 7, time.Now())
	require.NoError(t, err)
	require.False(t, allowed, "the writing instance publishes protection immediately")
	allowed, err = other.AdmitAccountPerformanceRequest(context.Background(), 7, time.Now())
	require.NoError(t, err)
	require.True(t, allowed, "another instance does not yet know about the new opt-in")
	repo.listErr = errors.New("refresh unavailable")
	_, err = other.Refresh(context.Background())
	require.Error(t, err)
	allowed, err = other.AdmitAccountPerformanceRequest(context.Background(), 7, time.Now())
	require.NoError(t, err)
	require.True(t, allowed, "failed refreshes cannot discover previously unknown opt-ins")
	repo.listErr = nil
	_, err = other.Refresh(context.Background())
	require.NoError(t, err)
	allowed, err = other.AdmitAccountPerformanceRequest(context.Background(), 7, time.Now())
	require.NoError(t, err)
	require.False(t, allowed)
	require.Equal(t, int64(2), repo.admitCalls.Load())
}

func TestAccountMetricOptInRefreshCannotOverwriteConcurrentLocalEnable(t *testing.T) {
	repo := &metricOptInRepo{}
	gate, svc := newMetricOptInTestService(t, repo)
	started, unblock := make(chan struct{}), make(chan struct{})
	repo.listHook = func() ([]*AccountPerformanceMonitor, error) {
		close(started)
		<-unblock
		return nil, nil // Older snapshot predates the config write.
	}
	refreshed := make(chan error, 1)
	go func() { _, err := gate.Refresh(context.Background()); refreshed <- err }()
	<-started
	saved := make(chan error, 1)
	go func() {
		_, err := svc.SaveConfig(context.Background(), 7, 0, accountMetricConfigForTest(), nil, time.Now())
		saved <- err
	}()
	// Ordinary traffic must not wait behind a slow configuration refresh.
	allowed, err := gate.AdmitAccountPerformanceRequest(context.Background(), 9, time.Now())
	require.NoError(t, err)
	require.True(t, allowed)
	close(unblock)
	require.NoError(t, <-refreshed)
	require.NoError(t, <-saved)
	allowed, err = gate.AdmitAccountPerformanceRequest(context.Background(), 7, time.Now())
	require.NoError(t, err)
	require.False(t, allowed)
	require.Equal(t, int64(1), repo.admitCalls.Load())
}
