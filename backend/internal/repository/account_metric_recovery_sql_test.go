package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// Execute the actual aggregate queries against rows, not precomputed SQLMock
// answers. SQLite covers the shared query semantics; native dialect migration
// and locking tests still require PostgreSQL/MySQL.
func TestAccountMetricSnapshotSQLExcludesFailedUsageAndInternalRequests(t *testing.T) {
	for _, mysql := range []bool{false, true} {
		name := "numbered placeholders"
		if mysql {
			name = "question placeholders"
		}
		t.Run(name, func(t *testing.T) {
			db, err := sql.Open("sqlite", ":memory:")
			require.NoError(t, err)
			t.Cleanup(func() { _ = db.Close() })
			db.SetMaxOpenConns(1)
			verifyAccountMetricSnapshotSQL(t, db, mysql)
		})
	}
}

func verifyAccountMetricSnapshotSQL(t *testing.T, db *sql.DB, mysql bool) {
	t.Helper()
	ctx := context.Background()
	repo := &accountMetricRepository{db: db, mysql: mysql}
	for _, statement := range []string{
		// Every caller owns an isolated in-memory or uniquely named database.
		// Use ordinary tables like production: MySQL temporary tables cannot be
		// referenced more than once through a CTE in the same statement.
		`CREATE TABLE usage_logs (
		 id BIGINT, output_tokens BIGINT DEFAULT 0, image_input_tokens BIGINT DEFAULT 0,
		 stream BOOLEAN DEFAULT TRUE, openai_ws_mode BOOLEAN DEFAULT FALSE,
		 account_id BIGINT, api_key_id BIGINT, request_id VARCHAR(64), request_type INT,
		 actual_cost DOUBLE PRECISION, input_tokens BIGINT, cache_creation_tokens BIGINT,
		 cache_read_tokens BIGINT, first_token_ms BIGINT, duration_ms BIGINT, created_at TIMESTAMP)`,
		`CREATE TABLE ops_error_logs (
		 id BIGINT,
		 account_id BIGINT, api_key_id BIGINT, request_id VARCHAR(64), client_request_id VARCHAR(64),
		 request_type INT, is_count_tokens BOOLEAN, is_business_limited BOOLEAN,
		 error_owner VARCHAR(32), upstream_status_code INT, status_code INT, created_at TIMESTAMP)`,
	} {
		_, err := db.ExecContext(ctx, statement)
		require.NoError(t, err)
	}
	start := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
	at := start.Add(time.Minute)
	for i, row := range []struct {
		request                       string
		account, kind                 int
		cost                          float64
		input, cached, ttft, duration int
	}{
		{"success", 17, 2, 1, 25, 75, 100, 1000},
		{"retried-success", 17, 2, 1, 75, 25, 300, 3000},
		{"billed-image", 17, 1, 1, 0, 0, 0, 2000},
		{"failed-placeholder", 17, 2, 0, 0, 0, 9000, 9000},
		{"cyber-blocked", 17, 4, 1, 100, 0, 9000, 9000},
		{"probe", 17, 6, 1, 100, 0, 9000, 9000},
		{"other-account", 18, 2, 1, 100, 0, 9000, 9000},
	} {
		_, err := db.ExecContext(ctx, repo.query(`INSERT INTO usage_logs
		 (id, account_id, api_key_id, request_id, request_type, actual_cost, input_tokens, cache_creation_tokens,
		 cache_read_tokens, first_token_ms, duration_ms, created_at) VALUES (?, ?, 1, ?, ?, ?, ?, 0, ?, ?, ?, ?)`),
			i+1, row.account, row.request, row.kind, row.cost, row.input, row.cached, row.ttft, row.duration, at)
		require.NoError(t, err)
	}
	for i, row := range []struct {
		request                      string
		account, kind                int
		countTokens, businessLimited bool
		owner                        string
	}{
		{"failed-placeholder", 17, 2, false, false, "provider"},
		{"failed-placeholder", 17, 2, false, false, "provider"},
		{"error-only", 17, 2, false, false, "provider"},
		{"retried-success", 17, 2, false, false, "provider"},
		{"count-tokens", 17, 1, true, false, "provider"},
		{"business-limit", 17, 1, false, true, "provider"},
		{"client-error", 17, 1, false, false, "client"},
		{"probe", 17, 6, false, false, "provider"},
		{"cyber-blocked", 17, 4, false, false, "provider"},
		{"other-account", 18, 2, false, false, "provider"},
	} {
		_, err := db.ExecContext(ctx, repo.query(`INSERT INTO ops_error_logs
		 (id, account_id, api_key_id, request_id, client_request_id, request_type, is_count_tokens,
		 is_business_limited, error_owner, upstream_status_code, status_code, created_at)
		 VALUES (?, ?, 1, ?, NULL, ?, ?, ?, ?, 500, 502, ?)`),
			i+1, row.account, row.request, row.kind, row.countTokens, row.businessLimited, row.owner, at)
		require.NoError(t, err)
	}
	snapshot, err := repo.GetAccountMetricSnapshot(ctx, 17, start, start.Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, int64(2), snapshot.SuccessSamples, "terminal errors override billed partial HTTP output; billed images still count")
	require.Equal(t, int64(3), snapshot.ErrorSamples, "terminal failures deduplicate without being erased by partial output")
	require.InDelta(t, 0.4, *snapshot.SuccessRate, 0.0001)
	require.Equal(t, int64(1), snapshot.CacheSamples)
	require.InDelta(t, 0.75, *snapshot.CacheRate, 0.0001)
	require.Equal(t, int64(1), snapshot.TTFTSamples)
	require.InDelta(t, 100.0, *snapshot.AverageTTFTMs, 0.001)
	require.Equal(t, int64(2), snapshot.DurationSamples)
	require.InDelta(t, 1500.0, *snapshot.AverageDurationMs, 0.001)

	// Free-token successes count, zero timings are real, image input is excluded
	// from cache denominators, duplicate request IDs retain only the latest row.
	for _, statement := range []string{
		`INSERT INTO usage_logs (id,account_id,api_key_id,request_id,request_type,actual_cost,input_tokens,output_tokens,image_input_tokens,cache_creation_tokens,cache_read_tokens,first_token_ms,duration_ms,created_at)
		 VALUES (21,19,1,'free',2,0,40,1,20,0,80,0,0,?)`,
		`INSERT INTO usage_logs (id,account_id,api_key_id,request_id,request_type,actual_cost,input_tokens,output_tokens,image_input_tokens,cache_creation_tokens,cache_read_tokens,first_token_ms,duration_ms,created_at)
		 VALUES (22,19,1,'free',2,0,40,1,20,0,80,0,0,?)`,
		`INSERT INTO usage_logs (id,account_id,api_key_id,request_id,request_type,actual_cost,input_tokens,cache_creation_tokens,cache_read_tokens,first_token_ms,duration_ms,created_at)
		 VALUES (23,19,1,'ws',3,1,0,0,0,-1,-1,?)`,
		`INSERT INTO ops_error_logs (id,account_id,api_key_id,request_id,request_type,error_owner,status_code,created_at)
		 VALUES (21,19,1,'ws',3,'provider',500,?)`,
	} {
		_, err = db.ExecContext(ctx, repo.query(statement), at)
		require.NoError(t, err)
	}
	batch, err := repo.GetAccountMetricSnapshots(ctx, []int64{17, 19, 20}, start, start.Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, int64(2), batch[19].SuccessSamples)
	require.Equal(t, int64(1), batch[19].ErrorSamples)
	require.InDelta(t, 0.8, *batch[19].CacheRate, 0.0001)
	require.Equal(t, int64(1), batch[19].CacheSamples)
	require.Equal(t, int64(1), batch[19].TTFTSamples)
	require.Zero(t, *batch[19].AverageTTFTMs)
	require.Zero(t, *batch[19].AverageDurationMs)
	require.Nil(t, batch[20].SuccessRate)
}
