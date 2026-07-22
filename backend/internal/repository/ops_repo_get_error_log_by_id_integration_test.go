//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGetErrorLogByID_APIKeyPrefixAndUpstreamStatus(t *testing.T) {
	ctx := context.Background()
	_, _ = integrationDB.ExecContext(ctx, "DELETE FROM ops_error_logs")

	repo := NewOpsRepository(integrationDB).(*opsRepository)

	// ── Case 1: 带 deleted_key_owner 信息的记录 ──────────────────────────────
	owner := mustCreateUser(t, integrationEntClient, &service.User{
		Email: "deleted-key-owner-" + time.Now().Format("150405.000000000") + "@example.com",
	})

	var insertedID int64
	res, err := integrationDB.ExecContext(ctx, `
		INSERT INTO ops_error_logs (
			error_phase, error_type, severity, status_code, created_at,
			attempted_key_prefix, deleted_key_owner_user_id, deleted_key_name
		) VALUES (
			'auth', 'INVALID_API_KEY', 'error', 401, NOW(),
			'sk-test-abc', ?, 'my-deleted-key'
		)`,
		owner.ID,
	)
	require.NoError(t, err)
	insertedID, err = res.LastInsertId()
	require.NoError(t, err)
	require.Positive(t, insertedID)

	detail, err := repo.GetErrorLogByID(ctx, insertedID)
	require.NoError(t, err)
	require.NotNil(t, detail)

	require.Equal(t, "sk-test-abc", detail.AttemptedKeyPrefix)
	require.NotNil(t, detail.DeletedKeyOwnerUserID)
	require.Equal(t, owner.ID, *detail.DeletedKeyOwnerUserID)
	require.Equal(t, owner.Email, detail.DeletedKeyOwnerEmail)
	require.Equal(t, "my-deleted-key", detail.DeletedKeyName)

	// ── Case 2: 新列全为 NULL 的普通错误记录 ──────────────────────────────────
	var plainID int64
	res, err = integrationDB.ExecContext(ctx, `
		INSERT INTO ops_error_logs (
			error_phase, error_type, severity, status_code, created_at
		) VALUES (
			'upstream', 'upstream_error', 'error', 500, NOW()
		)`,
	)
	require.NoError(t, err)
	plainID, err = res.LastInsertId()
	require.NoError(t, err)

	plain, err := repo.GetErrorLogByID(ctx, plainID)
	require.NoError(t, err)
	require.Empty(t, plain.APIKeyPrefix)

	require.Empty(t, plain.AttemptedKeyPrefix, "no prefix for plain error")
	require.Nil(t, plain.DeletedKeyOwnerUserID, "no owner for plain error")
	require.Empty(t, plain.DeletedKeyOwnerEmail, "no owner email for plain error")
	require.Empty(t, plain.DeletedKeyName, "no key name for plain error")
	require.Empty(t, plain.APIKeyPrefix, "no api key prefix for plain error")

	// ── Case 3: 有效(未删除)key 报错,经 InsertErrorLog 快照 api_key_prefix ──────
	// 走真实 InsertErrorLog 写入路径(覆盖新列 + MySQL ? 占位符),再 GetErrorLogByID 读回。
	validID, err := repo.InsertErrorLog(ctx, &service.OpsInsertErrorLogInput{
		ErrorPhase:   "request",
		ErrorType:    "api_error",
		Severity:     "error",
		StatusCode:   402,
		CreatedAt:    time.Now(),
		APIKeyPrefix: "sk-valid",
	})
	require.NoError(t, err)

	valid, err := repo.GetErrorLogByID(ctx, validID)
	require.NoError(t, err)
	require.Equal(t, "sk-valid", valid.APIKeyPrefix)
	require.Empty(t, valid.AttemptedKeyPrefix, "attempted prefix and api key prefix are mutually exclusive")
	require.Nil(t, valid.DeletedKeyOwnerUserID, "valid key error has no deleted owner")
}
