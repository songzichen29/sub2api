//go:build unit

package repository

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
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

func usageLogInsertColumns(t *testing.T, query string) []string {
	t.Helper()
	const prefix = "INSERT INTO usage_logs ("
	start := strings.Index(query, prefix)
	require.NotEqual(t, -1, start)
	columnsStart := start + len(prefix)
	columnsEnd := strings.Index(query[columnsStart:], ") VALUES")
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

func TestBuildUsageLogBatchInsertQuery_UsesOnDuplicateKeyUpdate(t *testing.T) {
	log := &service.UsageLog{
		UserID:       1,
		APIKeyID:     2,
		AccountID:    3,
		RequestID:    "req-batch-no-update",
		Model:        "gpt-5",
		InputTokens:  10,
		OutputTokens: 5,
		TotalCost:    1.2,
		ActualCost:   1.2,
		CreatedAt:    time.Now().UTC(),
	}
	prepared := prepareUsageLogInsert(log)

	query := buildUsageLogMultiInsertQuery([]usageLogInsertPrepared{prepared})

	require.Contains(t, query, "ON DUPLICATE KEY UPDATE id = id")
	require.NotContains(t, strings.ToUpper(query), "ON CONFLICT")
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
		_, err := execUsageLogInsert(context.Background(), recorder, prepared)
		require.NoError(t, err)
		require.Len(t, usageLogInsertColumns(t, recorder.query), len(prepared.args))
		require.Equal(t, len(prepared.args), strings.Count(recorder.query, "?"))
		require.Len(t, recorder.args, len(prepared.args))
	})

	t.Run("multi insert", func(t *testing.T) {
		query, args := buildUsageLogBestEffortInsertQuery([]usageLogInsertPrepared{prepared, prepared})
		require.Len(t, usageLogInsertColumns(t, query), len(prepared.args))
		require.Equal(t, len(prepared.args)*2, strings.Count(query, "?"))
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
