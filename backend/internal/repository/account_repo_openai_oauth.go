package repository

import (
	"context"
	"encoding/json"
	"time"

	"entgo.io/ent/dialect"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// UpdateOpenAIOAuthCredentialsIfUnchanged 仅保存本次刷新对应的凭据，并原子写入调度失效事件。
func (r *accountRepository) UpdateOpenAIOAuthCredentialsIfUnchanged(
	ctx context.Context,
	id int64,
	expectedCredentials map[string]any,
	expectedProxyID *int64,
	credentials map[string]any,
) (bool, error) {
	expectedJSON, err := json.Marshal(normalizeJSONMap(expectedCredentials))
	if err != nil {
		return false, err
	}
	credentialsJSON, err := json.Marshal(normalizeJSONMap(credentials))
	if err != nil {
		return false, err
	}
	if accountRepositoryDialect(r) == dialect.MySQL {
		return r.updateOpenAIOAuthCredentialsMySQL(ctx, id, string(expectedJSON), expectedProxyID, string(credentialsJSON))
	}
	result, err := r.sql.ExecContext(ctx, `
		WITH updated AS (
		UPDATE accounts AS a
		SET credentials = $1::jsonb, updated_at = NOW()
		WHERE a.id = $2
			AND a.deleted_at IS NULL
			AND a.platform = $3
			AND a.type = $4
			AND a.credentials = $5::jsonb
			AND a.proxy_id IS NOT DISTINCT FROM $6
			AND ($3 <> 'openai' OR a.parent_account_id IS NULL)
		RETURNING a.id
		)
		INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)
		SELECT $7, updated.id, NULL, NULL FROM updated
	`, string(credentialsJSON), id, service.PlatformOpenAI, service.AccountTypeOAuth,
		string(expectedJSON), expectedProxyID, service.SchedulerOutboxEventAccountChanged)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected == 0 {
		return false, nil
	}
	r.syncSchedulerAccountSnapshotDetached(ctx, id)
	return true, nil
}

func (r *accountRepository) updateOpenAIOAuthCredentialsMySQL(ctx context.Context, id int64, expectedJSON string, expectedProxyID *int64, credentialsJSON string) (bool, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.Client().ExecContext(ctx, `
		UPDATE accounts AS a
		SET credentials = CAST(? AS JSON), updated_at = NOW()
		WHERE a.id = ?
			AND a.deleted_at IS NULL
			AND a.platform = ?
			AND a.type = ?
			AND a.credentials = CAST(? AS JSON)
			AND a.proxy_id <=> ?
			AND a.parent_account_id IS NULL
	`, credentialsJSON, id, service.PlatformOpenAI, service.AccountTypeOAuth, expectedJSON, expectedProxyID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected == 0 {
		return false, nil
	}
	if err := enqueueSchedulerOutbox(ctx, tx.Client(), service.SchedulerOutboxEventAccountChanged, &id, nil, nil); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	r.syncSchedulerAccountSnapshotDetached(ctx, id)
	return true, nil
}

func (r *accountRepository) syncSchedulerAccountSnapshotDetached(ctx context.Context, accountID int64) {
	propagationCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	r.syncSchedulerAccountSnapshot(propagationCtx, accountID)
}
