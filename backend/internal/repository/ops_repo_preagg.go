package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func (r *opsRepository) UpsertHourlyMetrics(ctx context.Context, startTime, endTime time.Time) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	if startTime.IsZero() || endTime.IsZero() || !endTime.After(startTime) {
		return nil
	}

	start := startTime.UTC()
	end := endTime.UTC()

	q := fmt.Sprintf(`
INSERT INTO ops_metrics_hourly (
  bucket_start,
  platform,
  group_id,
  success_count,
  error_count_total,
  business_limited_count,
  error_count_sla,
  upstream_error_count_excl_429_529,
  upstream_429_count,
  upstream_529_count,
  token_consumed,
  duration_p50_ms,
  duration_p90_ms,
  duration_p95_ms,
  duration_p99_ms,
  duration_avg_ms,
  duration_max_ms,
  ttft_p50_ms,
  ttft_p90_ms,
  ttft_p95_ms,
  ttft_p99_ms,
  ttft_avg_ms,
  ttft_max_ms,
  ttft_sample_count,
  computed_at
)
SELECT
  x.bucket_start,
  x.platform,
  x.group_id,
  SUM(x.success_count) AS success_count,
  SUM(x.error_count_total) AS error_count_total,
  SUM(x.business_limited_count) AS business_limited_count,
  SUM(x.error_count_sla) AS error_count_sla,
  SUM(x.upstream_error_count_excl_429_529) AS upstream_error_count_excl_429_529,
  SUM(x.upstream_429_count) AS upstream_429_count,
  SUM(x.upstream_529_count) AS upstream_529_count,
  SUM(x.token_consumed) AS token_consumed,
  NULL AS duration_p50_ms,
  NULL AS duration_p90_ms,
  NULL AS duration_p95_ms,
  NULL AS duration_p99_ms,
  CAST(AVG(NULLIF(x.duration_avg_ms, 0)) AS DECIMAL(10,2)) AS duration_avg_ms,
  MAX(NULLIF(x.duration_max_ms, 0)) AS duration_max_ms,
  NULL AS ttft_p50_ms,
  NULL AS ttft_p90_ms,
  NULL AS ttft_p95_ms,
  NULL AS ttft_p99_ms,
  CAST(SUM(x.ttft_avg_ms * x.ttft_sample_count) / NULLIF(SUM(x.ttft_sample_count), 0) AS DECIMAL(10,2)) AS ttft_avg_ms,
  MAX(NULLIF(x.ttft_max_ms, 0)) AS ttft_max_ms,
  COALESCE(SUM(x.ttft_sample_count), 0) AS ttft_sample_count,
  NOW() AS computed_at
FROM (
  -- usage: overall
  SELECT
    FROM_UNIXTIME(UNIX_TIMESTAMP(ul.created_at) - MOD(UNIX_TIMESTAMP(ul.created_at), 3600)) AS bucket_start,
    NULL AS platform,
    NULL AS group_id,
    COUNT(*) AS success_count,
    0 AS error_count_total,
    0 AS business_limited_count,
    0 AS error_count_sla,
    0 AS upstream_error_count_excl_429_529,
    0 AS upstream_429_count,
    0 AS upstream_529_count,
    COALESCE(SUM(ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens), 0) AS token_consumed,
    COALESCE(AVG(COALESCE(ul.duration_ms, 0)), 0) AS duration_avg_ms,
    COALESCE(MAX(ul.duration_ms), 0) AS duration_max_ms,
    AVG(ul.first_token_ms) AS ttft_avg_ms,
    COALESCE(MAX(ul.first_token_ms), 0) AS ttft_max_ms,
    COUNT(ul.first_token_ms) AS ttft_sample_count
  FROM usage_logs ul
  WHERE ul.created_at >= ? AND ul.created_at < ?
  GROUP BY 1

  UNION ALL

  -- usage: platform
  SELECT
    FROM_UNIXTIME(UNIX_TIMESTAMP(ul.created_at) - MOD(UNIX_TIMESTAMP(ul.created_at), 3600)) AS bucket_start,
    g.platform AS platform,
    NULL AS group_id,
    COUNT(*) AS success_count,
    0,0,0,0,0,0,
    COALESCE(SUM(ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens), 0) AS token_consumed,
    COALESCE(AVG(COALESCE(ul.duration_ms, 0)), 0) AS duration_avg_ms,
    COALESCE(MAX(ul.duration_ms), 0) AS duration_max_ms,
    AVG(ul.first_token_ms) AS ttft_avg_ms,
    COALESCE(MAX(ul.first_token_ms), 0) AS ttft_max_ms,
    COUNT(ul.first_token_ms) AS ttft_sample_count
  FROM usage_logs ul
  JOIN %s g ON g.id = ul.group_id
  WHERE ul.created_at >= ? AND ul.created_at < ?
  GROUP BY 1,2

  UNION ALL

  -- usage: group
  SELECT
    FROM_UNIXTIME(UNIX_TIMESTAMP(ul.created_at) - MOD(UNIX_TIMESTAMP(ul.created_at), 3600)) AS bucket_start,
    g.platform AS platform,
    ul.group_id AS group_id,
    COUNT(*) AS success_count,
    0,0,0,0,0,0,
    COALESCE(SUM(ul.input_tokens + ul.output_tokens + ul.cache_creation_tokens + ul.cache_read_tokens), 0) AS token_consumed,
    COALESCE(AVG(COALESCE(ul.duration_ms, 0)), 0) AS duration_avg_ms,
    COALESCE(MAX(ul.duration_ms), 0) AS duration_max_ms,
    AVG(ul.first_token_ms) AS ttft_avg_ms,
    COALESCE(MAX(ul.first_token_ms), 0) AS ttft_max_ms,
    COUNT(ul.first_token_ms) AS ttft_sample_count
  FROM usage_logs ul
  JOIN %s g ON g.id = ul.group_id
  WHERE ul.created_at >= ? AND ul.created_at < ?
  GROUP BY 1,2,3

  UNION ALL

  -- errors: overall
  SELECT
    FROM_UNIXTIME(UNIX_TIMESTAMP(o.created_at) - MOD(UNIX_TIMESTAMP(o.created_at), 3600)) AS bucket_start,
    NULL AS platform,
    NULL AS group_id,
    0 AS success_count,
    SUM(CASE WHEN COALESCE(NULLIF(o.upstream_status_code, 0), o.status_code, 0) >= 400 THEN 1 ELSE 0 END) AS error_count_total,
    SUM(CASE WHEN COALESCE(NULLIF(o.upstream_status_code, 0), o.status_code, 0) >= 400 AND o.is_business_limited THEN 1 ELSE 0 END) AS business_limited_count,
    SUM(CASE WHEN COALESCE(NULLIF(o.upstream_status_code, 0), o.status_code, 0) >= 400 AND NOT o.is_business_limited THEN 1 ELSE 0 END) AS error_count_sla,
    SUM(CASE WHEN o.error_owner = 'provider' AND NOT o.is_business_limited AND COALESCE(NULLIF(o.upstream_status_code, 0), o.status_code, 0) NOT IN (429,529) THEN 1 ELSE 0 END) AS upstream_error_count_excl_429_529,
    SUM(CASE WHEN o.error_owner = 'provider' AND NOT o.is_business_limited AND COALESCE(NULLIF(o.upstream_status_code, 0), o.status_code, 0) = 429 THEN 1 ELSE 0 END) AS upstream_429_count,
    SUM(CASE WHEN o.error_owner = 'provider' AND NOT o.is_business_limited AND COALESCE(NULLIF(o.upstream_status_code, 0), o.status_code, 0) = 529 THEN 1 ELSE 0 END) AS upstream_529_count,
    0 AS token_consumed,
    0 AS duration_avg_ms,
    0 AS duration_max_ms,
    0 AS ttft_avg_ms,
    0 AS ttft_max_ms,
    0 AS ttft_sample_count
  FROM ops_error_logs o
  WHERE o.created_at >= ? AND o.created_at < ? AND o.is_count_tokens = FALSE
  GROUP BY 1

  UNION ALL

  -- errors: platform
  SELECT
    FROM_UNIXTIME(UNIX_TIMESTAMP(o.created_at) - MOD(UNIX_TIMESTAMP(o.created_at), 3600)) AS bucket_start,
    COALESCE(o.platform, 'unknown') AS platform,
    NULL AS group_id,
    0 AS success_count,
    SUM(CASE WHEN COALESCE(NULLIF(o.upstream_status_code, 0), o.status_code, 0) >= 400 THEN 1 ELSE 0 END),
    SUM(CASE WHEN COALESCE(NULLIF(o.upstream_status_code, 0), o.status_code, 0) >= 400 AND o.is_business_limited THEN 1 ELSE 0 END),
    SUM(CASE WHEN COALESCE(NULLIF(o.upstream_status_code, 0), o.status_code, 0) >= 400 AND NOT o.is_business_limited THEN 1 ELSE 0 END),
    SUM(CASE WHEN o.error_owner = 'provider' AND NOT o.is_business_limited AND COALESCE(NULLIF(o.upstream_status_code, 0), o.status_code, 0) NOT IN (429,529) THEN 1 ELSE 0 END),
    SUM(CASE WHEN o.error_owner = 'provider' AND NOT o.is_business_limited AND COALESCE(NULLIF(o.upstream_status_code, 0), o.status_code, 0) = 429 THEN 1 ELSE 0 END),
    SUM(CASE WHEN o.error_owner = 'provider' AND NOT o.is_business_limited AND COALESCE(NULLIF(o.upstream_status_code, 0), o.status_code, 0) = 529 THEN 1 ELSE 0 END),
    0,0,0,0,0,0
  FROM ops_error_logs o
  WHERE o.created_at >= ? AND o.created_at < ? AND o.is_count_tokens = FALSE
  GROUP BY 1,2

  UNION ALL

  -- errors: group
  SELECT
    FROM_UNIXTIME(UNIX_TIMESTAMP(o.created_at) - MOD(UNIX_TIMESTAMP(o.created_at), 3600)) AS bucket_start,
    COALESCE(o.platform, 'unknown') AS platform,
    o.group_id AS group_id,
    0 AS success_count,
    SUM(CASE WHEN COALESCE(NULLIF(o.upstream_status_code, 0), o.status_code, 0) >= 400 THEN 1 ELSE 0 END),
    SUM(CASE WHEN COALESCE(NULLIF(o.upstream_status_code, 0), o.status_code, 0) >= 400 AND o.is_business_limited THEN 1 ELSE 0 END),
    SUM(CASE WHEN COALESCE(NULLIF(o.upstream_status_code, 0), o.status_code, 0) >= 400 AND NOT o.is_business_limited THEN 1 ELSE 0 END),
    SUM(CASE WHEN o.error_owner = 'provider' AND NOT o.is_business_limited AND COALESCE(NULLIF(o.upstream_status_code, 0), o.status_code, 0) NOT IN (429,529) THEN 1 ELSE 0 END),
    SUM(CASE WHEN o.error_owner = 'provider' AND NOT o.is_business_limited AND COALESCE(NULLIF(o.upstream_status_code, 0), o.status_code, 0) = 429 THEN 1 ELSE 0 END),
    SUM(CASE WHEN o.error_owner = 'provider' AND NOT o.is_business_limited AND COALESCE(NULLIF(o.upstream_status_code, 0), o.status_code, 0) = 529 THEN 1 ELSE 0 END),
    0,0,0,0,0,0
  FROM ops_error_logs o
  WHERE o.created_at >= ? AND o.created_at < ? AND o.is_count_tokens = FALSE AND o.group_id IS NOT NULL
  GROUP BY 1,2,3
) x
WHERE x.bucket_start IS NOT NULL
GROUP BY x.bucket_start, x.platform, x.group_id
ON DUPLICATE KEY UPDATE
  success_count = VALUES(success_count),
  error_count_total = VALUES(error_count_total),
  business_limited_count = VALUES(business_limited_count),
  error_count_sla = VALUES(error_count_sla),
  upstream_error_count_excl_429_529 = VALUES(upstream_error_count_excl_429_529),
  upstream_429_count = VALUES(upstream_429_count),
  upstream_529_count = VALUES(upstream_529_count),
  token_consumed = VALUES(token_consumed),
  duration_p50_ms = VALUES(duration_p50_ms),
  duration_p90_ms = VALUES(duration_p90_ms),
  duration_p95_ms = VALUES(duration_p95_ms),
  duration_p99_ms = VALUES(duration_p99_ms),
  duration_avg_ms = VALUES(duration_avg_ms),
  duration_max_ms = VALUES(duration_max_ms),
  ttft_p50_ms = VALUES(ttft_p50_ms),
  ttft_p90_ms = VALUES(ttft_p90_ms),
  ttft_p95_ms = VALUES(ttft_p95_ms),
  ttft_p99_ms = VALUES(ttft_p99_ms),
  ttft_avg_ms = VALUES(ttft_avg_ms),
  ttft_max_ms = VALUES(ttft_max_ms),
  ttft_sample_count = VALUES(ttft_sample_count),
  computed_at = VALUES(computed_at)
`, quotedGroupsTable, quotedGroupsTable)

	_, err := r.db.ExecContext(ctx, q,
		start, end,
		start, end,
		start, end,
		start, end,
		start, end,
		start, end,
	)
	return err
}

