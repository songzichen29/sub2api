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

const qualityPlanColumns = `id, owner_id, group_id, name, config_json, enabled, public_visible, interval_minutes, next_run_at, created_at, active_run_id`
const qualityRunColumns = `id, plan_id, config_id, batch_id, target_json, status, verdict, phase, reason, result_json, remote_id, attempts, cancel_requested, lease_token, started_at, execution_started_at, finished_at`
const qualityRunSummaryColumns = `id, plan_id, config_id, batch_id, target_json, status, verdict, phase, reason, NULL, '', attempts, cancel_requested, '', started_at, execution_started_at, finished_at`

type qualityStoredConfig struct {
	service.ModelQualityConfig
	Configs []service.ModelQualityConfig `json:"configs,omitempty"`
}

// Keep the source fingerprint in the existing JSON column, outside public DTOs.
type qualityStoredTarget struct {
	service.ModelQualityTarget
	CredentialHash string `json:"_credential_hash,omitempty"`
	JudgeKeyID     int64  `json:"_judge_key_id,omitempty"`
}

func encodeQualityTarget(run *service.ModelQualityRun) ([]byte, error) {
	return json.Marshal(qualityStoredTarget{ModelQualityTarget: run.Target, CredentialHash: run.CredentialHash, JudgeKeyID: run.JudgeKeyID})
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
	var active sql.NullString
	if err := row.Scan(&p.ID, &p.OwnerID, &p.GroupID, &p.Name, &cfg, &p.Enabled, &p.PublicVisible, &p.IntervalMinutes, &p.NextRunAt, &p.CreatedAt, &active); err != nil {
		return nil, qualityNotFound(err)
	}
	p.ActiveRunID = active.String
	var stored qualityStoredConfig
	if err := json.Unmarshal([]byte(cfg), &stored); err != nil {
		return nil, err
	}
	p.Config = stored.ModelQualityConfig
	p.Configs = stored.Configs
	if len(p.Configs) == 0 {
		p.Configs = []service.ModelQualityConfig{p.Config}
	}
	for i := range p.Configs {
		if p.Configs[i].ID == "" {
			p.Configs[i].ID = service.QualityConfigID(p.Configs[i].Model, p.Configs[i].Benchmark, p.Configs[i].ReasoningEffort, p.Configs[i].ServiceTier)
		}
	}
	p.Config = p.Configs[0]
	return p, nil
}

