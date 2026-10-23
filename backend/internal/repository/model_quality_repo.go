package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type modelQualityRepository struct{ db *sql.DB }

func NewModelQualityRepository(db *sql.DB) service.ModelQualityRepository {
	return &modelQualityRepository{db: db}
}

const qualityPlanColumns = `id, owner_id, group_id, name, config_json, enabled, public_visible, interval_minutes, next_run_at, created_at`
const qualityRunColumns = `id, plan_id, target_json, status, verdict, phase, reason, result_json, remote_id, attempts, cancel_requested, lease_token, started_at, finished_at`
const qualityRunSummaryColumns = `id, plan_id, target_json, status, verdict, phase, reason, NULL, '', attempts, cancel_requested, '', started_at, finished_at`

// Keep the source fingerprint in the existing JSON column, outside public DTOs.
type qualityStoredTarget struct {
	service.ModelQualityTarget
	CredentialHash string `json:"_credential_hash,omitempty"`
}

func encodeQualityTarget(run *service.ModelQualityRun) ([]byte, error) {
	return json.Marshal(qualityStoredTarget{run.Target, run.CredentialHash})
}

func qualityNotFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrQualityNotFound
	}
	return err
}

func scanQualityPlan(row scannable) (*service.ModelQualityPlan, error) {
	p := &service.ModelQualityPlan{}
	var cfg string
	if err := row.Scan(&p.ID, &p.OwnerID, &p.GroupID, &p.Name, &cfg, &p.Enabled, &p.PublicVisible, &p.IntervalMinutes, &p.NextRunAt, &p.CreatedAt); err != nil {
		return nil, qualityNotFound(err)
	}
	if err := json.Unmarshal([]byte(cfg), &p.Config); err != nil {
		return nil, err
	}
	return p, nil
}

