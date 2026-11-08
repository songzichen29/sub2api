package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type accountMetricAdmissionSnapshot struct {
	required map[int64]struct{}
}

// AccountMetricAdmissionService caches only opt-in membership, never an allow
// decision for a monitored account. The latter always comes from persisted
// pause/trial state. Configuration changes on other instances become visible
// on the next successful background refresh.
type AccountMetricAdmissionService struct {
	repo AccountPerformanceRepository
	// Serialize refreshes with admin writes so an older list cannot overwrite
	// a just-enabled local account. Request reads do not take this lock.
	updateMu sync.Mutex
	snapshot atomic.Pointer[accountMetricAdmissionSnapshot]
}

func NewAccountMetricAdmissionService(repo AccountPerformanceRepository) *AccountMetricAdmissionService {
	return &AccountMetricAdmissionService{repo: repo}
}

func (s *AccountMetricAdmissionService) Refresh(ctx context.Context) ([]*AccountPerformanceMonitor, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("account metric admission repository is unavailable")
	}
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	monitors, err := s.repo.ListAccountPerformanceMonitors(ctx)
	if err != nil {
		// Keep the last known protected set when a refresh fails.
		return nil, err
	}
	next := &accountMetricAdmissionSnapshot{required: make(map[int64]struct{})}
	for _, monitor := range monitors {
		if monitor != nil && monitor.AccountID > 0 && (monitor.Config.Enabled || monitor.Paused) {
			next.required[monitor.AccountID] = struct{}{}
		}
	}
	s.snapshot.Store(next)
	return monitors, nil
}

func (s *AccountMetricAdmissionService) AdmitAccountPerformanceRequest(ctx context.Context, accountID int64, now time.Time) (bool, error) {
	if s == nil || s.repo == nil {
		return false, fmt.Errorf("account metric admission repository is unavailable")
	}
	if accountID <= 0 {
		return false, fmt.Errorf("account id must be positive")
	}
	current := s.snapshot.Load()
	if current == nil {
		// Production wiring initializes this before the server accepts traffic.
		return false, fmt.Errorf("account metric admission has not been initialized")
	}
	if _, required := current.required[accountID]; !required {
		return true, nil
	}
	return s.repo.AdmitAccountPerformanceRequest(ctx, accountID, now)
}

func (s *AccountMetricAdmissionService) applyConfigUpdate(accountID int64, enabling bool, write func() (*AccountPerformanceMonitor, error)) (*AccountPerformanceMonitor, error) {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	if enabling {
		// Publish protection before enabling in SQL. An uncertain commit/readback
		// must not leave this instance bypassing a newly enabled monitor.
		s.setRequired(accountID, true)
	}
	monitor, err := write()
	if err != nil {
		return nil, err
	}
	if monitor == nil || monitor.AccountID != accountID {
		return nil, fmt.Errorf("account metric configuration returned an invalid monitor")
	}
	s.setRequired(accountID, monitor.Config.Enabled || monitor.Paused)
	return monitor, nil
}

// setRequired is called with updateMu held. Snapshots remain immutable for
// concurrent request readers; disabled/unconfigured accounts do no SQL work.
func (s *AccountMetricAdmissionService) setRequired(accountID int64, required bool) {
	current := s.snapshot.Load()
	if current == nil {
		return // Do not convert incomplete bootstrap state into a valid snapshot.
	}
	next := &accountMetricAdmissionSnapshot{required: make(map[int64]struct{}, len(current.required)+1)}
	for id := range current.required {
		next.required[id] = struct{}{}
	}
	if required {
		next.required[accountID] = struct{}{}
	} else {
		delete(next.required, accountID)
	}
	s.snapshot.Store(next)
}
