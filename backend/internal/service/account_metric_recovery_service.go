package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const (
	accountMetricEvaluationInterval = 30 * time.Second
	accountMetricRollingWindow      = time.Hour
	accountMetricTrialWindow        = 10 * time.Minute
	accountMetricTrialLogGrace      = 30 * time.Second
)

type AccountMetricRecoveryService struct {
	repo      AccountPerformanceRepository
	admission *AccountMetricAdmissionService
	ops       *OpsService
	interval  time.Duration
	stopCh    chan struct{}
	stopOnce  sync.Once
	wg        sync.WaitGroup
}

func NewAccountMetricRecoveryService(repo AccountPerformanceRepository, ops *OpsService, interval time.Duration, admission ...*AccountMetricAdmissionService) *AccountMetricRecoveryService {
	if interval <= 0 {
		interval = accountMetricEvaluationInterval
	}
	svc := &AccountMetricRecoveryService{repo: repo, ops: ops, interval: interval, stopCh: make(chan struct{})}
	if len(admission) > 0 {
		svc.admission = admission[0]
	}
	return svc
}

func (s *AccountMetricRecoveryService) Start() {
	if s == nil || s.repo == nil {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		run := func(now time.Time) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := s.RunEvaluation(ctx, now.UTC()); err != nil {
				slog.Warn("account_metric_recovery_evaluation_failed", "error", err)
			}
		}
		run(time.Now())
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-s.stopCh:
				return
			case now := <-ticker.C:
				run(now)
			}
		}
	}()
}

func (s *AccountMetricRecoveryService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() { close(s.stopCh) })
	s.wg.Wait()
}

func (s *AccountMetricRecoveryService) RunEvaluation(ctx context.Context, now time.Time) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("account metric recovery repository is unavailable")
	}
	var monitors []*AccountPerformanceMonitor
	var err error
	if s.admission != nil {
		// Enablement must synchronize even when telemetry/evaluation is off.
		monitors, err = s.admission.Refresh(ctx)
	} else {
		monitors, err = s.repo.ListAccountPerformanceMonitors(ctx)
	}
	if err != nil {
		return fmt.Errorf("list account metric monitors: %w", err)
	}
	if s.ops == nil || !s.ops.IsMonitoringEnabled(ctx) {
		return nil
	}
	var evaluationErrors []error
	for _, monitor := range monitors {
		if monitor == nil || !monitor.Config.Enabled {
			continue
		}
		if err := s.evaluateMonitor(ctx, monitor, now.UTC()); err != nil {
			evaluationErrors = append(evaluationErrors, fmt.Errorf("account %d: %w", monitor.AccountID, err))
		}
	}
	return errors.Join(evaluationErrors...)
}

func (s *AccountMetricRecoveryService) evaluateMonitor(ctx context.Context, monitor *AccountPerformanceMonitor, now time.Time) error {
	config, err := NormalizeAccountMetricRecoveryConfig(monitor.Config)
	if err != nil {
		return err
	}
	monitor.Config = config
	if !monitor.Paused {
		start := now.Add(-accountMetricRollingWindow)
		if monitor.MetricsSince != nil && monitor.MetricsSince.After(start) {
			start = *monitor.MetricsSince
		}
		if !start.Before(now) {
			return nil
		}
		snapshot, err := s.repo.GetAccountMetricSnapshot(ctx, monitor.AccountID, start, now)
		if err != nil {
			return err
		}
		evaluation, err := EvaluateAccountMetricPause(monitor.Config, snapshot)
		if err != nil {
			return err
		}
		if !evaluation.Triggered {
			return nil
		}
		reason := AccountMetricPauseReason(evaluation.Evidence)
		_, err = s.repo.PauseAccountForMetricBreach(ctx, monitor.AccountID, monitor.Revision, reason, evaluation.Evidence, now)
		return err
	}

	if monitor.TrialStartedAt == nil {
		return nil
	}
	start := *monitor.TrialStartedAt
	if start.Before(now.Add(-accountMetricRollingWindow)) {
		start = now.Add(-accountMetricRollingWindow)
	}
	if !start.Before(now) {
		return nil
	}
	snapshot, err := s.repo.GetAccountMetricSnapshot(ctx, monitor.AccountID, start, now)
	if err != nil {
		return err
	}
	failure, err := EvaluateAccountMetricTrialFailure(monitor.Config, snapshot)
	if err != nil {
		return err
	}
	if failure.Triggered {
		_, err = s.repo.EndAccountMetricTrial(ctx, monitor.AccountID, monitor.Revision, "pause_threshold_breached", failure.Evidence, now)
		return err
	}
	recovery, err := EvaluateAccountMetricRecovery(monitor.Config, snapshot)
	if err != nil {
		return err
	}
	if recovery.Triggered && monitor.TrialRequests >= int64(monitor.Config.RecoverySamples) {
		_, err = s.repo.RecoverAccountFromMetricTrial(ctx, monitor.AccountID, monitor.Revision, recovery.Evidence, now)
		return err
	}
	budgetExhausted := monitor.TrialRequests >= int64(monitor.Config.RecoverySamples*3)
	trialExpired := now.Sub(*monitor.TrialStartedAt) >= accountMetricTrialWindow
	if !budgetExhausted && !trialExpired {
		return nil
	}
	settleAfter := monitor.TrialStartedAt.Add(accountMetricTrialWindow)
	if monitor.NextTrialAt != nil {
		settleAfter = *monitor.NextTrialAt
	}
	if trialExpired {
		settleAfter = monitor.TrialStartedAt.Add(accountMetricTrialWindow)
	}
	if now.Before(settleAfter.Add(accountMetricTrialLogGrace)) {
		return nil
	}
	outcome := "budget_exhausted"
	if trialExpired {
		outcome = "trial_window_expired"
	}
	_, err = s.repo.EndAccountMetricTrial(ctx, monitor.AccountID, monitor.Revision, outcome, recovery.Evidence, now)
	return err
}