func scanQualityRun(row scannable) (*service.ModelQualityRun, error) {
	r := &service.ModelQualityRun{}
	var target string
	var result sql.NullString
	if err := row.Scan(&r.ID, &r.PlanID, &target, &r.Status, &r.Verdict, &r.Phase, &r.Reason, &result, &r.RemoteID, &r.Attempts, &r.CancelRequested, &r.LeaseToken, &r.StartedAt, &r.FinishedAt); err != nil {
		return nil, qualityNotFound(err)
	}
	var stored qualityStoredTarget
	if err := json.Unmarshal([]byte(target), &stored); err != nil {
		return nil, err
	}
	r.Target, r.CredentialHash = stored.ModelQualityTarget, stored.CredentialHash
	if result.Valid {
		if err := json.Unmarshal([]byte(result.String), &r.Result); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *modelQualityRepository) CreatePlan(ctx context.Context, p *service.ModelQualityPlan) error {
	cfg, err := json.Marshal(p.Config)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO model_quality_plans (`+qualityPlanColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, p.ID, p.OwnerID, p.GroupID, p.Name, string(cfg), p.Enabled, p.PublicVisible, p.IntervalMinutes, p.NextRunAt, p.CreatedAt)
	return err
}

func (r *modelQualityRepository) GetPlan(ctx context.Context, id string) (*service.ModelQualityPlan, error) {
	return scanQualityPlan(r.db.QueryRowContext(ctx, `SELECT `+qualityPlanColumns+` FROM model_quality_plans WHERE id = ?`, id))
}

func (r *modelQualityRepository) ListPlans(ctx context.Context) ([]*service.ModelQualityPlan, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+qualityPlanColumns+` FROM model_quality_plans ORDER BY created_at DESC, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*service.ModelQualityPlan, 0)
	for rows.Next() {
		p, err := scanQualityPlan(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, p)
	}
	return items, rows.Err()
}

func (r *modelQualityRepository) UpdatePlan(ctx context.Context, p *service.ModelQualityPlan) error {
	cfg, err := json.Marshal(p.Config)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var active sql.NullString
	if err = tx.QueryRowContext(ctx, `SELECT active_run_id FROM model_quality_plans WHERE id = ? FOR UPDATE`, p.ID).Scan(&active); err != nil {
		return qualityNotFound(err)
	}
	if active.Valid {
		return service.ErrQualityBusy
	}
	_, err = tx.ExecContext(ctx, `UPDATE model_quality_plans SET name = ?, config_json = ?, enabled = ?, public_visible = ?, interval_minutes = ?, next_run_at = ? WHERE id = ?`, p.Name, string(cfg), p.Enabled, p.PublicVisible, p.IntervalMinutes, p.NextRunAt, p.ID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *modelQualityRepository) Queue(ctx context.Context, run *service.ModelQualityRun, scheduled bool) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var active sql.NullString
	var enabled bool
	var due time.Time
	var interval int
	err = tx.QueryRowContext(ctx, `SELECT active_run_id, enabled, next_run_at, interval_minutes FROM model_quality_plans WHERE id = ? FOR UPDATE`, run.PlanID).Scan(&active, &enabled, &due, &interval)
	if err != nil {
		return qualityNotFound(err)
	}
	if active.Valid || (scheduled && (!enabled || due.After(run.StartedAt))) {
		return service.ErrQualityBusy
	}
	target, err := encodeQualityTarget(run)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO model_quality_runs (id, plan_id, target_json, status, verdict, reason, lease_until, started_at) VALUES (?, ?, ?, 'queued', 'pending', '', ?, ?)`, run.ID, run.PlanID, string(target), run.StartedAt, run.StartedAt)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE model_quality_plans SET active_run_id = ?, next_run_at = ? WHERE id = ?`, run.ID, run.StartedAt.Add(time.Duration(interval)*time.Minute), run.PlanID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *modelQualityRepository) Claim(ctx context.Context, now time.Time, token string) (*service.ModelQualityRun, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	run, err := scanQualityRun(tx.QueryRowContext(ctx, `SELECT `+qualityRunColumns+` FROM model_quality_runs WHERE finished_at IS NULL AND lease_until <= ? ORDER BY lease_until, started_at LIMIT 1 FOR UPDATE SKIP LOCKED`, now))
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE model_quality_runs SET lease_token = ?, lease_until = ? WHERE id = ?`, token, now.Add(time.Minute), run.ID)
	if err != nil {
		return nil, err
	}
	run.LeaseToken = token
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return run, nil
}

func (r *modelQualityRepository) SaveRun(ctx context.Context, run *service.ModelQualityRun, next time.Time) error {
	target, err := encodeQualityTarget(run)
	if err != nil {
		return err
	}
	var result any
	if run.Result != nil {
		b, err := json.Marshal(run.Result)
		if err != nil {
			return err
		}
		result = string(b)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE model_quality_runs SET status = ?, verdict = ?, phase = ?, reason = ?, result_json = ?, remote_id = ?, attempts = ?, finished_at = ?, lease_until = ?, target_json = ? WHERE id = ? AND lease_token = ? AND finished_at IS NULL`, run.Status, run.Verdict, run.Phase, run.Reason, result, run.RemoteID, run.Attempts, run.FinishedAt, next, string(target), run.ID, run.LeaseToken)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return service.ErrQualityBusy
	}
	if run.FinishedAt != nil {
		_, err = tx.ExecContext(ctx, `UPDATE model_quality_plans SET active_run_id = NULL WHERE id = ? AND active_run_id = ?`, run.PlanID, run.ID)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *modelQualityRepository) RequestCancel(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE model_quality_runs SET cancel_requested = TRUE WHERE id = ? AND finished_at IS NULL`, id)
	return err
}

func (r *modelQualityRepository) GetRun(ctx context.Context, id string) (*service.ModelQualityRun, error) {
	return scanQualityRun(r.db.QueryRowContext(ctx, `SELECT `+qualityRunColumns+` FROM model_quality_runs WHERE id = ?`, id))
}

func (r *modelQualityRepository) History(ctx context.Context, id string, limit, offset int) ([]*service.ModelQualityRun, error) {
	return r.HistoryForPlans(ctx, []string{id}, limit, offset)
}

func (r *modelQualityRepository) HistoryForPlans(ctx context.Context, ids []string, limit, offset int) ([]*service.ModelQualityRun, error) {
	return r.runsForPlans(ctx, ids, limit, offset, qualityRunSummaryColumns)
}

func (r *modelQualityRepository) ResultsForPlans(ctx context.Context, ids []string, limit, offset int) ([]*service.ModelQualityRun, error) {
	return r.runsForPlans(ctx, ids, limit, offset, `id, plan_id, target_json, status, verdict, phase, reason, result_json, '', attempts, cancel_requested, '', started_at, finished_at`)
}

func (r *modelQualityRepository) runsForPlans(ctx context.Context, ids []string, limit, offset int, columns string) ([]*service.ModelQualityRun, error) {
	if len(ids) == 0 {
		return []*service.ModelQualityRun{}, nil
	}
	args := make([]any, 0, len(ids)+2)
	for _, id := range ids {
		args = append(args, id)
	}
	args = append(args, limit, offset)
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	rows, err := r.db.QueryContext(ctx, `SELECT `+columns+` FROM model_quality_runs WHERE plan_id IN (`+placeholders+`) ORDER BY started_at DESC, id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]*service.ModelQualityRun, 0)
	for rows.Next() {
		run, err := scanQualityRun(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, run)
	}
	return items, rows.Err()
}

func (r *modelQualityRepository) Stats(ctx context.Context, id string) (service.ModelQualityStats, error) {
	var s service.ModelQualityStats
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(CASE WHEN verdict = 'normal' THEN 1 ELSE 0 END),0), COALESCE(SUM(CASE WHEN verdict = 'degraded' THEN 1 ELSE 0 END),0), COALESCE(SUM(CASE WHEN verdict = 'error' THEN 1 ELSE 0 END),0), COALESCE(SUM(CASE WHEN verdict = 'unknown' THEN 1 ELSE 0 END),0) FROM model_quality_runs WHERE plan_id = ?`, id).Scan(&s.Total, &s.Passed, &s.Failed, &s.Errors, &s.Unknown)
	return s, err
}
