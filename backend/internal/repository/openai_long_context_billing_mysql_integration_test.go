//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestMySQLMigration050DefaultsOpenAILongContextBilling(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)

	migrationSQL, err := dbmigrations.MySQLFS.ReadFile("050_default_openai_long_context_billing.sql")
	require.NoError(t, err)

	insert := func(name, extra string, parentID any, quotaDimension any) int64 {
		t.Helper()
		result, err := tx.ExecContext(ctx, `
			INSERT INTO accounts (name, platform, type, extra, parent_account_id, quota_dimension)
			VALUES (?, ?, ?, ?, ?, ?)
		`, name, service.PlatformOpenAI, service.AccountTypeOAuth, extra, parentID, quotaDimension)
		require.NoError(t, err)
		id, err := result.LastInsertId()
		require.NoError(t, err)
		return id
	}

	ordinaryID := insert("mysql-migration-050-ordinary", `{}`, nil, nil)
	parentID := insert("mysql-migration-050-parent", `{"openai_long_context_billing_enabled": false}`, nil, nil)
	shadowID := insert("mysql-migration-050-shadow", `{}`, parentID, "spark")
	malformedID := insert("mysql-migration-050-malformed", `{"openai_long_context_billing_enabled": "false"}`, nil, nil)

	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err, "migration must be idempotent")

	readValue := func(id int64) (string, string) {
		t.Helper()
		var value, valueType string
		require.NoError(t, scanSingleRow(ctx, tx, `
			SELECT COALESCE(JSON_UNQUOTE(JSON_EXTRACT(extra, '$.openai_long_context_billing_enabled')), ''),
			       COALESCE(JSON_TYPE(JSON_EXTRACT(extra, '$.openai_long_context_billing_enabled')), '')
			FROM accounts WHERE id = ?
		`, []any{id}, &value, &valueType))
		return value, valueType
	}

	for _, id := range []int64{ordinaryID, parentID, shadowID, malformedID} {
		value, valueType := readValue(id)
		require.Equal(t, "false", value, "account %d must default to false", id)
		require.Equal(t, "BOOLEAN", valueType, "account %d must store a JSON boolean", id)
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE accounts
		SET extra = JSON_SET(extra, '$.openai_long_context_billing_enabled', true)
		WHERE id = ?
	`, parentID)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(migrationSQL))
	require.NoError(t, err)
	value, valueType := readValue(shadowID)
	require.Equal(t, "true", value)
	require.Equal(t, "BOOLEAN", valueType)
}
