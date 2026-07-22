package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestAuthCacheInvalidationOutboxRepository_ClaimUsesLeaseAndSkipLocked(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	created := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectQuery("(?s)claimed_at < DATE_SUB.*LIMIT \\?.*FOR UPDATE SKIP LOCKED").
		WithArgs(int64(30), 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "cache_key", "attempts", "delivery_stage", "created_at"}).
			AddRow(int64(4), strings.Repeat("a", 64), 2, 1, created))
	mock.ExpectExec("(?s)UPDATE auth_cache_invalidation_outbox.*claimed_by = \\?.*id IN \\(\\?\\)").
		WithArgs("worker-a", int64(4)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	repo := NewAuthCacheInvalidationOutboxRepository(db)
	events, err := repo.Claim(context.Background(), "worker-a", 100, 30*time.Second)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, int64(4), events[0].ID)
	require.Equal(t, 1, events[0].Stage)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAuthCacheInvalidationOutboxRepository_ClaimIsBoundedByDefault(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectBegin()
	mock.ExpectQuery("(?s)FROM auth_cache_invalidation_outbox.*LIMIT \\?.*SKIP LOCKED").
		WithArgs(int64(30), 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "cache_key", "attempts", "delivery_stage", "created_at"}))
	mock.ExpectCommit()
	repo := NewAuthCacheInvalidationOutboxRepository(db)
	_, err = repo.Claim(context.Background(), "worker", 0, 0)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAuthCacheInvalidationOutboxRepository_ClaimOwnershipTransitions(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := NewAuthCacheInvalidationOutboxRepository(db)

	next := time.Now().UTC().Add(time.Minute)
	mock.ExpectExec("UPDATE auth_cache_invalidation_outbox").
		WithArgs(next, int64(1), "worker").
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.ScheduleSecondPass(context.Background(), 1, "worker", next))

	retryAt := next.Add(time.Minute)
	mock.ExpectExec("UPDATE auth_cache_invalidation_outbox").
		WithArgs(retryAt, "publish failed", int64(2), "worker").
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.RetryClaimed(context.Background(), 2, "worker", retryAt, "publish failed"))

	mock.ExpectExec("DELETE FROM auth_cache_invalidation_outbox").
		WithArgs(int64(3), "worker").
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.DeleteClaimed(context.Background(), 3, "worker"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAuthCacheInvalidationOutboxRepository_RejectsLostClaim(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectExec("DELETE FROM auth_cache_invalidation_outbox").
		WithArgs(int64(3), "old-worker").
		WillReturnResult(sqlmock.NewResult(0, 0))
	repo := NewAuthCacheInvalidationOutboxRepository(db)
	err = repo.DeleteClaimed(context.Background(), 3, "old-worker")
	require.ErrorContains(t, err, "no longer owned")
}

func TestAuthCacheInvalidationOutboxRepository_StatsExposeDurableLagAndFailures(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	oldest := time.Now().UTC().Add(-time.Minute)
	mock.ExpectQuery("(?s)SELECT COUNT\\(\\*\\), MIN\\(created_at\\), COALESCE\\(MAX\\(attempts\\), 0\\)").
		WillReturnRows(sqlmock.NewRows([]string{"count", "min", "max", "last_error"}).AddRow(5, oldest, 7, "redis down"))
	repo := NewAuthCacheInvalidationOutboxRepository(db)
	stats, err := repo.Stats(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(5), stats.Pending)
	require.Equal(t, 7, stats.MaxAttempts)
	require.Equal(t, "redis down", stats.LastError)
	require.NotNil(t, stats.OldestCreatedAt)
}

func TestAuthCacheInvalidationMigration_SecurityCoverageAndNoPlaintextPayload(t *testing.T) {
	content, err := migrations.MySQLFS.ReadFile("061_auth_cache_invalidation_outbox.sql")
	require.NoError(t, err)
	sqlText := string(content)
	for _, required := range []string{
		"SHA2(OLD.`key`, 256)",
		"OLD.`key`", "OLD.`status`", "OLD.`deleted_at`", "OLD.`user_id`", "OLD.`group_id`",
		"OLD.`ip_whitelist`", "OLD.`ip_blacklist`", "OLD.`expires_at`",
		"trg_users_auth_cache_invalidation", "trg_groups_auth_cache_invalidation",
		"trg_user_allowed_groups_auth_cache", "FOR EACH ROW",
		"delivery_stage", "claimed_at", "available_at",
	} {
		require.Contains(t, sqlText, required)
	}
	require.NotContains(t, sqlText, "OLD.`quota_used`")
	require.NotContains(t, sqlText, "OLD.`last_used_at`")

	plaintext := "sk-plaintext-must-not-be-stored"
	sum := sha256.Sum256([]byte(plaintext))
	require.Len(t, hex.EncodeToString(sum[:]), 64)
	require.NotContains(t, sqlText, plaintext)
}
