package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	dbusagecleanuptask "github.com/Wei-Shaw/sub2api/ent/usagecleanuptask"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type usageCleanupRepository struct {
	client *dbent.Client
	sql    sqlExecutor
}

func NewUsageCleanupRepository(client *dbent.Client, sqlDB *sql.DB) service.UsageCleanupRepository {
	return newUsageCleanupRepositoryWithSQL(client, sqlDB)
}

func newUsageCleanupRepositoryWithSQL(client *dbent.Client, sqlq sqlExecutor) *usageCleanupRepository {
	return &usageCleanupRepository{client: client, sql: sqlq}
}

func (r *usageCleanupRepository) CreateTask(ctx context.Context, task *service.UsageCleanupTask) error {
	if task == nil {
		return nil
	}
	if r.client != nil {
		return r.createTaskWithEnt(ctx, task)
	}
	return r.createTaskWithSQL(ctx, task)
}

func (r *usageCleanupRepository) ListTasks(ctx context.Context, params pagination.PaginationParams) ([]service.UsageCleanupTask, *pagination.PaginationResult, error) {
	if r.client != nil {
		return r.listTasksWithEnt(ctx, params)
	}
	var total int64
	if err := scanSingleRow(ctx, r.sql, "SELECT COUNT(*) FROM usage_cleanup_tasks", nil, &total); err != nil {
		return nil, nil, err
	}
	if total == 0 {
		return []service.UsageCleanupTask{}, paginationResultFromTotal(0, params), nil
	}

	query := `
		SELECT id, status, filters, created_by, deleted_rows, error_message,
			canceled_by, canceled_at,
			started_at, finished_at, created_at, updated_at
		FROM usage_cleanup_tasks
		ORDER BY created_at DESC, id DESC
		LIMIT ? OFFSET ?
	`
	rows, err := r.sql.QueryContext(ctx, query, params.Limit(), params.Offset())
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()

	tasks := make([]service.UsageCleanupTask, 0)
	for rows.Next() {
		var task service.UsageCleanupTask
		var filtersJSON []byte
		var errMsg sql.NullString
		var canceledBy sql.NullInt64
		var canceledAt sql.NullTime
		var startedAt sql.NullTime
		var finishedAt sql.NullTime
		if err := rows.Scan(
			&task.ID,
			&task.Status,
			&filtersJSON,
			&task.CreatedBy,
			&task.DeletedRows,
			&errMsg,
			&canceledBy,
			&canceledAt,
			&startedAt,
			&finishedAt,
			&task.CreatedAt,
			&task.UpdatedAt,
		); err != nil {
			return nil, nil, err
		}
		if err := json.Unmarshal(filtersJSON, &task.Filters); err != nil {
			return nil, nil, fmt.Errorf("parse cleanup filters: %w", err)
		}
		if errMsg.Valid {
			task.ErrorMsg = &errMsg.String
		}
		if canceledBy.Valid {
			v := canceledBy.Int64
			task.CanceledBy = &v
		}
		if canceledAt.Valid {
			task.CanceledAt = &canceledAt.Time
		}
		if startedAt.Valid {
			task.StartedAt = &startedAt.Time
		}
		if finishedAt.Valid {
			task.FinishedAt = &finishedAt.Time
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return tasks, paginationResultFromTotal(total, params), nil
}

func (r *usageCleanupRepository) ClaimNextPendingTask(ctx context.Context, staleRunningAfterSeconds int64) (*service.UsageCleanupTask, error) {
	if staleRunningAfterSeconds <= 0 {
		staleRunningAfterSeconds = 1800
	}
	query := `
		UPDATE usage_cleanup_tasks
		SET status = ?,
			started_at = NOW(),
			finished_at = NULL,
			error_message = NULL,
			updated_at = NOW()
		WHERE (
			status = ?
			OR (
				status = ?
				AND started_at IS NOT NULL
				AND started_at < DATE_SUB(NOW(), INTERVAL ? SECOND)
			)
		)
		ORDER BY created_at ASC
		LIMIT 1
	`
	res, err := r.sql.ExecContext(ctx, query,
		service.UsageCleanupStatusRunning,
		service.UsageCleanupStatusPending,
		service.UsageCleanupStatusRunning,
		staleRunningAfterSeconds,
	)
	if err != nil {
		return nil, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, nil
	}

	selectQuery := `
		SELECT id, status, filters, created_by, deleted_rows, error_message,
			started_at, finished_at, created_at, updated_at
		FROM usage_cleanup_tasks
		WHERE status = ?
		ORDER BY started_at DESC, id DESC
		LIMIT 1
	`
	var task service.UsageCleanupTask
	var filtersJSON []byte
	var errMsg sql.NullString
	var startedAt sql.NullTime
	var finishedAt sql.NullTime
	if err := scanSingleRow(ctx, r.sql, selectQuery, []any{service.UsageCleanupStatusRunning},
		&task.ID,
		&task.Status,
		&filtersJSON,
		&task.CreatedBy,
		&task.DeletedRows,
		&errMsg,
		&startedAt,
		&finishedAt,
		&task.CreatedAt,
		&task.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(filtersJSON, &task.Filters); err != nil {
		return nil, fmt.Errorf("parse cleanup filters: %w", err)
	}
	if errMsg.Valid {
		task.ErrorMsg = &errMsg.String
	}
	if startedAt.Valid {
		task.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		task.FinishedAt = &finishedAt.Time
	}
	return &task, nil
}

func (r *usageCleanupRepository) GetTaskStatus(ctx context.Context, taskID int64) (string, error) {
	if r.client != nil {
		return r.getTaskStatusWithEnt(ctx, taskID)
	}
	var status string
	if err := scanSingleRow(ctx, r.sql, "SELECT status FROM usage_cleanup_tasks WHERE id = ?", []any{taskID}, &status); err != nil {
		return "", err
	}
	return status, nil
}

func (r *usageCleanupRepository) UpdateTaskProgress(ctx context.Context, taskID int64, deletedRows int64) error {
	if r.client != nil {
		return r.updateTaskProgressWithEnt(ctx, taskID, deletedRows)
	}
	query := `
		UPDATE usage_cleanup_tasks
		SET deleted_rows = ?,
			updated_at = NOW()
		WHERE id = ?
	`
	_, err := r.sql.ExecContext(ctx, query, deletedRows, taskID)
	return err
}

func (r *usageCleanupRepository) CancelTask(ctx context.Context, taskID int64, canceledBy int64) (bool, error) {
	if r.client != nil {
		return r.cancelTaskWithEnt(ctx, taskID, canceledBy)
	}
	query := `
		UPDATE usage_cleanup_tasks
		SET status = ?,
			canceled_by = ?,
			canceled_at = NOW(),
			finished_at = NOW(),
			error_message = NULL,
			updated_at = NOW()
		WHERE id = ?
			AND status IN (?, ?)
	`
	res, err := r.sql.ExecContext(ctx, query,
		service.UsageCleanupStatusCanceled,
		canceledBy,
		taskID,
		service.UsageCleanupStatusPending,
		service.UsageCleanupStatusRunning,
	)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (r *usageCleanupRepository) MarkTaskSucceeded(ctx context.Context, taskID int64, deletedRows int64) error {
	if r.client != nil {
		return r.markTaskSucceededWithEnt(ctx, taskID, deletedRows)
	}
	query := `
		UPDATE usage_cleanup_tasks
		SET status = ?,
			deleted_rows = ?,
			finished_at = NOW(),
			updated_at = NOW()
		WHERE id = ?
	`
	_, err := r.sql.ExecContext(ctx, query, service.UsageCleanupStatusSucceeded, deletedRows, taskID)
	return err
}

func (r *usageCleanupRepository) MarkTaskFailed(ctx context.Context, taskID int64, deletedRows int64, errorMsg string) error {
	if r.client != nil {
		return r.markTaskFailedWithEnt(ctx, taskID, deletedRows, errorMsg)
	}
	query := `
		UPDATE usage_cleanup_tasks
		SET status = ?,
			deleted_rows = ?,
			error_message = ?,
			finished_at = NOW(),
			updated_at = NOW()
		WHERE id = ?
	`
	_, err := r.sql.ExecContext(ctx, query, service.UsageCleanupStatusFailed, deletedRows, errorMsg, taskID)
	return err
}

func (r *usageCleanupRepository) DeleteUsageLogsBatch(ctx context.Context, filters service.UsageCleanupFilters, limit int) (int64, error) {
	if filters.StartTime.IsZero() || filters.EndTime.IsZero() {
		return 0, fmt.Errorf("cleanup filters missing time range")
	}
	whereClause, args := buildUsageCleanupWhere(filters)
	if whereClause == "" {
		return 0, fmt.Errorf("cleanup filters missing time range")
	}
	args = append(args, limit)
	if db, ok := r.sql.(*sql.DB); ok {
		return r.deleteUsageLogsBatchWithRollupInvalidation(ctx, db, whereClause, args)
	}
	query := fmt.Sprintf(`
		WITH target AS (
			SELECT id
			FROM usage_logs
			WHERE %s
			ORDER BY created_at ASC, id ASC
			LIMIT ?
		)
		DELETE FROM usage_logs
		WHERE id IN (SELECT id FROM target)
	`, whereClause)

	res, err := r.sql.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	deleted, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return deleted, nil
}

func (r *usageCleanupRepository) deleteUsageLogsBatchWithRollupInvalidation(ctx context.Context, db *sql.DB, whereClause string, args []any) (int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	rollback := func(err error) (int64, error) {
		_ = tx.Rollback()
		return 0, err
	}

	if err := lockGroupUsageRollupState(ctx, tx); err != nil {
		return rollback(err)
	}
	var earliestDeletedAt sql.NullTime
	selectQuery := fmt.Sprintf(`
		SELECT MIN(created_at)
		FROM (
			SELECT created_at
			FROM usage_logs
			WHERE %s
			ORDER BY created_at ASC, id ASC
			LIMIT ?
		) target
	`, whereClause)
	if err := tx.QueryRowContext(ctx, selectQuery, args...).Scan(&earliestDeletedAt); err != nil {
		return rollback(err)
	}
	deleteQuery := fmt.Sprintf(`
		DELETE FROM usage_logs
		WHERE id IN (
			SELECT id FROM (
				SELECT id
				FROM usage_logs
				WHERE %s
				ORDER BY created_at ASC, id ASC
				LIMIT ?
			) target
		)
	`, whereClause)
	result, err := tx.ExecContext(ctx, deleteQuery, args...)
	if err != nil {
		return rollback(err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return rollback(err)
	}

	if deleted > 0 && earliestDeletedAt.Valid {
		if err := invalidateGroupUsageRollupsAt(ctx, tx, earliestDeletedAt.Time); err != nil {
			return rollback(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return deleted, nil
}

func buildUsageCleanupWhere(filters service.UsageCleanupFilters) (string, []any) {
	conditions := make([]string, 0, 8)
	args := make([]any, 0, 8)
	if !filters.StartTime.IsZero() {
		conditions = append(conditions, "created_at >= ?")
		args = append(args, filters.StartTime)
	}
	if !filters.EndTime.IsZero() {
		conditions = append(conditions, "created_at <= ?")
		args = append(args, filters.EndTime)
	}
	if filters.UserID != nil {
		conditions = append(conditions, "user_id = ?")
		args = append(args, *filters.UserID)
	}
	if filters.APIKeyID != nil {
		conditions = append(conditions, "api_key_id = ?")
		args = append(args, *filters.APIKeyID)
	}
	if filters.AccountID != nil {
		conditions = append(conditions, "account_id = ?")
		args = append(args, *filters.AccountID)
	}
	if filters.GroupID != nil {
		conditions = append(conditions, "group_id = ?")
		args = append(args, *filters.GroupID)
	}
	if filters.Model != nil {
		model := strings.TrimSpace(*filters.Model)
		if model != "" {
			conditions = append(conditions, "model = ?")
			args = append(args, model)
		}
	}
	if filters.RequestType != nil {
		condition, conditionArgs := buildRequestTypeFilterCondition(1, *filters.RequestType)
		conditions = append(conditions, condition)
		args = append(args, conditionArgs...)
	} else if filters.Stream != nil {
		conditions = append(conditions, "stream = ?")
		args = append(args, *filters.Stream)
	}
	if filters.BillingType != nil {
		conditions = append(conditions, "billing_type = ?")
		args = append(args, *filters.BillingType)
	}
	return strings.Join(conditions, " AND "), args
}

func (r *usageCleanupRepository) createTaskWithEnt(ctx context.Context, task *service.UsageCleanupTask) error {
	client := clientFromContext(ctx, r.client)
	filtersJSON, err := json.Marshal(task.Filters)
	if err != nil {
		return fmt.Errorf("marshal cleanup filters: %w", err)
	}
	created, err := client.UsageCleanupTask.
		Create().
		SetStatus(task.Status).
		SetFilters(json.RawMessage(filtersJSON)).
		SetCreatedBy(task.CreatedBy).
		SetDeletedRows(task.DeletedRows).
		Save(ctx)
	if err != nil {
		return err
	}
	task.ID = created.ID
	task.CreatedAt = created.CreatedAt
	task.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *usageCleanupRepository) createTaskWithSQL(ctx context.Context, task *service.UsageCleanupTask) error {
	filtersJSON, err := json.Marshal(task.Filters)
	if err != nil {
		return fmt.Errorf("marshal cleanup filters: %w", err)
	}
	query := `
		INSERT INTO usage_cleanup_tasks (
			status,
			filters,
			created_by,
			deleted_rows
		) VALUES (?, ?, ?, ?)
	`
	if _, err := r.sql.ExecContext(ctx, query, task.Status, filtersJSON, task.CreatedBy, task.DeletedRows); err != nil {
		return err
	}
	if err := scanSingleRow(ctx, r.sql,
		"SELECT id, created_at, updated_at FROM usage_cleanup_tasks WHERE created_by = ? ORDER BY id DESC LIMIT 1",
		[]any{task.CreatedBy},
		&task.ID, &task.CreatedAt, &task.UpdatedAt,
	); err != nil {
		return err
	}
	return nil
}

func (r *usageCleanupRepository) listTasksWithEnt(ctx context.Context, params pagination.PaginationParams) ([]service.UsageCleanupTask, *pagination.PaginationResult, error) {
	client := clientFromContext(ctx, r.client)
	query := client.UsageCleanupTask.Query()
	total, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
	}
	if total == 0 {
		return []service.UsageCleanupTask{}, paginationResultFromTotal(0, params), nil
	}
	rows, err := query.
		Order(dbent.Desc(dbusagecleanuptask.FieldCreatedAt), dbent.Desc(dbusagecleanuptask.FieldID)).
		Offset(params.Offset()).
		Limit(params.Limit()).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}
	tasks := make([]service.UsageCleanupTask, 0, len(rows))
	for _, row := range rows {
		task, err := usageCleanupTaskFromEnt(row)
		if err != nil {
			return nil, nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, paginationResultFromTotal(int64(total), params), nil
}

func (r *usageCleanupRepository) getTaskStatusWithEnt(ctx context.Context, taskID int64) (string, error) {
	client := clientFromContext(ctx, r.client)
	task, err := client.UsageCleanupTask.Query().
		Where(dbusagecleanuptask.IDEQ(taskID)).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return "", sql.ErrNoRows
		}
		return "", err
	}
	return task.Status, nil
}

func (r *usageCleanupRepository) updateTaskProgressWithEnt(ctx context.Context, taskID int64, deletedRows int64) error {
	client := clientFromContext(ctx, r.client)
	now := time.Now()
	_, err := client.UsageCleanupTask.Update().
		Where(dbusagecleanuptask.IDEQ(taskID)).
		SetDeletedRows(deletedRows).
		SetUpdatedAt(now).
		Save(ctx)
	return err
}

func (r *usageCleanupRepository) cancelTaskWithEnt(ctx context.Context, taskID int64, canceledBy int64) (bool, error) {
	client := clientFromContext(ctx, r.client)
	now := time.Now()
	affected, err := client.UsageCleanupTask.Update().
		Where(
			dbusagecleanuptask.IDEQ(taskID),
			dbusagecleanuptask.StatusIn(service.UsageCleanupStatusPending, service.UsageCleanupStatusRunning),
		).
		SetStatus(service.UsageCleanupStatusCanceled).
		SetCanceledBy(canceledBy).
		SetCanceledAt(now).
		SetFinishedAt(now).
		ClearErrorMessage().
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (r *usageCleanupRepository) markTaskSucceededWithEnt(ctx context.Context, taskID int64, deletedRows int64) error {
	client := clientFromContext(ctx, r.client)
	now := time.Now()
	_, err := client.UsageCleanupTask.Update().
		Where(dbusagecleanuptask.IDEQ(taskID)).
		SetStatus(service.UsageCleanupStatusSucceeded).
		SetDeletedRows(deletedRows).
		SetFinishedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	return err
}

func (r *usageCleanupRepository) markTaskFailedWithEnt(ctx context.Context, taskID int64, deletedRows int64, errorMsg string) error {
	client := clientFromContext(ctx, r.client)
	now := time.Now()
	_, err := client.UsageCleanupTask.Update().
		Where(dbusagecleanuptask.IDEQ(taskID)).
		SetStatus(service.UsageCleanupStatusFailed).
		SetDeletedRows(deletedRows).
		SetErrorMessage(errorMsg).
		SetFinishedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	return err
}

func usageCleanupTaskFromEnt(row *dbent.UsageCleanupTask) (service.UsageCleanupTask, error) {
	task := service.UsageCleanupTask{
		ID:          row.ID,
		Status:      row.Status,
		CreatedBy:   row.CreatedBy,
		DeletedRows: row.DeletedRows,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
	if len(row.Filters) > 0 {
		if err := json.Unmarshal(row.Filters, &task.Filters); err != nil {
			return service.UsageCleanupTask{}, fmt.Errorf("parse cleanup filters: %w", err)
		}
	}
	if row.ErrorMessage != nil {
		task.ErrorMsg = row.ErrorMessage
	}
	if row.CanceledBy != nil {
		task.CanceledBy = row.CanceledBy
	}
	if row.CanceledAt != nil {
		task.CanceledAt = row.CanceledAt
	}
	if row.StartedAt != nil {
		task.StartedAt = row.StartedAt
	}
	if row.FinishedAt != nil {
		task.FinishedAt = row.FinishedAt
	}
	return task, nil
}
