package repository

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountRepository_SetTempUnschedulable_NoRowsAffectedDoesNotWriteOutbox(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(0)}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)
	until := time.Now().Add(10 * time.Minute)

	err := repo.SetTempUnschedulable(context.Background(), 42, until, "retry")
	require.NoError(t, err)
	require.Len(t, exec.execQueries, 1)
	require.Contains(t, exec.execQueries[0], "UPDATE accounts")
	require.NotContains(t, strings.Join(exec.execQueries, "\n"), "scheduler_outbox")
}

func TestAccountRepository_ResetQuotaUsedAndClearRateLimitCooldown_NoRowsAffectedReturnsNotFoundWithoutOutbox(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(0)}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)

	err := repo.ResetQuotaUsedAndClearRateLimitCooldown(context.Background(), 42)

	require.ErrorIs(t, err, service.ErrAccountNotFound)
	require.Len(t, exec.execQueries, 1)
	require.Contains(t, exec.execQueries[0], "UPDATE accounts")
	require.NotContains(t, strings.Join(exec.execQueries, "\n"), "scheduler_outbox")
}

func TestAccountRepository_UpdateOpenAIOAuthCredentialsIfUnchanged_UsesExactAttemptStateAndAtomicOutbox(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(1)}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)
	proxyID := int64(29)

	applied, err := repo.UpdateOpenAIOAuthCredentialsIfUnchanged(
		context.Background(),
		42,
		map[string]any{"refresh_token": "attempted", "_token_version": int64(9)},
		&proxyID,
		map[string]any{"refresh_token": "rotated", "_token_version": int64(10)},
	)

	require.NoError(t, err)
	require.True(t, applied)
	require.Len(t, exec.execQueries, 1, "credential update and outbox insertion must share one SQL statement")
	normalized := normalizeSQLWhitespace(exec.execQueries[0])
	require.Contains(t, normalized, "WITH updated AS ( UPDATE accounts AS a SET credentials = $1::jsonb")
	require.Contains(t, normalized, "a.deleted_at IS NULL")
	require.Contains(t, normalized, "a.platform = $3")
	require.Contains(t, normalized, "a.type = $4")
	require.Contains(t, normalized, "a.credentials = $5::jsonb")
	require.Contains(t, normalized, "a.proxy_id IS NOT DISTINCT FROM $6")
	require.Contains(t, normalized, "($3 <> 'openai' OR a.parent_account_id IS NULL)")
	require.Contains(t, normalized, "RETURNING a.id ) INSERT INTO scheduler_outbox")
	require.Contains(t, normalized, "SELECT $7, updated.id, NULL, NULL FROM updated")
	require.Len(t, exec.execArgs[0], 7)
	arg0, ok := exec.execArgs[0][0].(string)
	require.True(t, ok)
	require.JSONEq(t, `{"refresh_token":"rotated","_token_version":10}`, arg0)
	require.Equal(t, int64(42), exec.execArgs[0][1])
	require.Equal(t, service.PlatformOpenAI, exec.execArgs[0][2])
	require.Equal(t, service.AccountTypeOAuth, exec.execArgs[0][3])
	arg4, ok := exec.execArgs[0][4].(string)
	require.True(t, ok)
	require.JSONEq(t, `{"refresh_token":"attempted","_token_version":9}`, arg4)
	require.Equal(t, &proxyID, exec.execArgs[0][5])
	require.Equal(t, service.SchedulerOutboxEventAccountChanged, exec.execArgs[0][6])
}

func TestAccountRepository_UpdateOpenAIOAuthCredentialsIfUnchanged_NoMatchDoesNotApply(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(0)}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)

	applied, err := repo.UpdateOpenAIOAuthCredentialsIfUnchanged(
		context.Background(), 42, map[string]any{"refresh_token": "attempted"}, nil,
		map[string]any{"refresh_token": "rotated"},
	)

	require.NoError(t, err)
	require.False(t, applied)
	require.Len(t, exec.execQueries, 1)
	require.Nil(t, exec.execArgs[0][5], "a nil proxy must match only a database NULL proxy")
	require.Contains(t, normalizeSQLWhitespace(exec.execQueries[0]), "SELECT $7, updated.id, NULL, NULL FROM updated",
		"a CAS miss must not insert an independent invalidation event")
}