func scanQualityRun(row scannable) (*service.ModelQualityRun, error) {
	r := &service.ModelQualityRun{}
	var target string
	var result sql.NullString
	if err := row.Scan(&r.ID, &r.PlanID, &r.ConfigID, &r.BatchID, &target, &r.Status, &r.Verdict, &r.Phase, &r.Reason, &result, &r.RemoteID, &r.Attempts, &r.CancelRequested, &r.LeaseToken, &r.StartedAt, &r.ExecutionStartedAt, &r.FinishedAt); err != nil {
		return nil, qualityNotFound(err)
	}
	var stored qualityStoredTarget
	if err := json.Unmarshal([]byte(target), &stored); err != nil {
		return nil, err
	}
	r.Target, r.CredentialHash = stored.ModelQualityTarget, stored.CredentialHash
	r.JudgeKeyID = stored.JudgeKeyID
	if r.ConfigID == "" {
		r.ConfigID = r.Target.ConfigID
		if r.ConfigID == "" {
			r.ConfigID = service.QualityConfigID(r.Target.Model, r.Target.Benchmark, r.Target.ReasoningEffort, r.Target.ServiceTier)
		}
	}
	if r.BatchID == "" {
		r.BatchID = r.ID
	}
	r.Target.ConfigID = r.ConfigID
	if result.Valid {
		if err := json.Unmarshal([]byte(result.String), &r.Result); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *modelQualityRepository) CreatePlan(ctx context.Context, p *service.ModelQualityPlan) error {
	cfg, err := json.Marshal(qualityStoredConfig{ModelQualityConfig: p.Config, Configs: p.Configs})
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO model_quality_plans (`+qualityPlanColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`, p.ID, p.OwnerID, p.GroupID, p.Name, string(cfg), p.Enabled, p.PublicVisible, p.IntervalMinutes, p.NextRunAt, p.CreatedAt)
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
	cfg, err := json.Marshal(qualityStoredConfig{ModelQualityConfig: p.Config, Configs: p.Configs})
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

func (r *modelQualityRepository) DeletePlan(ctx context.Context, id string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var active sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT active_run_id FROM model_quality_plans WHERE id = ? FOR UPDATE`, id).Scan(&active); err != nil {
		return qualityNotFound(err)
	}
	if active.Valid {
		return service.ErrQualityBusy
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM model_quality_runs WHERE plan_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM model_quality_plans WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *modelQualityRepository) Queue(ctx context.Context, run *service.ModelQualityRun, scheduled bool) error {
	if run.BatchID == "" {
		run.BatchID = run.ID
	}
	return r.QueueBatch(ctx, []*service.ModelQualityRun{run}, scheduled)
}

func (r *modelQualityRepository) QueueBatch(ctx context.Context, runs []*service.ModelQualityRun, scheduled bool) error {
	if len(runs) == 0 || len(runs) > 100 {
		return service.ErrQualityInvalid
	}
	run := runs[0]
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var active sql.NullString
	var enabled bool
	var due time.Time
	var interval int
	var configJSON string
	err = tx.QueryRowContext(ctx, `SELECT active_run_id, enabled, next_run_at, interval_minutes, config_json FROM model_quality_plans WHERE id = ? FOR UPDATE`, run.PlanID).Scan(&active, &enabled, &due, &interval, &configJSON)
	if err != nil {
		return qualityNotFound(err)
	}
	if active.Valid || (scheduled && (!enabled || due.After(run.StartedAt))) {
		return service.ErrQualityBusy
	}
	// A plan may have been edited after the service read it but before this
	// lock. Reject a stale round rather than submitting obsolete dimensions.
	var config qualityStoredConfig
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return err
	}
	configs := config.Configs
	if len(configs) == 0 {
		configs = []service.ModelQualityConfig{config.ModelQualityConfig}
	}
	if len(configs) != len(runs) {
		return service.ErrQualityBusy
	}
	for i, item := range runs {
		c, t := configs[i], item.Target
		if c.MonitorID != t.MonitorID || c.Model != t.Model || c.Benchmark != t.Benchmark || c.ReasoningEffort != t.ReasoningEffort || c.ServiceTier != t.ServiceTier {
			return service.ErrQualityBusy
		}
	}
	for _, item := range runs {
		if item.PlanID != run.PlanID || item.BatchID != run.BatchID {
			return service.ErrQualityInvalid
		}
		target, err := encodeQualityTarget(item)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO model_quality_runs (id, plan_id, config_id, batch_id, model, benchmark, target_json, status, verdict, reason, lease_until, started_at) VALUES (?, ?, ?, ?, ?, ?, ?, 'queued', 'pending', '', ?, ?)`, item.ID, item.PlanID, item.ConfigID, item.BatchID, item.Target.Model, item.Target.Benchmark, string(target), item.StartedAt, item.StartedAt)
		if err != nil {
			return err
		}
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
	_, err = tx.ExecContext(ctx, `UPDATE model_quality_runs SET lease_token = ?, lease_until = ?, execution_started_at = COALESCE(execution_started_at, ?) WHERE id = ?`, token, now.Add(time.Minute), now, run.ID)
	if err != nil {
		return nil, err
	}
	if run.ExecutionStartedAt == nil {
		run.ExecutionStartedAt = &now
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
	// Serialize batch completion with queue/edit/delete. Claims only lock run
	// rows, so every transaction needing both locks takes the plan lock first.
	if run.FinishedAt != nil {
		var active sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT active_run_id FROM model_quality_plans WHERE id = ? FOR UPDATE`, run.PlanID).Scan(&active); err != nil {
			return qualityNotFound(err)
		}
	}
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
		var active sql.NullString
		err = tx.QueryRowContext(ctx, `SELECT id FROM model_quality_runs WHERE plan_id = ? AND finished_at IS NULL ORDER BY started_at, id LIMIT 1 FOR UPDATE`, run.PlanID).Scan(&active)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if errors.Is(err, sql.ErrNoRows) {
			active = sql.NullString{}
		}
		_, err = tx.ExecContext(ctx, `UPDATE model_quality_plans SET active_run_id = ? WHERE id = ?`, active, run.PlanID)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *modelQualityRepository) RequestCancel(ctx context.Context, id string) error {
	run, err := r.GetRun(ctx, id)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `UPDATE model_quality_runs SET cancel_requested = TRUE WHERE plan_id = ? AND batch_id = ? AND finished_at IS NULL`, run.PlanID, run.BatchID)
	return err
}

func (r *modelQualityRepository) GetRun(ctx context.Context, id string) (*service.ModelQualityRun, error) {
	return scanQualityRun(r.db.QueryRowContext(ctx, `SELECT `+qualityRunColumns+` FROM model_quality_runs WHERE id = ?`, id))
}

func (r *modelQualityRepository) History(ctx context.Context, id string, limit, offset int) ([]*service.ModelQualityRun, error) {
	return r.HistoryForPlans(ctx, []string{id}, limit, offset)
}

func (r *modelQualityRepository) HistoryForPlans(ctx context.Context, ids []string, limit, offset int) ([]*service.ModelQualityRun, error) {
	return r.runsForPlans(ctx, ids, "", limit, offset, qualityRunSummaryColumns)
}

func (r *modelQualityRepository) ResultsForPlans(ctx context.Context, ids []string, benchmark string, limit, offset int) ([]*service.ModelQualityRun, error) {
	return r.runsForPlans(ctx, ids, benchmark, limit, offset, qualityRunColumns)
}

func qualityResultScope(planID, batchID, excludeBatchID string) (string, []any) {
	where := `plan_id = ? AND finished_at IS NOT NULL`
	args := []any{planID}
	if batchID != "" {
		where += ` AND batch_id = ?`
		args = append(args, batchID)
	}
	if excludeBatchID != "" {
		where += ` AND batch_id <> ?`
		args = append(args, excludeBatchID)
	}
	return where, args
}

func (r *modelQualityRepository) ResultDimensions(ctx context.Context, planID, batchID, excludeBatchID string) ([]service.ModelQualityDimension, error) {
	where, args := qualityResultScope(planID, batchID, excludeBatchID)
	rows, err := r.db.QueryContext(ctx, `SELECT DISTINCT model, benchmark FROM model_quality_runs WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []service.ModelQualityDimension{}
	for rows.Next() {
		var d service.ModelQualityDimension
		if err := rows.Scan(&d.Model, &d.Benchmark); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *modelQualityRepository) PlanResults(ctx context.Context, planID, batchID, excludeBatchID, model, benchmark string, limit, offset int) ([]*service.ModelQualityRun, error) {
	where, args := qualityResultScope(planID, batchID, excludeBatchID)
	where += ` AND model = ? AND benchmark = ?`
	args = append(args, model, benchmark, limit, offset)
	return r.queryRuns(ctx, `SELECT `+qualityRunColumns+` FROM model_quality_runs WHERE `+where+` ORDER BY started_at DESC, id DESC LIMIT ? OFFSET ?`, args...)
}

func (r *modelQualityRepository) Batches(ctx context.Context, planID string, limit int) ([]service.ModelQualityBatch, error) {
	// Publish a block once every combination in this round has finished.
	// Any degraded result keeps the round yellow; transport failures remain
	// unknown rather than being counted as degradation.
	rows, err := r.db.QueryContext(ctx, `SELECT batch_id, MIN(started_at), MAX(finished_at), COUNT(*),
		CASE WHEN SUM(CASE WHEN verdict = 'degraded' THEN 1 ELSE 0 END) > 0 THEN 'degraded'
		WHEN SUM(CASE WHEN verdict = 'error' THEN 1 ELSE 0 END) > 0 THEN 'error'
		WHEN SUM(CASE WHEN verdict <> 'normal' THEN 1 ELSE 0 END) > 0 THEN 'unknown'
		ELSE 'normal' END,
		SUM(CASE WHEN verdict = 'normal' THEN 1 ELSE 0 END),
		SUM(CASE WHEN verdict = 'degraded' THEN 1 ELSE 0 END),
		SUM(CASE WHEN verdict = 'error' THEN 1 ELSE 0 END),
		SUM(CASE WHEN verdict NOT IN ('normal', 'degraded', 'error') THEN 1 ELSE 0 END)
		FROM model_quality_runs WHERE plan_id = ? GROUP BY batch_id
		HAVING COUNT(*) = COUNT(finished_at) ORDER BY MIN(started_at) DESC, batch_id DESC LIMIT ?`, planID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []service.ModelQualityBatch{}
	for rows.Next() {
		b := service.ModelQualityBatch{PlanID: planID}
		if err := rows.Scan(&b.ID, &b.StartedAt, &b.FinishedAt, &b.Count, &b.Verdict, &b.Stats.Passed, &b.Stats.Failed, &b.Stats.Errors, &b.Stats.Unknown); err != nil {
			return nil, err
		}
		b.Stats.Total = int64(b.Count)
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *modelQualityRepository) runsForPlans(ctx context.Context, ids []string, benchmark string, limit, offset int, columns string) ([]*service.ModelQualityRun, error) {
	if len(ids) == 0 {
		return []*service.ModelQualityRun{}, nil
	}
	args := make([]any, 0, len(ids)+3)
	for _, id := range ids {
		args = append(args, id)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	where := `plan_id IN (` + placeholders + `) AND finished_at IS NOT NULL`
	if benchmark != "" {
		where += ` AND benchmark = ?`
		args = append(args, benchmark)
	}
	args = append(args, limit, offset)
	return r.queryRuns(ctx, `SELECT `+columns+` FROM model_quality_runs WHERE `+where+` ORDER BY started_at DESC, id DESC LIMIT ? OFFSET ?`, args...)
}

func (r *modelQualityRepository) queryRuns(ctx context.Context, query string, args ...any) ([]*service.ModelQualityRun, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
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
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(CASE WHEN verdict = 'normal' THEN 1 ELSE 0 END),0), COALESCE(SUM(CASE WHEN verdict = 'degraded' THEN 1 ELSE 0 END),0), COALESCE(SUM(CASE WHEN verdict = 'error' THEN 1 ELSE 0 END),0), COALESCE(SUM(CASE WHEN verdict NOT IN ('normal', 'degraded', 'error') THEN 1 ELSE 0 END),0) FROM model_quality_runs WHERE plan_id = ? AND finished_at IS NOT NULL`, id).Scan(&s.Total, &s.Passed, &s.Failed, &s.Errors, &s.Unknown)
	return s, err
}

// Group only the last 24 hours, independent of the clicked batch and page.
// Target snapshots keep legacy rows without config_id separated by effort/tier.
func (r *modelQualityRepository) RecentStats(ctx context.Context, planID string, now time.Time) (map[string]service.ModelQualityStats, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT config_id, target_json, COUNT(*),
		SUM(CASE WHEN verdict = 'normal' THEN 1 ELSE 0 END),
		SUM(CASE WHEN verdict = 'degraded' THEN 1 ELSE 0 END),
		SUM(CASE WHEN verdict = 'error' THEN 1 ELSE 0 END),
		SUM(CASE WHEN verdict NOT IN ('normal', 'degraded', 'error') THEN 1 ELSE 0 END)
		FROM model_quality_runs WHERE plan_id = ? AND finished_at >= ? AND finished_at <= ?
		GROUP BY config_id, target_json`, planID, now.Add(-24*time.Hour), now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]service.ModelQualityStats{}
	for rows.Next() {
		var id, target string
		var stats service.ModelQualityStats
		if err := rows.Scan(&id, &target, &stats.Total, &stats.Passed, &stats.Failed, &stats.Errors, &stats.Unknown); err != nil {
			return nil, err
		}
		if id == "" {
			var snapshot service.ModelQualityTarget
			if err := json.Unmarshal([]byte(target), &snapshot); err != nil {
				return nil, err
			}
			id = snapshot.ConfigID
			if id == "" {
				id = service.QualityConfigID(snapshot.Model, snapshot.Benchmark, snapshot.ReasoningEffort, snapshot.ServiceTier)
			}
		}
		total := out[id]
		total.Add(stats)
		out[id] = total
	}
	return out, rows.Err()
}
