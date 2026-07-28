package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *opsRepository) ListRequestDetails(ctx context.Context, filter *service.OpsRequestDetailFilter) ([]*service.OpsRequestDetail, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, fmt.Errorf("nil ops repository")
	}

	page, pageSize, startTime, endTime := filter.Normalize()
	offset := (page - 1) * pageSize

	conditions := make([]string, 0, 16)
	filterArgs := make([]any, 0, 20)

	cteArgs := []any{
		startTime.UTC(), endTime.UTC(),
		startTime.UTC(), endTime.UTC(),
	}

	addCondition := func(condition string, values ...any) {
		conditions = append(conditions, condition)
		filterArgs = append(filterArgs, values...)
	}

	if filter != nil {
		if kind := strings.TrimSpace(strings.ToLower(filter.Kind)); kind != "" && kind != "all" {
			if kind != string(service.OpsRequestKindSuccess) && kind != string(service.OpsRequestKindError) {
				return nil, 0, fmt.Errorf("invalid kind")
			}
			addCondition("kind = ?", kind)
		}

		if platform := strings.TrimSpace(strings.ToLower(filter.Platform)); platform != "" {
			addCondition("platform = ?", platform)
		}
		if filter.GroupID != nil && *filter.GroupID > 0 {
			addCondition("group_id = ?", *filter.GroupID)
		}

		if filter.UserID != nil && *filter.UserID > 0 {
			addCondition("user_id = ?", *filter.UserID)
		}
		if filter.APIKeyID != nil && *filter.APIKeyID > 0 {
			addCondition("api_key_id = ?", *filter.APIKeyID)
		}
		if filter.AccountID != nil && *filter.AccountID > 0 {
			addCondition("account_id = ?", *filter.AccountID)
		}

		if model := strings.TrimSpace(filter.Model); model != "" {
			addCondition("model = ?", model)
		}
		if requestID := strings.TrimSpace(filter.RequestID); requestID != "" {
			addCondition("request_id = ?", requestID)
		}
		if q := strings.TrimSpace(filter.Query); q != "" {
			like := "%" + strings.ToLower(q) + "%"
			addCondition(
				"(LOWER(COALESCE(request_id,'')) LIKE ? OR LOWER(COALESCE(model,'')) LIKE ? OR LOWER(COALESCE(message,'')) LIKE ?)",
				like, like, like,
			)
		}

		if filter.MinDurationMs != nil {
			addCondition("duration_ms >= ?", *filter.MinDurationMs)
		}
		if filter.MaxDurationMs != nil {
			addCondition("duration_ms <= ?", *filter.MaxDurationMs)
		}
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	cte := fmt.Sprintf(`
WITH combined AS (
  SELECT
    'success' AS kind,
    ul.created_at AS created_at,
    ul.request_id AS request_id,
    COALESCE(NULLIF(g.platform, ''), NULLIF(a.platform, ''), '') AS platform,
    ul.model AS model,
    ul.duration_ms AS duration_ms,
    200 AS status_code,
    NULL AS error_id,
    NULL AS phase,
    NULL AS severity,
    NULL AS message,
    ul.user_id AS user_id,
    ul.api_key_id AS api_key_id,
    ul.account_id AS account_id,
    COALESCE(a.name, '') AS account_name,
    ul.group_id AS group_id,
    ul.stream AS stream
  FROM usage_logs ul
  LEFT JOIN %s g ON g.id = ul.group_id
  LEFT JOIN accounts a ON a.id = ul.account_id
  WHERE ul.created_at >= ? AND ul.created_at < ?

  UNION ALL

  SELECT
    'error' AS kind,
    o.created_at AS created_at,
    COALESCE(NULLIF(o.request_id,''), NULLIF(o.client_request_id,''), '') AS request_id,
    COALESCE(NULLIF(o.platform, ''), NULLIF(g.platform, ''), NULLIF(a.platform, ''), '') AS platform,
    o.model AS model,
    o.duration_ms AS duration_ms,
    COALESCE(NULLIF(o.upstream_status_code, 0), o.status_code, 0) AS status_code,
    o.id AS error_id,
    o.error_phase AS phase,
    o.severity AS severity,
    o.error_message AS message,
    o.user_id AS user_id,
    o.api_key_id AS api_key_id,
    o.account_id AS account_id,
    COALESCE(a.name, '') AS account_name,
    o.group_id AS group_id,
    o.stream AS stream
  FROM ops_error_logs o
  LEFT JOIN %s g ON g.id = o.group_id
  LEFT JOIN accounts a ON a.id = o.account_id
  WHERE o.created_at >= ? AND o.created_at < ?
    AND COALESCE(NULLIF(o.upstream_status_code, 0), o.status_code, 0) >= 400
)
`, quotedGroupsTable, quotedGroupsTable)

	countQuery := fmt.Sprintf(`%s SELECT COUNT(1) FROM combined %s`, cte, where)
	countArgs := append(append([]any{}, cteArgs...), filterArgs...)

	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		if err == sql.ErrNoRows {
			total = 0
		} else {
			return nil, 0, err
		}
	}

	sort := "ORDER BY created_at DESC"
	if filter != nil {
		switch strings.TrimSpace(strings.ToLower(filter.Sort)) {
		case "", "created_at_desc":
			// default
		case "duration_desc":
			sort = "ORDER BY (duration_ms IS NULL) ASC, duration_ms DESC, created_at DESC"
		default:
			return nil, 0, fmt.Errorf("invalid sort")
		}
	}

	listQuery := fmt.Sprintf(`
%s
SELECT
  kind,
  created_at,
  request_id,
  platform,
  model,
  duration_ms,
  status_code,
  error_id,
  phase,
  severity,
  message,
  user_id,
  api_key_id,
  account_id,
  account_name,
  group_id,
  stream
FROM combined
%s
%s
LIMIT ? OFFSET ?
`, cte, where, sort)

	listArgs := append(append(append([]any{}, cteArgs...), filterArgs...), pageSize, offset)
	rows, err := r.db.QueryContext(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	toIntPtr := func(v sql.NullInt64) *int {
		if !v.Valid {
			return nil
		}
		i := int(v.Int64)
		return &i
	}
	toInt64Ptr := func(v sql.NullInt64) *int64 {
		if !v.Valid {
			return nil
		}
		i := v.Int64
		return &i
	}

	out := make([]*service.OpsRequestDetail, 0, pageSize)
	for rows.Next() {
		var (
			kind      string
			createdAt time.Time
			requestID sql.NullString
			platform  sql.NullString
			model     sql.NullString

			durationMs sql.NullInt64
			statusCode sql.NullInt64
			errorID    sql.NullInt64

			phase    sql.NullString
			severity sql.NullString
			message  sql.NullString

			userID      sql.NullInt64
			apiKeyID    sql.NullInt64
			accountID   sql.NullInt64
			accountName sql.NullString
			groupID     sql.NullInt64

			stream bool
		)

		if err := rows.Scan(
			&kind,
			&createdAt,
			&requestID,
			&platform,
			&model,
			&durationMs,
			&statusCode,
			&errorID,
			&phase,
			&severity,
			&message,
			&userID,
			&apiKeyID,
			&accountID,
			&accountName,
			&groupID,
			&stream,
		); err != nil {
			return nil, 0, err
		}

		item := &service.OpsRequestDetail{
			Kind:      service.OpsRequestKind(kind),
			CreatedAt: createdAt,
			RequestID: strings.TrimSpace(requestID.String),
			Platform:  strings.TrimSpace(platform.String),
			Model:     strings.TrimSpace(model.String),

			DurationMs: toIntPtr(durationMs),
			StatusCode: toIntPtr(statusCode),
			ErrorID:    toInt64Ptr(errorID),
			Phase:      phase.String,
			Severity:   severity.String,
			Message:    message.String,

			UserID:      toInt64Ptr(userID),
			APIKeyID:    toInt64Ptr(apiKeyID),
			AccountID:   toInt64Ptr(accountID),
			AccountName: strings.TrimSpace(accountName.String),
			GroupID:     toInt64Ptr(groupID),

			Stream: stream,
		}

		if item.Platform == "" {
			item.Platform = "unknown"
		}

		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return out, total, nil
}