func TestAccountRepository_UpdateOpenAIOAuthCredentialsIfUnchanged_OutboxFailureDoesNotReportSuccess(t *testing.T) {
	outboxErr := errors.New("scheduler outbox insert failed")
	exec := &recordingSQLExecutor{err: outboxErr}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)

	applied, err := repo.UpdateOpenAIOAuthCredentialsIfUnchanged(
		context.Background(), 42, map[string]any{"refresh_token": "attempted"}, nil,
		map[string]any{"refresh_token": "rotated"},
	)

	require.ErrorIs(t, err, outboxErr)
	require.False(t, applied)
	require.Len(t, exec.execQueries, 1)
	require.Contains(t, normalizeSQLWhitespace(exec.execQueries[0]), "INSERT INTO scheduler_outbox")
}

func TestAccountRepository_ListOAuthRefreshCandidates_SQLFilter(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	var capturedSQL string
	mock.ExpectQuery("SELECT id").
		WillReturnRows(sqlmock.NewRows([]string{"id"})).
		WillDelayFor(0)

	repo := newAccountRepositoryWithSQL(nil, captureQuerySQL{db: db, captured: &capturedSQL}, nil)

	accounts, err := repo.ListOAuthRefreshCandidates(context.Background())
	require.NoError(t, err)
	require.Empty(t, accounts)

	normalized := normalizeSQLWhitespace(capturedSQL)
	require.Contains(t, normalized, "deleted_at IS NULL")
	require.NotContains(t, normalized, "schedulable = TRUE",
		"paused (schedulable=false, status=active) accounts must remain OAuth refresh candidates: excluding them lets their stored access_token expire and the usage-window probe then reports a false 'needs re-auth'; permanent rejection is already covered by status = 'active' and refresh failures are bounded by the retry-cooldown exclusion")
	require.Contains(t, normalized, "status = 'active'")
	// setup-token 的 access_token 同为 8h 短期令牌，必须与 oauth 一起纳入后台刷新候选
	require.Contains(t, normalized, "type IN ('oauth', 'setup-token')")
	require.Contains(t, normalized, "platform IN ('anthropic', 'openai', 'gemini', 'antigravity')")
	require.Contains(t, normalized, "JSON_UNQUOTE(JSON_EXTRACT(credentials, '$.refresh_token'))")
	require.Contains(t, normalized, "TRIM(COALESCE")
	require.Contains(t, normalized, "temp_unschedulable_until > NOW(6)")
	require.Contains(t, normalized, "temp_unschedulable_reason LIKE 'token refresh retry exhausted:%'")
	require.Contains(t, normalized, "COALESCE(temp_unschedulable_until > NOW(6), FALSE)")
	require.Contains(t, normalized, "COALESCE(temp_unschedulable_reason LIKE 'token refresh retry exhausted:%', FALSE)")
	require.Contains(t, normalized, "AND NOT (",
		"must negate the fully COALESCE-normalized blocked predicate so NULL temp_unschedulable_until rows remain refresh candidates")
	require.Contains(t, normalized, "ORDER BY priority ASC, id ASC")
	require.NotContains(t, normalized, "credentials->>")
	require.NotContains(t, normalized, "credentials ?")
	require.NoError(t, mock.ExpectationsWereMet())
}

type captureQuerySQL struct {
	db       *sql.DB
	captured *string
}

func (c captureQuerySQL) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return c.db.ExecContext(ctx, query, args...)
}

func (c captureQuerySQL) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if c.captured != nil {
		*c.captured = query
	}
	return c.db.QueryContext(ctx, query, args...)
}

func normalizeSQLWhitespace(sql string) string {
	return strings.Join(regexp.MustCompile(`\s+`).Split(strings.TrimSpace(sql), -1), " ")
}

type rowsAffectedResult int64

func (r rowsAffectedResult) LastInsertId() (int64, error) { return 0, nil }
func (r rowsAffectedResult) RowsAffected() (int64, error) { return int64(r), nil }

type recordingSQLExecutor struct {
	result      sql.Result
	err         error
	execQueries []string
	execArgs    [][]any
}

func (e *recordingSQLExecutor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	e.execQueries = append(e.execQueries, query)
	e.execArgs = append(e.execArgs, append([]any(nil), args...))
	if e.err != nil {
		return nil, e.err
	}
	return e.result, nil
}

func (e *recordingSQLExecutor) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return nil, sql.ErrNoRows
}