func (r *opsRepository) UpsertDailyMetrics(ctx context.Context, startTime, endTime time.Time) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("nil ops repository")
	}
	if startTime.IsZero() || endTime.IsZero() || !endTime.After(startTime) {
		return nil
	}

	start := startTime.UTC()
	end := endTime.UTC()

	q := `
INSERT INTO ops_metrics_daily (
  bucket_date,
  platform,
  group_id,
  success_count,
  error_count_total,
  business_limited_count,
  error_count_sla,
  upstream_error_count_excl_429_529,
  upstream_429_count,
  upstream_529_count,
  token_consumed,
  duration_p50_ms,
  duration_p90_ms,
  duration_p95_ms,
  duration_p99_ms,
  duration_avg_ms,
  duration_max_ms,
  ttft_p50_ms,
  ttft_p90_ms,
  ttft_p95_ms,
  ttft_p99_ms,
  ttft_avg_ms,
  ttft_max_ms,
  ttft_sample_count,
  computed_at
)
SELECT
  DATE(bucket_start) AS bucket_date,
  platform,
  group_id,
  COALESCE(SUM(success_count), 0),
  COALESCE(SUM(error_count_total), 0),
  COALESCE(SUM(business_limited_count), 0),
  COALESCE(SUM(error_count_sla), 0),
  COALESCE(SUM(upstream_error_count_excl_429_529), 0),
  COALESCE(SUM(upstream_429_count), 0),
  COALESCE(SUM(upstream_529_count), 0),
  COALESCE(SUM(token_consumed), 0),
  NULL,
  NULL,
  MAX(duration_p95_ms),
  MAX(duration_p99_ms),
  CAST(AVG(NULLIF(duration_avg_ms, 0)) AS DECIMAL(10,2)),
  MAX(duration_max_ms),
  NULL,
  NULL,
  MAX(ttft_p95_ms),
  MAX(ttft_p99_ms),
  CAST(SUM(ttft_avg_ms * ttft_sample_count) / NULLIF(SUM(ttft_sample_count), 0) AS DECIMAL(10,2)),
  MAX(ttft_max_ms),
  COALESCE(SUM(ttft_sample_count), 0),
  NOW()
FROM ops_metrics_hourly
WHERE bucket_start >= ? AND bucket_start < ?
GROUP BY DATE(bucket_start), platform, group_id
ON DUPLICATE KEY UPDATE
  success_count = VALUES(success_count),
  error_count_total = VALUES(error_count_total),
  business_limited_count = VALUES(business_limited_count),
  error_count_sla = VALUES(error_count_sla),
  upstream_error_count_excl_429_529 = VALUES(upstream_error_count_excl_429_529),
  upstream_429_count = VALUES(upstream_429_count),
  upstream_529_count = VALUES(upstream_529_count),
  token_consumed = VALUES(token_consumed),
  duration_p50_ms = VALUES(duration_p50_ms),
  duration_p90_ms = VALUES(duration_p90_ms),
  duration_p95_ms = VALUES(duration_p95_ms),
  duration_p99_ms = VALUES(duration_p99_ms),
  duration_avg_ms = VALUES(duration_avg_ms),
  duration_max_ms = VALUES(duration_max_ms),
  ttft_p50_ms = VALUES(ttft_p50_ms),
  ttft_p90_ms = VALUES(ttft_p90_ms),
  ttft_p95_ms = VALUES(ttft_p95_ms),
  ttft_p99_ms = VALUES(ttft_p99_ms),
  ttft_avg_ms = VALUES(ttft_avg_ms),
  ttft_max_ms = VALUES(ttft_max_ms),
  ttft_sample_count = VALUES(ttft_sample_count),
  computed_at = VALUES(computed_at)
`

	_, err := r.db.ExecContext(ctx, q, start, end)
	return err
}

func (r *opsRepository) GetLatestHourlyBucketStart(ctx context.Context) (time.Time, bool, error) {
	if r == nil || r.db == nil {
		return time.Time{}, false, fmt.Errorf("nil ops repository")
	}

	var value sql.NullTime
	if err := r.db.QueryRowContext(ctx, `SELECT MAX(bucket_start) FROM ops_metrics_hourly`).Scan(&value); err != nil {
		return time.Time{}, false, err
	}
	if !value.Valid {
		return time.Time{}, false, nil
	}
	return value.Time.UTC(), true, nil
}

func (r *opsRepository) GetLatestDailyBucketDate(ctx context.Context) (time.Time, bool, error) {
	if r == nil || r.db == nil {
		return time.Time{}, false, fmt.Errorf("nil ops repository")
	}

	var value sql.NullTime
	if err := r.db.QueryRowContext(ctx, `SELECT MAX(bucket_date) FROM ops_metrics_daily`).Scan(&value); err != nil {
		return time.Time{}, false, err
	}
	if !value.Valid {
		return time.Time{}, false, nil
	}
	t := value.Time.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), true, nil
}
