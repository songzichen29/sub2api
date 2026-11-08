package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Metric-only statistics: do not change the billing/dashboard success filters.
// Window functions and CASE keep this query portable across PostgreSQL/MySQL 8.
// Terminal failures override partial HTTP output; WS connection errors retain
// already completed turns. No request-count cap truncates the rolling hour.
func accountMetricSnapshotsSQL(count int, mysql bool) string {
	ids := strings.TrimSuffix(strings.Repeat("?,", count), ",")
	localID, clientID := "'local:' || e.request_id", "'client:' || e.client_request_id"
	if mysql {
		localID, clientID = "CONCAT('local:', e.request_id)", "CONCAT('client:', e.client_request_id)"
	}
	return fmt.Sprintf(`WITH error_rows AS (
 SELECT e.id,e.account_id,e.api_key_id,e.request_id,e.client_request_id,e.request_type,e.created_at,
 ROW_NUMBER() OVER (PARTITION BY account_id, COALESCE(api_key_id,0),
   COALESCE(NULLIF(request_id,''),NULLIF(client_request_id,'')),
   CASE WHEN COALESCE(NULLIF(request_id,''),NULLIF(client_request_id,'')) IS NULL THEN id ELSE 0 END
   ORDER BY created_at DESC,id DESC) AS metric_rank
 FROM ops_error_logs e
 WHERE e.account_id IN (%s) AND e.created_at >= ? AND e.created_at < ?
   AND COALESCE(e.is_count_tokens,FALSE)=FALSE AND COALESCE(e.request_type,0) NOT IN (4,6)
   AND COALESCE(e.is_business_limited,FALSE)=FALSE AND e.error_owner='provider'
   AND COALESCE(NULLIF(e.upstream_status_code,0),e.status_code,0)>=400
), errors AS (SELECT * FROM error_rows WHERE metric_rank=1), usage_rows AS (
 SELECT ul.account_id,ul.input_tokens,ul.image_input_tokens,ul.cache_creation_tokens,ul.cache_read_tokens,
 ul.first_token_ms,ul.duration_ms,ul.request_type,ul.stream,ul.openai_ws_mode,
 ROW_NUMBER() OVER (PARTITION BY ul.account_id,COALESCE(ul.api_key_id,0),NULLIF(ul.request_id,''),
   CASE WHEN NULLIF(ul.request_id,'') IS NULL THEN ul.id ELSE 0 END
   ORDER BY ul.created_at DESC,ul.id DESC) AS metric_rank
 FROM usage_logs ul
 WHERE ul.account_id IN (%s) AND ul.created_at >= ? AND ul.created_at < ?
   AND COALESCE(ul.request_type,0) NOT IN (4,6)
   AND (ul.actual_cost>0 OR ul.input_tokens>0 OR ul.output_tokens>0 OR ul.cache_creation_tokens>0 OR ul.cache_read_tokens>0)
   AND (ul.request_type=3 OR ul.openai_ws_mode=TRUE OR NOT EXISTS (
     SELECT 1 FROM errors e WHERE e.account_id=ul.account_id AND COALESCE(e.api_key_id,0)=COALESCE(ul.api_key_id,0)
       AND COALESCE(e.request_type,0)<>3
       AND NULLIF(ul.request_id,'') IN (NULLIF(e.request_id,''),NULLIF(e.client_request_id,''),%s,%s)
   ))
), measurements AS (
 SELECT ul.*,
   (CASE WHEN input_tokens-COALESCE(image_input_tokens,0)>0 THEN input_tokens-COALESCE(image_input_tokens,0) ELSE 0 END)
     + cache_creation_tokens + cache_read_tokens AS prompt_tokens,
   CASE WHEN COALESCE(request_type,0) NOT IN (1,4,6) AND (stream=TRUE OR openai_ws_mode=TRUE OR request_type IN (2,3))
     AND input_tokens>=0 AND cache_creation_tokens>=0 AND cache_read_tokens>=0 AND COALESCE(image_input_tokens,0)>=0 THEN 1 ELSE 0 END AS cache_eligible,
   CASE WHEN (CASE WHEN request_type IN (1,2,3,4,5,6) THEN request_type
     WHEN openai_ws_mode=TRUE THEN 3 WHEN stream=TRUE THEN 2 ELSE 1 END) IN (2,3) THEN 1 ELSE 0 END AS ttft_eligible
 FROM usage_rows ul WHERE metric_rank=1
), observations AS (
 SELECT account_id, 1 AS success, 0 AS failure,
   CASE WHEN cache_eligible=1 AND prompt_tokens>0 THEN cache_read_tokens ELSE 0 END AS cached,
   CASE WHEN cache_eligible=1 AND prompt_tokens>0 THEN prompt_tokens ELSE 0 END AS prompt,
   CASE WHEN cache_eligible=1 AND prompt_tokens>0 THEN 1 ELSE 0 END AS cache_sample,
   CASE WHEN ttft_eligible=1 AND first_token_ms>=0 THEN first_token_ms END AS ttft,
   CASE WHEN duration_ms>=0 THEN duration_ms END AS duration
 FROM measurements
 UNION ALL
 SELECT account_id,0,1,0,0,0,NULL,NULL FROM errors
)
SELECT account_id,SUM(success),SUM(failure),
 SUM(cached)*1.0/NULLIF(SUM(prompt),0),SUM(cache_sample),AVG(ttft),COUNT(ttft),AVG(duration),COUNT(duration)
FROM observations GROUP BY account_id`, ids, ids, localID, clientID)
}

