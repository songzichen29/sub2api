package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Platform is derived from group/account (usage_logs has no provider column on upstream schema).
const channelMonitorV2PlatformSQL = `lower(` + usageLogEffectivePlatformExpr + `)`
const channelMonitorV2ModelSQL = `COALESCE(NULLIF(TRIM(ul.requested_model), ''), NULLIF(TRIM(ul.model), ''), 'unknown')`

// Tiered retention balances UI windows against storage:
//
//	1m facts  → short (late writes + rebuild rollups)
//	5m/1h/12h/1d rollups → longer, aligned to 90m / 24h / 7d / 30d(+audit)
//
// Backfill may still write short-lived 1m rows for old windows so rollups can be
// built; prune at end of each recompute drops them past their TTL while rollups remain.
const (
	channelMonitorV2RetentionUser1m      = 3 * 24 * time.Hour
	channelMonitorV2RetentionMetrics1m   = 7 * 24 * time.Hour
	channelMonitorV2RetentionError1m     = 7 * 24 * time.Hour
	channelMonitorV2RetentionHistogram1m = 7 * 24 * time.Hour
	channelMonitorV2RetentionRollup5m    = 7 * 24 * time.Hour  // bucket_seconds=300
	channelMonitorV2RetentionRollup1h    = 30 * 24 * time.Hour // 3600
	channelMonitorV2RetentionRollup12h   = 45 * 24 * time.Hour // 43200
	channelMonitorV2RetentionRollup1d    = 90 * 24 * time.Hour // 86400
	channelMonitorV2RetentionMax         = channelMonitorV2RetentionRollup1d
)

// channelMonitorV2MaxRetention is the longest stored window (1d rollup). Used to
// clamp recompute/backfill so we never scan older than product history needs.
func channelMonitorV2MaxRetention() time.Duration {
	return channelMonitorV2RetentionMax
}

func channelMonitorV2RetentionCutoff(now time.Time, retention time.Duration) time.Time {
	return now.UTC().Truncate(time.Minute).Add(-retention)
}

type channelMonitorV2RetentionRule struct {
	table         string
	retention     time.Duration
	bucketSeconds int // 0 = fact table (no bucket_seconds column)
}

// channelMonitorV2RetentionRules is ordered coarse→fine for predictable prune plans.
var channelMonitorV2RetentionRules = []channelMonitorV2RetentionRule{
	{table: "channel_monitor_v2_user_metrics_1m", retention: channelMonitorV2RetentionUser1m},
	{table: "channel_monitor_v2_metrics_1m", retention: channelMonitorV2RetentionMetrics1m},
	{table: "channel_monitor_v2_error_metrics_1m", retention: channelMonitorV2RetentionError1m},
	{table: "channel_monitor_v2_latency_histograms_1m", retention: channelMonitorV2RetentionHistogram1m},
	{table: "channel_monitor_v2_metrics_rollup", retention: channelMonitorV2RetentionRollup5m, bucketSeconds: 300},
	{table: "channel_monitor_v2_user_metrics_rollup", retention: channelMonitorV2RetentionRollup5m, bucketSeconds: 300},
	{table: "channel_monitor_v2_error_metrics_rollup", retention: channelMonitorV2RetentionRollup5m, bucketSeconds: 300},
	{table: "channel_monitor_v2_latency_histograms_rollup", retention: channelMonitorV2RetentionRollup5m, bucketSeconds: 300},
	{table: "channel_monitor_v2_metrics_rollup", retention: channelMonitorV2RetentionRollup1h, bucketSeconds: 3600},
	{table: "channel_monitor_v2_user_metrics_rollup", retention: channelMonitorV2RetentionRollup1h, bucketSeconds: 3600},
	{table: "channel_monitor_v2_error_metrics_rollup", retention: channelMonitorV2RetentionRollup1h, bucketSeconds: 3600},
	{table: "channel_monitor_v2_latency_histograms_rollup", retention: channelMonitorV2RetentionRollup1h, bucketSeconds: 3600},
	{table: "channel_monitor_v2_metrics_rollup", retention: channelMonitorV2RetentionRollup12h, bucketSeconds: 43200},
	{table: "channel_monitor_v2_user_metrics_rollup", retention: channelMonitorV2RetentionRollup12h, bucketSeconds: 43200},
	{table: "channel_monitor_v2_error_metrics_rollup", retention: channelMonitorV2RetentionRollup12h, bucketSeconds: 43200},
	{table: "channel_monitor_v2_latency_histograms_rollup", retention: channelMonitorV2RetentionRollup12h, bucketSeconds: 43200},
	{table: "channel_monitor_v2_metrics_rollup", retention: channelMonitorV2RetentionRollup1d, bucketSeconds: 86400},
	{table: "channel_monitor_v2_user_metrics_rollup", retention: channelMonitorV2RetentionRollup1d, bucketSeconds: 86400},
	{table: "channel_monitor_v2_error_metrics_rollup", retention: channelMonitorV2RetentionRollup1d, bucketSeconds: 86400},
	{table: "channel_monitor_v2_latency_histograms_rollup", retention: channelMonitorV2RetentionRollup1d, bucketSeconds: 86400},
}

