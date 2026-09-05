//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestMySQLGroupUsageRollupTriggersInvalidateHistoricalChanges(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	client := tx.Client()

	group := mustCreateGroup(t, client, &service.Group{Name: "mysql-rollup-trigger-group"})
	user := mustCreateUser(t, client, &service.User{Email: "mysql-rollup-trigger-user@example.com"})
	account := mustCreateAccount(t, client, &service.Account{Name: "mysql-rollup-trigger-account"})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "mysql-rollup-trigger-key", Name: "rollup"})

	_, err := tx.ExecContext(ctx, `
		UPDATE usage_group_rollup_state
		SET closed_before = '2026-01-01', timezone_name = '+00:00'
		WHERE id = 1
	`)
	require.NoError(t, err)

	createdAt := time.Date(2020, time.January, 2, 8, 0, 0, 0, time.UTC)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO usage_logs (request_id, model, actual_cost, created_at, api_key_id, account_id, group_id, user_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, "mysql-rollup-trigger-request", "test-model", 1.25, createdAt, apiKey.ID, account.ID, group.ID, user.ID)
	require.NoError(t, err)

	var closedBefore string
	require.NoError(t, scanSingleRow(ctx, tx,
		"SELECT DATE_FORMAT(closed_before, '%Y-%m-%d') FROM usage_group_rollup_state WHERE id = 1",
		nil, &closedBefore))
	require.Equal(t, "2020-01-02", closedBefore)

	_, err = tx.ExecContext(ctx, "DELETE FROM usage_logs WHERE request_id = ?", "mysql-rollup-trigger-request")
	require.NoError(t, err)
	require.NoError(t, scanSingleRow(ctx, tx,
		"SELECT DATE_FORMAT(closed_before, '%Y-%m-%d') FROM usage_group_rollup_state WHERE id = 1",
		nil, &closedBefore))
	require.Equal(t, "2020-01-02", closedBefore, "deleting historical usage must keep the invalidated watermark")
}
