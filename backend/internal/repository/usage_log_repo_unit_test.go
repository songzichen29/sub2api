//go:build unit

package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

type usageLogSQLRecorder struct {
	query string
	args  []any
}

func (r *usageLogSQLRecorder) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	r.query = query
	r.args = append([]any(nil), args...)
	return usageLogStaticResult{}, nil
}

func (r *usageLogSQLRecorder) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	panic("unexpected QueryContext call")
}

type usageLogStaticResult struct{}

func (usageLogStaticResult) LastInsertId() (int64, error) { return 1, nil }
func (usageLogStaticResult) RowsAffected() (int64, error) { return 1, nil }

type usageLogRetryRecorder struct {
	errors   []error
	attempts int
}

func (r *usageLogRetryRecorder) ExecContext(_ context.Context, _ string, _ ...any) (sql.Result, error) {
	r.attempts++
	if r.attempts <= len(r.errors) && r.errors[r.attempts-1] != nil {
		return nil, r.errors[r.attempts-1]
	}
	return usageLogStaticResult{}, nil
}

func (r *usageLogRetryRecorder) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	panic("unexpected QueryContext call")
}

func usageLogInsertColumns(t *testing.T, query string) []string {
	t.Helper()
	const prefix = "INSERT INTO usage_logs ("
	start := strings.Index(query, prefix)
	require.NotEqual(t, -1, start)
	columnsStart := start + len(prefix)
	columnsEnd := strings.Index(query[columnsStart:], ")")
	require.NotEqual(t, -1, columnsEnd)
	columns := strings.Split(query[columnsStart:columnsStart+columnsEnd], ",")
	for index := range columns {
		columns[index] = strings.TrimSpace(columns[index])
	}
	return columns
}

func TestSafeDateFormat(t *testing.T) {
	tests := []struct {
		name        string
		granularity string
		expected    string
	}{
		// 合法值
		{"hour", "hour", "%Y-%m-%d %H:00"},
		{"day", "day", "%Y-%m-%d"},
		{"week", "week", "%x-%v"},
		{"month", "month", "%Y-%m"},

		// 非法值回退到默认
		{"空字符串", "", "%Y-%m-%d"},
		{"未知粒度 year", "year", "%Y-%m-%d"},
		{"未知粒度 minute", "minute", "%Y-%m-%d"},

		// 恶意字符串
		{"SQL 注入尝试", "'; DROP TABLE users; --", "%Y-%m-%d"},
		{"带引号", "day'", "%Y-%m-%d"},
		{"带括号", "day)", "%Y-%m-%d"},
		{"Unicode", "日", "%Y-%m-%d"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := safeMySQLDateFormat(tc.granularity)
			require.Equal(t, tc.expected, got, "safeMySQLDateFormat(%q)", tc.granularity)
		})
	}
}

func TestBuildMySQLUsageLogInsertQuery_UsesInsertIgnore(t *testing.T) {
	query := buildMySQLUsageLogInsertQuery(`
		INSERT INTO usage_logs (request_id, api_key_id) VALUES ($1, $2)
		ON CONFLICT (request_id, api_key_id) DO NOTHING
		RETURNING id, created_at
	`)

	require.Contains(t, query, "INSERT IGNORE INTO usage_logs")
	require.Equal(t, 2, strings.Count(query, "?"))
	require.NotContains(t, strings.ToUpper(query), "ON CONFLICT")
	require.NotContains(t, strings.ToUpper(query), "RETURNING")
}

func TestBuildMySQLUsageLogBestEffortInsertQuery_UsesSingleMultiRowStatement(t *testing.T) {
	prepared := prepareUsageLogInsert(&service.UsageLog{
		UserID:    1,
		APIKeyID:  2,
		AccountID: 3,
		RequestID: "req-mysql-batch",
		Model:     "gpt-5",
	})

	query, args := buildMySQLUsageLogBestEffortInsertQuery([]usageLogInsertPrepared{prepared, prepared})
	normalizedQuery := strings.Replace(query, "INSERT IGNORE INTO", "INSERT INTO", 1)

	require.True(t, strings.HasPrefix(query, "INSERT IGNORE INTO usage_logs ("))
	require.NotContains(t, strings.ToUpper(query), "ON CONFLICT")
	require.NotContains(t, strings.ToUpper(query), "WITH INPUT")
	require.Len(t, usageLogInsertColumns(t, normalizedQuery), len(prepared.args))
	require.Len(t, usageLogInsertColumnNames, len(usageLogInsertArgTypes))
	require.Equal(t, len(prepared.args)*2, strings.Count(query, "?"))
	require.Len(t, args, len(prepared.args)*2)
}

