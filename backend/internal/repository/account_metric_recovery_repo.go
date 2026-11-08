package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const accountPerformanceMonitorColumns = `
account_id, config, revision, paused, pause_generation, paused_at, reason,
trial_started_at, trial_requests, next_trial_at, metrics_since, created_at, updated_at`

type accountMetricRepository struct {
	db    *sql.DB
	mysql bool
}

func NewAccountMetricRepository(db *sql.DB) service.AccountMetricRepository {
	return &accountMetricRepository{db: db, mysql: opsIsMySQLDB(db)}
}

func NewAccountPerformanceRepository(db *sql.DB) service.AccountPerformanceRepository {
	return &accountMetricRepository{db: db, mysql: opsIsMySQLDB(db)}
}

func (r *accountMetricRepository) query(query string) string {
	if r != nil && r.mysql {
		return query
	}
	return opsRebindQuestionPlaceholders(query)
}

type accountMetricRowScanner interface {
	Scan(dest ...any) error
}

func scanAccountPerformanceMonitor(scanner accountMetricRowScanner) (*service.AccountPerformanceMonitor, error) {
	var (
		monitor                   service.AccountPerformanceMonitor
		configBytes               []byte
		pausedAt, trialStartedAt  sql.NullTime
		nextTrialAt, metricsSince sql.NullTime
		createdAt, updatedAt      sql.NullTime
	)
	if err := scanner.Scan(
		&monitor.AccountID,
		&configBytes,
		&monitor.Revision,
		&monitor.Paused,
		&monitor.PauseGeneration,
		&pausedAt,
		&monitor.Reason,
		&trialStartedAt,
		&monitor.TrialRequests,
		&nextTrialAt,
		&metricsSince,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(configBytes, &monitor.Config); err != nil {
		return nil, fmt.Errorf("decode account metric config: %w", err)
	}
	monitor.PausedAt = nullableTimePointer(pausedAt)
	monitor.TrialStartedAt = nullableTimePointer(trialStartedAt)
	monitor.NextTrialAt = nullableTimePointer(nextTrialAt)
	monitor.MetricsSince = nullableTimePointer(metricsSince)
	monitor.CreatedAt = nullableTimePointer(createdAt)
	monitor.UpdatedAt = nullableTimePointer(updatedAt)
	return &monitor, nil
}

func nullableTimePointer(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func metricEventJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func (r *accountMetricRepository) getMonitor(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, accountID int64, forUpdate bool) (*service.AccountPerformanceMonitor, error) {
	query := "SELECT " + accountPerformanceMonitorColumns + " FROM account_performance_monitors WHERE account_id = ?"
	if forUpdate {
		query += " FOR UPDATE"
	}
	monitor, err := scanAccountPerformanceMonitor(queryer.QueryRowContext(ctx, r.query(query), accountID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return monitor, err
}

func (r *accountMetricRepository) GetAccountPerformanceMonitor(ctx context.Context, accountID int64) (*service.AccountPerformanceMonitor, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil account metric repository")
	}
	if accountID <= 0 {
		return nil, fmt.Errorf("account id must be positive")
	}
	monitor, err := r.getMonitor(ctx, r.db, accountID, false)
	if err != nil {
		return nil, err
	}
	if monitor == nil {
		return &service.AccountPerformanceMonitor{AccountID: accountID}, nil
	}
	return monitor, nil
}

func (r *accountMetricRepository) ListAccountPerformanceMonitors(ctx context.Context) ([]*service.AccountPerformanceMonitor, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil account metric repository")
	}
	rows, err := r.db.QueryContext(ctx, "SELECT "+accountPerformanceMonitorColumns+" FROM account_performance_monitors ORDER BY account_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	monitors := make([]*service.AccountPerformanceMonitor, 0)
	for rows.Next() {
		monitor, scanErr := scanAccountPerformanceMonitor(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		monitors = append(monitors, monitor)
	}
	return monitors, rows.Err()
}

func (r *accountMetricRepository) insertEvent(ctx context.Context, tx *sql.Tx, accountID int64, action string, actorID *int64, revision, generation int64, evidence any, now time.Time) error {
	_, err := tx.ExecContext(ctx, r.query(`
INSERT INTO account_performance_events (
  account_id, action, actor_id, revision, pause_generation, evidence, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?)`), accountID, action, nullableInt64(actorID), revision, generation, metricEventJSON(evidence), now.UTC())
	return err
}

func (r *accountMetricRepository) SaveAccountMetricRecoveryConfig(ctx context.Context, accountID, expectedRevision int64, config service.AccountMetricRecoveryConfig, actorID *int64, now time.Time) (*service.AccountPerformanceMonitor, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil account metric repository")
	}
	if accountID <= 0 || expectedRevision < 0 {
		return nil, fmt.Errorf("invalid account metric configuration identity")
	}
	config, err := service.NormalizeAccountMetricRecoveryConfig(config)
	if err != nil {
		return nil, err
	}
	configJSON := metricEventJSON(config)
	now = now.UTC()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := r.getMonitor(ctx, tx, accountID, true)
	if err != nil {
		return nil, err
	}
	if current == nil {
		if expectedRevision != 0 {
			return nil, service.ErrAccountMetricRevisionConflict
		}
		_, err = tx.ExecContext(ctx, r.query(`
INSERT INTO account_performance_monitors (
  account_id, config, revision, paused, pause_generation, reason, trial_requests,
  metrics_since, created_at, updated_at
) VALUES (?, ?, 1, FALSE, 0, '', 0, ?, ?, ?)`), accountID, configJSON, now, now, now)
		if err != nil {
			if isDuplicateSQLRecordError(err) {
				return nil, service.ErrAccountMetricRevisionConflict
			}
			return nil, err
		}
		if err := r.insertEvent(ctx, tx, accountID, "configure", actorID, 1, 0, map[string]any{"enabled": config.Enabled, "config": config}, now); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return r.GetAccountPerformanceMonitor(ctx, accountID)
	}
	if current.Revision != expectedRevision {
		return nil, service.ErrAccountMetricRevisionConflict
	}

	revision := current.Revision + 1
	generation := current.PauseGeneration
	paused := current.Paused
	var pausedAt, trialStartedAt, nextTrialAt, metricsSince any
	var reason string
	var trialRequests int64
	if paused && !config.Enabled {
		paused = false
		generation++
	} else if paused {
		pausedAt = current.PausedAt
		trialRequests = 0
		next := now.Add(time.Duration(config.CooldownSeconds) * time.Second)
		nextTrialAt = next
		reason = current.Reason
	} else {
		metricsSince = now
	}
	_, err = tx.ExecContext(ctx, r.query(`
UPDATE account_performance_monitors SET
  config = ?, revision = ?, paused = ?, pause_generation = ?, paused_at = ?, reason = ?,
  trial_started_at = ?, trial_requests = ?, next_trial_at = ?, metrics_since = ?, updated_at = ?
WHERE account_id = ? AND revision = ?`),
		configJSON, revision, paused, generation, pausedAt, reason, trialStartedAt, trialRequests, nextTrialAt, metricsSince, now,
		accountID, expectedRevision)
	if err != nil {
		return nil, err
	}
	if err := r.insertEvent(ctx, tx, accountID, "configure", actorID, revision, generation, map[string]any{"enabled": config.Enabled, "config": config}, now); err != nil {
		return nil, err
	}
	if current.Paused && !config.Enabled {
		if err := r.insertEvent(ctx, tx, accountID, "release", actorID, revision, generation, map[string]any{"source": "configuration_disabled"}, now); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetAccountPerformanceMonitor(ctx, accountID)
}

func isDuplicateSQLRecordError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate entry") || strings.Contains(message, "duplicate key") || strings.Contains(message, "unique constraint")
}

func (r *accountMetricRepository) ReleaseAccountMetricPause(ctx context.Context, accountID, expectedRevision, expectedPauseGeneration int64, actorID *int64, now time.Time) (*service.AccountPerformanceMonitor, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil account metric repository")
	}
	now = now.UTC()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := r.getMonitor(ctx, tx, accountID, true)
	if err != nil {
		return nil, err
	}
	if current == nil || current.Revision != expectedRevision || current.PauseGeneration != expectedPauseGeneration || !current.Paused {
		return nil, service.ErrAccountMetricPauseConflict
	}
	current.Config.Enabled = false
	configJSON := metricEventJSON(current.Config)
	revision := current.Revision + 1
	generation := current.PauseGeneration + 1
	_, err = tx.ExecContext(ctx, r.query(`
UPDATE account_performance_monitors SET
  config = ?, revision = ?, paused = FALSE, pause_generation = ?, paused_at = NULL,
  reason = '', trial_started_at = NULL, trial_requests = 0, next_trial_at = NULL,
  metrics_since = NULL, updated_at = ?
WHERE account_id = ? AND revision = ? AND pause_generation = ? AND paused = TRUE`),
		configJSON, revision, generation, now, accountID, expectedRevision, expectedPauseGeneration)
	if err != nil {
		return nil, err
	}
	if err := r.insertEvent(ctx, tx, accountID, "release", actorID, revision, generation, map[string]any{"source": "admin"}, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetAccountPerformanceMonitor(ctx, accountID)
}

func (r *accountMetricRepository) ListAccountPerformanceEvents(ctx context.Context, accountID, beforeID int64, limit int) ([]service.AccountPerformanceEvent, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil account metric repository")
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := r.db.QueryContext(ctx, r.query(`
SELECT id, account_id, action, actor_id, revision, pause_generation, evidence, created_at
FROM account_performance_events
WHERE account_id = ? AND (? <= 0 OR id < ?)
ORDER BY id DESC LIMIT ?`), accountID, beforeID, beforeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]service.AccountPerformanceEvent, 0)
	for rows.Next() {
		var event service.AccountPerformanceEvent
		var actorID sql.NullInt64
		var evidence []byte
		if err := rows.Scan(&event.ID, &event.AccountID, &event.Action, &actorID, &event.Revision, &event.PauseGeneration, &evidence, &event.CreatedAt); err != nil {
			return nil, err
		}
		if actorID.Valid {
			value := actorID.Int64
			event.ActorID = &value
		}
		event.Evidence = append(event.Evidence[:0], evidence...)
		events = append(events, event)
	}
	return events, rows.Err()
}

func (r *accountMetricRepository) PauseAccountForMetricBreach(ctx context.Context, accountID int64, token service.AccountMetricEvaluationToken, reason string, evidence []service.AccountMetricRuleEvidence, now time.Time) (bool, error) {
	revision := token.Revision
	if r == nil || r.db == nil {
		return false, fmt.Errorf("nil account metric repository")
	}
	now = now.UTC()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := r.getMonitor(ctx, tx, accountID, true)
	if err != nil {
		return false, err
	}
	if !matchesAccountMetricEvaluation(current, token) || current.Paused || !current.Config.Enabled {
		return false, nil
	}
	generation := current.PauseGeneration + 1
	nextTrialAt := now.Add(time.Duration(current.Config.CooldownSeconds) * time.Second)
	_, err = tx.ExecContext(ctx, r.query(`
UPDATE account_performance_monitors SET paused = TRUE, pause_generation = ?, paused_at = ?,
  reason = ?, trial_started_at = NULL, trial_requests = 0, next_trial_at = ?, metrics_since = NULL,
  updated_at = ?
WHERE account_id = ? AND revision = ? AND pause_generation = ? AND paused = FALSE`),
		generation, now, reason, nextTrialAt, now, accountID, revision, current.PauseGeneration)
	if err != nil {
		return false, err
	}
	if err := r.insertEvent(ctx, tx, accountID, "pause", nil, revision, generation, evidence, now); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (r *accountMetricRepository) RecoverAccountFromMetricTrial(ctx context.Context, accountID int64, token service.AccountMetricEvaluationToken, evidence []service.AccountMetricRuleEvidence, now time.Time) (bool, error) {
	revision := token.Revision
	if r == nil || r.db == nil {
		return false, fmt.Errorf("nil account metric repository")
	}
	now = now.UTC()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := r.getMonitor(ctx, tx, accountID, true)
	if err != nil {
		return false, err
	}
	if !matchesAccountMetricEvaluation(current, token) || !current.Paused || current.TrialStartedAt == nil || !current.Config.AutomaticRecoveryEnabled() {
		return false, nil
	}
	config, err := service.NormalizeAccountMetricRecoveryConfig(current.Config)
	if err != nil {
		return false, err
	}
	if current.TrialRequests < int64(config.RecoverySamples) {
		return false, nil
	}
	generation := current.PauseGeneration + 1
	_, err = tx.ExecContext(ctx, r.query(`
UPDATE account_performance_monitors SET paused = FALSE, pause_generation = ?, paused_at = NULL,
  reason = '', trial_started_at = NULL, trial_requests = 0, next_trial_at = NULL,
  metrics_since = ?, updated_at = ?
WHERE account_id = ? AND revision = ? AND pause_generation = ? AND paused = TRUE`),
		generation, now, now, accountID, revision, current.PauseGeneration)
	if err != nil {
		return false, err
	}
	if err := r.insertEvent(ctx, tx, accountID, "recover", nil, revision, generation, evidence, now); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (r *accountMetricRepository) EndAccountMetricTrial(ctx context.Context, accountID int64, token service.AccountMetricEvaluationToken, outcome string, evidence []service.AccountMetricRuleEvidence, now time.Time) (bool, error) {
	revision := token.Revision
	if r == nil || r.db == nil {
		return false, fmt.Errorf("nil account metric repository")
	}
	now = now.UTC()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := r.getMonitor(ctx, tx, accountID, true)
	if err != nil {
		return false, err
	}
	if !matchesAccountMetricEvaluation(current, token) || !current.Paused || current.TrialStartedAt == nil {
		return false, nil
	}
	config, err := service.NormalizeAccountMetricRecoveryConfig(current.Config)
	if err != nil {
		return false, err
	}
	nextTrialAt := now.Add(time.Duration(config.CooldownSeconds) * time.Second)
	_, err = tx.ExecContext(ctx, r.query(`
UPDATE account_performance_monitors SET trial_started_at = NULL, trial_requests = 0,
  next_trial_at = ?, metrics_since = NULL, updated_at = ?
WHERE account_id = ? AND revision = ? AND pause_generation = ? AND paused = TRUE`),
		nextTrialAt, now, accountID, revision, current.PauseGeneration)
	if err != nil {
		return false, err
	}
	if err := r.insertEvent(ctx, tx, accountID, "trial", nil, revision, current.PauseGeneration, map[string]any{"outcome": outcome, "rules": evidence}, now); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (r *accountMetricRepository) AdmitAccountPerformanceRequest(ctx context.Context, accountID int64, now time.Time) (bool, error) {
	if r == nil || r.db == nil {
		return false, fmt.Errorf("nil account metric repository")
	}
	if accountID <= 0 {
		return false, fmt.Errorf("account id must be positive")
	}
	now = now.UTC()
	// The feature is opt-in. Avoid opening a write transaction for the common
	// unconfigured/unpaused path; only paused accounts need the row lock used by
	// the shared trial rate and budget reservation.
	current, err := r.getMonitor(ctx, r.db, accountID, false)
	if err != nil {
		return false, err
	}
	if current == nil || !current.Paused {
		return true, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err = r.getMonitor(ctx, tx, accountID, true)
	if err != nil {
		return false, err
	}
	// A concurrent release or configuration update may have changed the row
	// between the read and the lock acquisition. Re-evaluate the current state
	// under the lock before deciding whether to admit the request.
	if current == nil || !current.Paused {
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return true, nil
	}
	config, err := service.NormalizeAccountMetricRecoveryConfig(current.Config)
	if err != nil {
		return false, err
	}
	if !config.Enabled || !config.AutomaticRecoveryEnabled() {
		return false, nil
	}
	cooldownUntil := current.NextTrialAt
	if cooldownUntil == nil && current.PausedAt != nil {
		value := current.PausedAt.Add(time.Duration(config.CooldownSeconds) * time.Second)
		cooldownUntil = &value
	}
	if current.TrialStartedAt == nil && cooldownUntil == nil {
		return false, nil
	}
	if cooldownUntil != nil && now.Before(*cooldownUntil) {
		return false, nil
	}
	budget := int64(config.RecoverySamples * 3)
	if current.TrialStartedAt != nil && (now.Sub(*current.TrialStartedAt) >= 10*time.Minute || current.TrialRequests >= budget) {
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return false, nil
	}

	started := current.TrialStartedAt == nil
	trialStartedAt := now
	metricsSince := now
	if !started {
		trialStartedAt = *current.TrialStartedAt
		metricsSince = time.Time{}
	}
	trialRequests := current.TrialRequests + 1
	nextTrialAt := now.Add(5 * time.Second)
	if started {
		_, err = tx.ExecContext(ctx, r.query(`
UPDATE account_performance_monitors SET trial_started_at = ?, trial_requests = ?,
  next_trial_at = ?, metrics_since = ?, updated_at = ?
WHERE account_id = ? AND revision = ? AND pause_generation = ? AND paused = TRUE`),
			trialStartedAt, trialRequests, nextTrialAt, metricsSince, now, accountID, current.Revision, current.PauseGeneration)
	} else {
		_, err = tx.ExecContext(ctx, r.query(`
UPDATE account_performance_monitors SET trial_requests = ?, next_trial_at = ?, updated_at = ?
WHERE account_id = ? AND revision = ? AND pause_generation = ? AND paused = TRUE`),
			trialRequests, nextTrialAt, now, accountID, current.Revision, current.PauseGeneration)
	}
	if err != nil {
		return false, err
	}
	if err := r.insertEvent(ctx, tx, accountID, "trial", nil, current.Revision, current.PauseGeneration, map[string]any{"request": trialRequests, "budget": budget, "started": started}, now); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func matchesAccountMetricEvaluation(current *service.AccountPerformanceMonitor, token service.AccountMetricEvaluationToken) bool {
	if current == nil || current.Revision != token.Revision || current.PauseGeneration != token.PauseGeneration {
		return false
	}
	if current.TrialStartedAt == nil || token.TrialStartedAt == nil {
		return current.TrialStartedAt == nil && token.TrialStartedAt == nil
	}
	return current.TrialStartedAt.Equal(*token.TrialStartedAt)
}