func (s *AccountMetricRecoveryService) GetStatus(ctx context.Context, accountID int64, now time.Time) (*AccountPerformanceStatus, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("account metric recovery repository is unavailable")
	}
	monitor, err := s.repo.GetAccountPerformanceMonitor(ctx, accountID)
	if err != nil {
		return nil, err
	}
	status := &AccountPerformanceStatus{Monitor: monitor}
	if s.ops == nil || !s.ops.IsMonitoringEnabled(ctx) {
		return status, nil
	}
	start := now.UTC().Add(-accountMetricRollingWindow)
	if monitor.MetricsSince != nil && monitor.MetricsSince.After(start) {
		start = *monitor.MetricsSince
	}
	if monitor.Paused && monitor.TrialStartedAt != nil && monitor.TrialStartedAt.After(start) {
		start = *monitor.TrialStartedAt
	}
	if !start.Before(now.UTC()) {
		return status, nil
	}
	status.Metrics, err = s.repo.GetAccountMetricSnapshot(ctx, accountID, start, now.UTC())
	if err != nil {
		return nil, err
	}
	status.MetricsAvailable = true
	return status, nil
}

func (s *AccountMetricRecoveryService) SaveConfig(ctx context.Context, accountID, expectedRevision int64, config AccountMetricRecoveryConfig, actorID *int64, now time.Time) (*AccountPerformanceMonitor, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("account metric recovery repository is unavailable")
	}
	config, err := NormalizeAccountMetricRecoveryConfig(config)
	if err != nil {
		return nil, err
	}
	write := func() (*AccountPerformanceMonitor, error) {
		return s.repo.SaveAccountMetricRecoveryConfig(ctx, accountID, expectedRevision, config, actorID, now)
	}
	if s.admission != nil {
		return s.admission.applyConfigUpdate(accountID, config.Enabled, write)
	}
	return write()
}

func (s *AccountMetricRecoveryService) Release(ctx context.Context, accountID, expectedRevision, expectedPauseGeneration int64, actorID *int64, now time.Time) (*AccountPerformanceMonitor, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("account metric recovery repository is unavailable")
	}
	write := func() (*AccountPerformanceMonitor, error) {
		return s.repo.ReleaseAccountMetricPause(ctx, accountID, expectedRevision, expectedPauseGeneration, actorID, now)
	}
	if s.admission != nil {
		return s.admission.applyConfigUpdate(accountID, false, write)
	}
	return write()
}

func (s *AccountMetricRecoveryService) ListEvents(ctx context.Context, accountID, beforeID int64, limit int) ([]AccountPerformanceEvent, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("account metric recovery repository is unavailable")
	}
	return s.repo.ListAccountPerformanceEvents(ctx, accountID, beforeID, limit)
}