func (r *accountMetricRepository) GetAccountMetricSnapshot(ctx context.Context, accountID int64, start, end time.Time) (*service.AccountMetricSnapshot, error) {
	rows, err := r.GetAccountMetricSnapshots(ctx, []int64{accountID}, start, end)
	if err != nil {
		return nil, err
	}
	return rows[accountID], nil
}

func (r *accountMetricRepository) GetAccountMetricSnapshots(ctx context.Context, ids []int64, start, end time.Time) (map[int64]*service.AccountMetricSnapshot, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("nil account metric repository")
	}
	if len(ids) > 100 {
		return nil, fmt.Errorf("at most 100 accounts per metric batch")
	}
	start, end = start.UTC(), end.UTC()
	if start.IsZero() || end.IsZero() || !start.Before(end) {
		return nil, fmt.Errorf("invalid account metric window")
	}
	result := make(map[int64]*service.AccountMetricSnapshot, len(ids))
	args := make([]any, 0, len(ids)*2+4)
	for _, id := range ids {
		if id <= 0 {
			return nil, fmt.Errorf("account id must be positive")
		}
		result[id] = &service.AccountMetricSnapshot{AccountID: id, WindowStart: start, WindowEnd: end}
		args = append(args, id)
	}
	if len(ids) == 0 {
		return result, nil
	}
	args = append(args, start, end)
	args = append(args, args...)
	rows, err := r.db.QueryContext(ctx, r.query(accountMetricSnapshotsSQL(len(ids), r.mysql)), args...)
	if err != nil {
		return nil, fmt.Errorf("aggregate account metrics: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		entry := &service.AccountMetricSnapshot{WindowStart: start, WindowEnd: end}
		if err := rows.Scan(&entry.AccountID, &entry.SuccessSamples, &entry.ErrorSamples, &entry.CacheRate, &entry.CacheSamples,
			&entry.AverageTTFTMs, &entry.TTFTSamples, &entry.AverageDurationMs, &entry.DurationSamples); err != nil {
			return nil, err
		}
		if total := entry.SuccessSamples + entry.ErrorSamples; total > 0 {
			rate := float64(entry.SuccessSamples) / float64(total)
			entry.SuccessRate = &rate
		}
		result[entry.AccountID] = entry
	}
	return result, rows.Err()
}

func (r *accountMetricRepository) GetAccountPerformanceMonitors(ctx context.Context, ids []int64) (map[int64]*service.AccountPerformanceMonitor, error) {
	result := make(map[int64]*service.AccountPerformanceMonitor, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	if r == nil || r.db == nil || len(ids) > 100 {
		return nil, fmt.Errorf("invalid account monitor batch")
	}
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, fmt.Errorf("account id must be positive")
		}
		result[id] = &service.AccountPerformanceMonitor{AccountID: id}
		args = append(args, id)
	}
	rows, err := r.db.QueryContext(ctx, r.query("SELECT "+accountPerformanceMonitorColumns+" FROM account_performance_monitors WHERE account_id IN ("+strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")+")"), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		monitor, err := scanAccountPerformanceMonitor(rows)
		if err != nil {
			return nil, err
		}
		result[monitor.AccountID] = monitor
	}
	return result, rows.Err()
}