func TestExecUsageLogInsertNoResult_RetriesTransientMySQLWriteErrors(t *testing.T) {
	prepared := prepareUsageLogInsert(&service.UsageLog{
		UserID:    1,
		APIKeyID:  2,
		AccountID: 3,
		RequestID: "req-mysql-retry",
		Model:     "gpt-5",
	})

	t.Run("deadlock then success", func(t *testing.T) {
		recorder := &usageLogRetryRecorder{errors: []error{
			&mysql.MySQLError{Number: 1213, Message: "deadlock"},
			&mysql.MySQLError{Number: 1213, Message: "deadlock"},
		}}

		require.NoError(t, execUsageLogInsertNoResult(context.Background(), recorder, prepared, true))
		require.Equal(t, usageLogMySQLWriteMaxAttempts, recorder.attempts)
	})

	t.Run("lock wait timeout then success", func(t *testing.T) {
		recorder := &usageLogRetryRecorder{errors: []error{
			&mysql.MySQLError{Number: 1205, Message: "lock wait timeout"},
		}}

		require.NoError(t, execUsageLogInsertNoResult(context.Background(), recorder, prepared, true))
		require.Equal(t, 2, recorder.attempts)
	})

	t.Run("non transient error is not retried", func(t *testing.T) {
		wantErr := errors.New("invalid column")
		recorder := &usageLogRetryRecorder{errors: []error{wantErr}}

		err := execUsageLogInsertNoResult(context.Background(), recorder, prepared, true)
		require.ErrorIs(t, err, wantErr)
		require.Equal(t, 1, recorder.attempts)
	})

	t.Run("deadlock stops at retry limit", func(t *testing.T) {
		wantErr := &mysql.MySQLError{Number: 1213, Message: "deadlock"}
		recorder := &usageLogRetryRecorder{errors: []error{wantErr, wantErr, wantErr}}

		err := execUsageLogInsertNoResult(context.Background(), recorder, prepared, true)
		require.ErrorIs(t, err, wantErr)
		require.Equal(t, usageLogMySQLWriteMaxAttempts, recorder.attempts)
	})
}

func TestUsageLogSQLShapeMatchesPreparedArguments(t *testing.T) {
	prepared := prepareUsageLogInsert(&service.UsageLog{
		UserID:      1,
		APIKeyID:    2,
		AccountID:   3,
		RequestID:   "req-sql-shape",
		Model:       "gpt-5",
		CreatedAt:   time.Now().UTC(),
		InputTokens: 10,
	})

	t.Run("single insert", func(t *testing.T) {
		recorder := &usageLogSQLRecorder{}
		err := execUsageLogInsertNoResult(context.Background(), recorder, prepared)
		require.NoError(t, err)
		require.Len(t, usageLogInsertColumns(t, recorder.query), len(prepared.args))
		require.Equal(t, len(prepared.args), strings.Count(recorder.query, "$"))
		require.Len(t, recorder.args, len(prepared.args))
	})

	t.Run("multi insert", func(t *testing.T) {
		query, args := buildUsageLogBestEffortInsertQuery([]usageLogInsertPrepared{prepared, prepared})
		require.Len(t, usageLogInsertColumns(t, query), len(prepared.args))
		require.Equal(t, len(prepared.args)*2, strings.Count(query, "$"))
		require.Len(t, args, len(prepared.args)*2)
	})

	t.Run("select scan", func(t *testing.T) {
		selectColumns := strings.Split(usageLogSelectColumns, ",")
		require.Len(t, selectColumns, len(prepared.args)+1, "SELECT includes id plus every inserted column")
		require.Contains(t, selectColumns, " image_input_tokens")
		require.Contains(t, selectColumns, " image_input_cost")
		require.Contains(t, selectColumns, " long_context_billing_applied")
	})
}