func (r *channelMonitorV2Repository) pruneChannelMonitorV2Retention(ctx context.Context, tx *sql.Tx, now time.Time) error {
	// During historical bootstrap, retain all 1m facts until the cursor reaches
	// the oldest rollup boundary. Otherwise adjacent chunks would rebuild the
	// same daily bucket from source rows already pruned by the prior chunk.
	var backfillCursor time.Time
	if err := tx.QueryRowContext(ctx, `SELECT backfill_cursor FROM channel_monitor_v2_watermarks WHERE id = 1`).Scan(&backfillCursor); err == nil && backfillCursor.After(channelMonitorV2RetentionCutoff(now, channelMonitorV2RetentionMax)) {
		return nil
	}
	for _, rule := range channelMonitorV2RetentionRules {
		cutoff := channelMonitorV2RetentionCutoff(now, rule.retention)
		var err error
		if rule.bucketSeconds == 0 {
			_, err = tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE bucket_start < ?`, rule.table), cutoff)
		} else {
			_, err = tx.ExecContext(ctx,
				fmt.Sprintf(`DELETE FROM %s WHERE bucket_seconds = ? AND bucket_start < ?`, rule.table),
				rule.bucketSeconds, cutoff,
			)
		}
		if err != nil {
			return fmt.Errorf("prune %s (bucket_seconds=%d): %w", rule.table, rule.bucketSeconds, err)
		}
	}
	return nil
}

func (r *channelMonitorV2Repository) RecomputeRange(ctx context.Context, start, end time.Time) (err error) {
	start = start.UTC().Truncate(time.Minute)
	end = end.UTC().Truncate(time.Minute)
	now := time.Now().UTC().Truncate(time.Minute)
	// Clamp to longest rollup TTL so backfill does not scan beyond product history.
	maxCutoff := channelMonitorV2RetentionCutoff(now, channelMonitorV2MaxRetention())
	if start.Before(maxCutoff) {
		start = maxCutoff
	}
	if !start.Before(end) {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Idempotent window rewrite: drop existing facts/rollups in [start,end) then re-insert.
	for _, table := range []string{
		"channel_monitor_v2_latency_histograms_rollup",
		"channel_monitor_v2_error_metrics_rollup",
		"channel_monitor_v2_user_metrics_rollup",
		"channel_monitor_v2_metrics_rollup",
		"channel_monitor_v2_latency_histograms_1m",
		"channel_monitor_v2_error_metrics_1m",
		"channel_monitor_v2_user_metrics_1m",
		"channel_monitor_v2_metrics_1m",
	} {
		if _, err = tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE bucket_start >= ? AND bucket_start < ?", table), start, end); err != nil {
			return err
		}
	}

	if _, err = tx.ExecContext(ctx, fmt.Sprintf(channelMonitorV2UsageMetricsSQL, channelMonitorV2PlatformSQL, channelMonitorV2ModelSQL), start, end); err != nil {
		return fmt.Errorf("aggregate channel monitor v2 usage: %w", err)
	}
	if _, err = tx.ExecContext(ctx, fmt.Sprintf(channelMonitorV2UserMetricsSQL, channelMonitorV2PlatformSQL, channelMonitorV2ModelSQL), start, end); err != nil {
		return fmt.Errorf("aggregate channel monitor v2 users: %w", err)
	}
	if _, err = tx.ExecContext(ctx, fmt.Sprintf(channelMonitorV2HistogramSQL, channelMonitorV2PlatformSQL, channelMonitorV2ModelSQL, channelMonitorV2HistogramBoundSQL("latency.value_ms")), start, end); err != nil {
		return fmt.Errorf("aggregate channel monitor v2 histograms: %w", err)
	}
	if err = aggregateChannelMonitorV2Errors(ctx, tx, start, end); err != nil {
		return fmt.Errorf("aggregate channel monitor v2 errors: %w", err)
	}
	if err = r.recomputeFixedRollups(ctx, tx, start, end); err != nil {
		return err
	}
	// Drop rows past per-tier TTL (1m short, coarse rollups long). Safe after rollup
	// so a backfill chunk can build 1d rollups from temporary 1m rows then discard 1m.
	if err = r.pruneChannelMonitorV2Retention(ctx, tx, now); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, channelMonitorV2WatermarkSQL, start, start, end, start); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

var channelMonitorV2UsageMetricsSQL = `
INSERT INTO channel_monitor_v2_metrics_1m (
  bucket_start, platform, group_id, model, success_requests,
  input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
  ttft_sum_ms, ttft_count, duration_sum_ms, duration_count, computed_at
)
SELECT TIMESTAMPADD(MINUTE, TIMESTAMPDIFF(MINUTE, '1970-01-01 00:00:00', ul.created_at), '1970-01-01 00:00:00'),
       %s, COALESCE(ul.group_id, 0), %s,
       COUNT(DISTINCT CASE
         WHEN COALESCE(ul.request_type, 0) NOT IN (4, 6) AND ` + usageLogSuccessFilterUL + `
         THEN COALESCE(NULLIF(ul.request_id, ''), CONCAT('usage:', CAST(ul.id AS CHAR))) END),
       COALESCE(SUM(CASE WHEN ` + usageLogSuccessFilterUL + ` THEN ul.input_tokens ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN ` + usageLogSuccessFilterUL + ` THEN ul.output_tokens ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN ` + usageLogSuccessFilterUL + ` THEN ul.cache_creation_tokens ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN ` + usageLogSuccessFilterUL + ` THEN ul.cache_read_tokens ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN ul.first_token_ms IS NOT NULL AND ` + usageLogSuccessFilterUL + ` THEN ul.first_token_ms ELSE 0 END), 0),
       SUM(CASE WHEN ul.first_token_ms IS NOT NULL AND ` + usageLogSuccessFilterUL + ` THEN 1 ELSE 0 END),
       COALESCE(SUM(CASE WHEN ul.duration_ms IS NOT NULL AND ` + usageLogSuccessFilterUL + ` THEN ul.duration_ms ELSE 0 END), 0),
       SUM(CASE WHEN ul.duration_ms IS NOT NULL AND ` + usageLogSuccessFilterUL + ` THEN 1 ELSE 0 END), NOW(6)
FROM usage_logs ul
LEFT JOIN ` + "`groups`" + ` g ON g.id = ul.group_id
LEFT JOIN accounts a ON a.id = ul.account_id
WHERE ul.created_at >= ? AND ul.created_at < ?
GROUP BY 1, 2, 3, 4`

var channelMonitorV2UserMetricsSQL = `
INSERT INTO channel_monitor_v2_user_metrics_1m (
  bucket_start, platform, group_id, model, user_id, success_requests,
  input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
  ttft_sum_ms, ttft_count, duration_sum_ms, duration_count, computed_at
)
SELECT TIMESTAMPADD(MINUTE, TIMESTAMPDIFF(MINUTE, '1970-01-01 00:00:00', ul.created_at), '1970-01-01 00:00:00'),
       %s, COALESCE(ul.group_id, 0), %s, ul.user_id,
       COUNT(DISTINCT CASE
         WHEN COALESCE(ul.request_type, 0) NOT IN (4, 6) AND ` + usageLogSuccessFilterUL + `
         THEN COALESCE(NULLIF(ul.request_id, ''), CONCAT('usage:', CAST(ul.id AS CHAR))) END),
       COALESCE(SUM(CASE WHEN ` + usageLogSuccessFilterUL + ` THEN ul.input_tokens ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN ` + usageLogSuccessFilterUL + ` THEN ul.output_tokens ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN ` + usageLogSuccessFilterUL + ` THEN ul.cache_creation_tokens ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN ` + usageLogSuccessFilterUL + ` THEN ul.cache_read_tokens ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN ul.first_token_ms IS NOT NULL AND ` + usageLogSuccessFilterUL + ` THEN ul.first_token_ms ELSE 0 END), 0),
       SUM(CASE WHEN ul.first_token_ms IS NOT NULL AND ` + usageLogSuccessFilterUL + ` THEN 1 ELSE 0 END),
       COALESCE(SUM(CASE WHEN ul.duration_ms IS NOT NULL AND ` + usageLogSuccessFilterUL + ` THEN ul.duration_ms ELSE 0 END), 0),
       SUM(CASE WHEN ul.duration_ms IS NOT NULL AND ` + usageLogSuccessFilterUL + ` THEN 1 ELSE 0 END), NOW(6)
FROM usage_logs ul
LEFT JOIN ` + "`groups`" + ` g ON g.id = ul.group_id
LEFT JOIN accounts a ON a.id = ul.account_id
WHERE ul.created_at >= ? AND ul.created_at < ? AND ul.user_id IS NOT NULL
GROUP BY 1, 2, 3, 4, 5`

var channelMonitorV2HistogramSQL = `
INSERT INTO channel_monitor_v2_latency_histograms_1m (
  bucket_start, platform, group_id, model, user_id, metric, upper_bound_ms, sample_count
)
SELECT TIMESTAMPADD(MINUTE, TIMESTAMPDIFF(MINUTE, '1970-01-01 00:00:00', ul.created_at), '1970-01-01 00:00:00'),
       %s, COALESCE(ul.group_id, 0), %s,
       audience.user_id, latency.metric, %s, COUNT(*)
FROM usage_logs ul
LEFT JOIN ` + "`groups`" + ` g ON g.id = ul.group_id
LEFT JOIN accounts a ON a.id = ul.account_id
JOIN JSON_TABLE(JSON_ARRAY(0, ul.user_id), '$[*]'
  COLUMNS (user_id BIGINT PATH '$' NULL ON ERROR)) AS audience ON TRUE
JOIN JSON_TABLE(JSON_ARRAY(
    JSON_OBJECT('metric', 'ttft', 'value_ms', ul.first_token_ms),
    JSON_OBJECT('metric', 'duration', 'value_ms', ul.duration_ms)
  ), '$[*]' COLUMNS (
    metric VARCHAR(20) PATH '$.metric',
    value_ms BIGINT PATH '$.value_ms' NULL ON EMPTY NULL ON ERROR
  )) AS latency ON TRUE
WHERE ul.created_at >= ? AND ul.created_at < ?
  AND audience.user_id IS NOT NULL AND latency.value_ms IS NOT NULL AND latency.value_ms >= 0
  AND ` + usageLogSuccessFilterUL + `
GROUP BY 1, 2, 3, 4, 5, 6, 7`

func channelMonitorV2HistogramBoundSQL(column string) string {
	return `CASE
WHEN ` + column + ` <= 50 THEN 50 WHEN ` + column + ` <= 100 THEN 100
WHEN ` + column + ` <= 250 THEN 250 WHEN ` + column + ` <= 500 THEN 500
WHEN ` + column + ` <= 1000 THEN 1000 WHEN ` + column + ` <= 2000 THEN 2000
WHEN ` + column + ` <= 3000 THEN 3000 WHEN ` + column + ` <= 5000 THEN 5000
WHEN ` + column + ` <= 8000 THEN 8000 WHEN ` + column + ` <= 10000 THEN 10000
WHEN ` + column + ` <= 15000 THEN 15000 WHEN ` + column + ` <= 30000 THEN 30000
WHEN ` + column + ` <= 60000 THEN 60000 WHEN ` + column + ` <= 120000 THEN 120000
WHEN ` + column + ` <= 300000 THEN 300000 WHEN ` + column + ` <= 600000 THEN 600000
ELSE 2147483647 END`
}

const channelMonitorV2ErrorDedupLookback = 90 * time.Minute

// Error dedup lookback is bounded to 90 minutes. MySQL cannot reuse a CTE
// across three INSERT statements, so one transaction-local temporary table
// holds the classified rows for the metric, user and taxonomy aggregates.
const channelMonitorV2ErrorAggregationSQL = `
CREATE TEMPORARY TABLE channel_monitor_v2_classified_errors AS
WITH candidate_ids AS (
  SELECT DISTINCT request_id
  FROM ops_error_logs
  WHERE created_at >= ? AND created_at < ? AND NULLIF(request_id, '') IS NOT NULL
), ranked AS (
  SELECT
    TIMESTAMPADD(MINUTE, TIMESTAMPDIFF(MINUTE, '1970-01-01 00:00:00', current_error.created_at), '1970-01-01 00:00:00') AS bucket_start,
    lower(CASE
      WHEN g.platform = 'composite' THEN COALESCE(NULLIF(TRIM(a.platform), ''), NULLIF(NULLIF(lower(TRIM(current_error.platform)), ''), 'composite'), 'unknown')
      ELSE COALESCE(NULLIF(TRIM(current_error.platform), ''), 'unknown')
    END) AS platform,
    COALESCE(current_error.group_id, 0) AS group_id,
    COALESCE(NULLIF(TRIM(current_error.requested_model), ''), NULLIF(TRIM(current_error.model), ''), 'unknown') AS model,
    current_error.user_id, current_error.error_type, current_error.error_owner,
    COALESCE(current_error.status_code, 0) AS status_code,
    COALESCE(current_error.upstream_status_code, 0) AS upstream_status_code,
    lower(CONCAT_WS(' ', current_error.error_type, current_error.error_source, current_error.error_message,
      current_error.upstream_error_message, current_error.upstream_error_detail, current_error.error_body)) AS error_text,
    (CASE WHEN JSON_TYPE(current_error.upstream_errors) = 'ARRAY'
      THEN COALESCE(JSON_LENGTH(current_error.upstream_errors), 0) > 0 ELSE FALSE END
      OR current_error.error_owner = 'provider' OR current_error.upstream_status_code IS NOT NULL) AS upstream_affected,
    CASE WHEN JSON_TYPE(current_error.upstream_errors) = 'ARRAY'
      THEN COALESCE(JSON_LENGTH(current_error.upstream_errors), 0) ELSE 0 END AS upstream_attempts,
    ROW_NUMBER() OVER (
      PARTITION BY COALESCE(NULLIF(current_error.request_id, ''), CONCAT('error:', CAST(current_error.id AS CHAR)))
      ORDER BY current_error.created_at DESC, current_error.id DESC
    ) AS row_num
  FROM ops_error_logs current_error
  LEFT JOIN ` + "`groups`" + ` g ON g.id = current_error.group_id
  LEFT JOIN accounts a ON a.id = current_error.account_id
  WHERE (
      (NULLIF(current_error.request_id, '') IS NULL AND current_error.created_at >= ? AND current_error.created_at < ?)
      OR (
        current_error.request_id IN (SELECT request_id FROM candidate_ids)
        AND current_error.created_at >= ?
        AND current_error.created_at < ?
      )
    )
    AND NOT current_error.is_count_tokens
    AND (COALESCE(current_error.status_code, 0) >= 400 OR current_error.error_type = 'cyber_policy')
), dedup AS (
  SELECT * FROM ranked WHERE row_num = 1
)
SELECT *, CASE
  WHEN error_type = 'cyber_policy' OR error_text LIKE '%content policy%' OR error_text LIKE '%content_policy%'
    OR error_text LIKE '%safety policy%' OR error_text LIKE '%moderation%' OR error_text LIKE '%blocked keyword%' THEN 'content_policy'
  WHEN status_code = 401 OR upstream_status_code = 401 OR error_text LIKE '%unauthorized%'
    OR error_text LIKE '%invalid api key%' OR error_text LIKE '%invalid_api_key%' OR error_text LIKE '%authentication%'
    OR error_text LIKE '%api_key_disabled%' THEN 'authentication'
  WHEN error_text LIKE '%context window%' OR error_text LIKE '%context length%' OR error_text LIKE '%maximum prompt length%'
    OR error_text LIKE '%too many tokens%' OR error_text LIKE '%max_tokens%' THEN 'context_limit'
  WHEN error_text LIKE '%failed to deserialize%' OR error_text LIKE '%missing required parameter%'
    OR error_text LIKE '%invalid request%' OR error_text LIKE '%invalid_request%' OR error_text LIKE '%tool_choice%' THEN 'invalid_request'
  WHEN error_text LIKE '%does not support the requested model%' OR error_text LIKE '%not supported by any configured account%'
    OR error_text LIKE '%model not supported%' OR error_text LIKE '%unsupported model%' THEN 'model_unsupported'
  WHEN error_text LIKE '%group not allowed%' OR error_text LIKE '%group_not_allowed%' OR error_text LIKE '%group access%' THEN 'group_access'
  WHEN error_text LIKE '%run out of credits%' OR error_text LIKE '%insufficient balance%' OR error_text LIKE '%insufficient quota%'
    OR error_text LIKE '%subscription%' OR error_text LIKE '%quota exceeded%' OR error_text LIKE '%billing hard limit%' THEN 'quota_or_balance'
  WHEN error_text LIKE '%no available accounts%' OR error_text LIKE '%no healthy account%'
    OR error_text LIKE '%no healthy upstream account%' OR error_text LIKE '%failover budget exhausted%'
    OR error_text LIKE '%account pool%' THEN 'account_pool_unavailable'
  WHEN status_code = 429 OR upstream_status_code = 429 OR error_text LIKE '%rate limit%' OR error_text LIKE '%rate_limit%'
    OR error_text LIKE '%high demand%' OR error_text LIKE '%overloaded%' OR error_text LIKE '%concurrency limit%'
    OR error_text LIKE '%capacity%' THEN 'rate_or_capacity'
  WHEN status_code IN (408, 504) OR error_text LIKE '%timeout%' OR error_text LIKE '%deadline exceeded%'
    OR error_text LIKE '%error code: 524%' OR error_text LIKE '%gateway time-out%' OR error_text LIKE '%gateway timeout%' THEN 'timeout'
  WHEN error_text LIKE '%transport%' OR error_text LIKE '%stream_read_error%' OR error_text LIKE '%connection reset%'
    OR error_text LIKE '%connection refused%' OR error_text LIKE '%tls%' OR error_text LIKE '%http2%'
    OR error_text LIKE '%missing terminal event%' OR error_text LIKE '%unexpected eof%' THEN 'transport_or_stream'
  WHEN status_code = 403 OR upstream_status_code = 403 THEN 'upstream_forbidden'
  WHEN status_code = 404 OR upstream_status_code = 404 THEN 'not_found'
  WHEN status_code = 499 OR error_text LIKE '%client cancelled%' OR error_text LIKE '%client canceled%'
    OR error_text LIKE '%context canceled%' THEN 'client_cancelled'
  WHEN upstream_status_code >= 500 OR (error_owner = 'provider' AND status_code >= 500) THEN 'upstream_5xx'
  WHEN status_code >= 500 OR error_type = 'internal' OR error_owner = 'system' THEN 'internal'
  ELSE 'other' END AS category
FROM dedup
WHERE bucket_start >= ? AND bucket_start < ?`

const channelMonitorV2ErrorMetricsInsertSQL = `
INSERT INTO channel_monitor_v2_metrics_1m (
  bucket_start, platform, group_id, model, error_requests,
  upstream_affected_requests, upstream_attempt_count, computed_at
)
SELECT bucket_start, platform, group_id, model, COUNT(*),
       SUM(CASE WHEN upstream_affected THEN 1 ELSE 0 END), SUM(upstream_attempts), NOW(6)
FROM channel_monitor_v2_classified_errors
GROUP BY bucket_start, platform, group_id, model
ON DUPLICATE KEY UPDATE
  error_requests = VALUES(error_requests),
  upstream_affected_requests = VALUES(upstream_affected_requests),
  upstream_attempt_count = VALUES(upstream_attempt_count),
  computed_at = NOW(6)`

const channelMonitorV2ErrorUserInsertSQL = `
INSERT INTO channel_monitor_v2_user_metrics_1m (
  bucket_start, platform, group_id, model, user_id, error_requests, computed_at
)
SELECT bucket_start, platform, group_id, model, user_id, COUNT(*), NOW(6)
FROM channel_monitor_v2_classified_errors
WHERE user_id IS NOT NULL
GROUP BY bucket_start, platform, group_id, model, user_id
ON DUPLICATE KEY UPDATE error_requests = VALUES(error_requests), computed_at = NOW(6)`

const channelMonitorV2ErrorCategoryInsertSQL = `
INSERT INTO channel_monitor_v2_error_metrics_1m (
  bucket_start, platform, group_id, model, error_category, taxonomy_version, error_requests
)
SELECT bucket_start, platform, group_id, model, category, 1, COUNT(*)
FROM channel_monitor_v2_classified_errors
GROUP BY bucket_start, platform, group_id, model, category
ON DUPLICATE KEY UPDATE error_requests = VALUES(error_requests)`

func aggregateChannelMonitorV2Errors(ctx context.Context, tx *sql.Tx, start, end time.Time) (err error) {
	const tempTable = "channel_monitor_v2_classified_errors"
	if _, err := tx.ExecContext(ctx, "DROP TEMPORARY TABLE IF EXISTS "+tempTable); err != nil {
		return err
	}
	defer func() {
		_, dropErr := tx.ExecContext(ctx, "DROP TEMPORARY TABLE IF EXISTS "+tempTable)
		if err == nil && dropErr != nil {
			err = dropErr
		}
	}()
	lookback := start.Add(-channelMonitorV2ErrorDedupLookback)
	if _, err = tx.ExecContext(ctx, channelMonitorV2ErrorAggregationSQL,
		start, end, start, end, lookback, end, start, end,
	); err != nil {
		return err
	}
	for _, query := range []string{
		channelMonitorV2ErrorMetricsInsertSQL,
		channelMonitorV2ErrorUserInsertSQL,
		channelMonitorV2ErrorCategoryInsertSQL,
	} {
		if _, err = tx.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	return nil
}

// Floor matches channelMonitorV2RetentionMax (90d). Keep the 90-day expression
// in sync when changing channelMonitorV2RetentionRollup1d.
//
// Coverage starts track how far back recompute has walked (chunk start), not
// "min(source_log.created_at)". Using global min(ops_error_logs) pins
// error_coverage_start to the first real error forever and collapses UI windows
// when errors only exist in a recent slice (common on first upgrade).
const channelMonitorV2WatermarkSQL = `
INSERT INTO channel_monitor_v2_watermarks (id, usage_coverage_start, error_coverage_start, data_through, last_successful_at, backfill_cursor, updated_at)
VALUES (
  1,
  ?,
  ?,
  ?, NOW(6), ?, NOW(6)
)
ON DUPLICATE KEY UPDATE
  usage_coverage_start = GREATEST(
    TIMESTAMPADD(DAY, -90, TIMESTAMPADD(MINUTE, TIMESTAMPDIFF(MINUTE, '1970-01-01 00:00:00', UTC_TIMESTAMP(6)), '1970-01-01 00:00:00')),
    LEAST(COALESCE(channel_monitor_v2_watermarks.usage_coverage_start, VALUES(usage_coverage_start)), VALUES(usage_coverage_start))
  ),
  error_coverage_start = GREATEST(
    TIMESTAMPADD(DAY, -90, TIMESTAMPADD(MINUTE, TIMESTAMPDIFF(MINUTE, '1970-01-01 00:00:00', UTC_TIMESTAMP(6)), '1970-01-01 00:00:00')),
    LEAST(COALESCE(channel_monitor_v2_watermarks.error_coverage_start, VALUES(error_coverage_start)), VALUES(error_coverage_start))
  ),
  data_through = GREATEST(COALESCE(channel_monitor_v2_watermarks.data_through, VALUES(data_through)), VALUES(data_through)),
  last_successful_at = NOW(6),
  backfill_cursor = LEAST(COALESCE(channel_monitor_v2_watermarks.backfill_cursor, VALUES(backfill_cursor)), VALUES(backfill_cursor)),
  updated_at = NOW(6)`

var channelMonitorV2FixedRollupSeconds = []int{300, 3600, 43200, 86400}

func (r *channelMonitorV2Repository) recomputeFixedRollups(ctx context.Context, tx *sql.Tx, start, end time.Time) error {
	for _, seconds := range channelMonitorV2FixedRollupSeconds {
		// Coarse buckets are immutable between boundaries during the normal
		// trailing refresh. Historical backfills and boundary-crossing windows
		// still rebuild them; this avoids repeatedly regrouping the full current
		// day/user table every few minutes.
		if seconds >= 43200 && sameFixedRollupBucket(start, end, seconds) {
			continue
		}
		interval := time.Duration(seconds) * time.Second
		boundsStart := start.UTC().Truncate(interval)
		boundsEnd := end.UTC().Add(-time.Nanosecond).Truncate(interval).Add(interval)
		for _, table := range []string{
			"channel_monitor_v2_latency_histograms_rollup",
			"channel_monitor_v2_error_metrics_rollup",
			"channel_monitor_v2_user_metrics_rollup",
			"channel_monitor_v2_metrics_rollup",
		} {
			if _, err := tx.ExecContext(ctx, fmt.Sprintf(channelMonitorV2FixedRollupDeleteSQL, table), seconds, boundsStart, boundsEnd); err != nil {
				return err
			}
		}
		bucketExpr := channelMonitorV2BucketExpr("m.bucket_start", seconds)
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(channelMonitorV2MetricsRollupSQL, bucketExpr, seconds), boundsStart, boundsEnd); err != nil {
			return fmt.Errorf("roll up channel monitor v2 metrics %ds: %w", seconds, err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(channelMonitorV2UserMetricsRollupSQL, bucketExpr, seconds), boundsStart, boundsEnd); err != nil {
			return fmt.Errorf("roll up channel monitor v2 user metrics %ds: %w", seconds, err)
		}
		histogramBucketExpr := channelMonitorV2BucketExpr("h.bucket_start", seconds)
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(channelMonitorV2HistogramRollupSQL, histogramBucketExpr, seconds), boundsStart, boundsEnd); err != nil {
			return fmt.Errorf("roll up channel monitor v2 histograms %ds: %w", seconds, err)
		}
		errorBucketExpr := channelMonitorV2BucketExpr("e.bucket_start", seconds)
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(channelMonitorV2ErrorRollupSQL, errorBucketExpr, seconds), boundsStart, boundsEnd); err != nil {
			return fmt.Errorf("roll up channel monitor v2 errors %ds: %w", seconds, err)
		}
	}
	return nil
}

func sameFixedRollupBucket(start, end time.Time, seconds int) bool {
	if !end.After(start) {
		return true
	}
	interval := time.Duration(seconds) * time.Second
	return start.Truncate(interval).Equal(end.Add(-time.Nanosecond).Truncate(interval))
}

const channelMonitorV2FixedRollupDeleteSQL = `DELETE FROM %s
WHERE bucket_seconds = ? AND bucket_start >= ? AND bucket_start < ?`

const channelMonitorV2MetricsRollupSQL = `
INSERT INTO channel_monitor_v2_metrics_rollup (
  bucket_start, bucket_seconds, platform, group_id, model, success_requests, error_requests,
  upstream_affected_requests, upstream_attempt_count, input_tokens, output_tokens,
  cache_creation_tokens, cache_read_tokens, ttft_sum_ms, ttft_count, duration_sum_ms,
  duration_count, computed_at
)
SELECT %s, %d,
       platform, group_id, model, SUM(success_requests), SUM(error_requests),
       SUM(upstream_affected_requests), SUM(upstream_attempt_count), SUM(input_tokens),
       SUM(output_tokens), SUM(cache_creation_tokens), SUM(cache_read_tokens),
       SUM(ttft_sum_ms), SUM(ttft_count), SUM(duration_sum_ms), SUM(duration_count), NOW()
FROM channel_monitor_v2_metrics_1m m
WHERE m.bucket_start >= ? AND m.bucket_start < ?
GROUP BY 1, 2, 3, 4, 5`

const channelMonitorV2UserMetricsRollupSQL = `
INSERT INTO channel_monitor_v2_user_metrics_rollup (
  bucket_start, bucket_seconds, platform, group_id, model, user_id, success_requests,
  error_requests, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens,
  ttft_sum_ms, ttft_count, duration_sum_ms, duration_count, computed_at
)
SELECT %s, %d,
       platform, group_id, model, user_id, SUM(success_requests), SUM(error_requests),
       SUM(input_tokens), SUM(output_tokens), SUM(cache_creation_tokens), SUM(cache_read_tokens),
       SUM(ttft_sum_ms), SUM(ttft_count), SUM(duration_sum_ms), SUM(duration_count), NOW()
FROM channel_monitor_v2_user_metrics_1m m
WHERE m.bucket_start >= ? AND m.bucket_start < ?
GROUP BY 1, 2, 3, 4, 5, 6`

const channelMonitorV2HistogramRollupSQL = `
INSERT INTO channel_monitor_v2_latency_histograms_rollup (
  bucket_start, bucket_seconds, platform, group_id, model, user_id, metric, upper_bound_ms, sample_count
)
SELECT %s, %d,
       platform, group_id, model, user_id, metric, upper_bound_ms, SUM(sample_count)
FROM channel_monitor_v2_latency_histograms_1m h
WHERE h.bucket_start >= ? AND h.bucket_start < ?
GROUP BY 1, 2, 3, 4, 5, 6, 7, 8`

const channelMonitorV2ErrorRollupSQL = `
INSERT INTO channel_monitor_v2_error_metrics_rollup (
  bucket_start, bucket_seconds, platform, group_id, model, error_category, taxonomy_version, error_requests
)
SELECT %s, %d,
       platform, group_id, model, error_category, taxonomy_version, SUM(error_requests)
FROM channel_monitor_v2_error_metrics_1m e
WHERE e.bucket_start >= ? AND e.bucket_start < ?
GROUP BY 1, 2, 3, 4, 5, 6, 7`
