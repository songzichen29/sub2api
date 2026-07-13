package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type contentModerationRepository struct {
	db *sql.DB
}

func NewContentModerationRepository(db *sql.DB) service.ContentModerationRepository {
	return &contentModerationRepository{db: db}
}

func (r *contentModerationRepository) CreateLog(ctx context.Context, log *service.ContentModerationLog) error {
	if log == nil {
		return nil
	}
	categoryScores, err := json.Marshal(log.CategoryScores)
	if err != nil {
		return fmt.Errorf("marshal moderation category scores: %w", err)
	}
	thresholdSnapshot, err := json.Marshal(log.ThresholdSnapshot)
	if err != nil {
		return fmt.Errorf("marshal moderation thresholds: %w", err)
	}
	var userID any
	if log.UserID != nil {
		userID = *log.UserID
	}
	var apiKeyID any
	if log.APIKeyID != nil {
		apiKeyID = *log.APIKeyID
	}
	var groupID any
	if log.GroupID != nil {
		groupID = *log.GroupID
	}
	var latency any
	if log.UpstreamLatencyMS != nil {
		latency = *log.UpstreamLatencyMS
	}
	result, err := r.db.ExecContext(ctx, `
INSERT INTO content_moderation_logs (
    request_id, user_id, user_email, api_key_id, api_key_name, group_id, group_name,
    endpoint, provider, model, mode, action, flagged, highest_category, highest_score,
    category_scores, threshold_snapshot, input_excerpt, request_body, request_body_message_count, upstream_latency_ms, error,
    violation_count, auto_banned, email_sent, queue_delay_ms, matched_keyword
) VALUES (
    ?, ?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?
)`,
		log.RequestID, userID, log.UserEmail, apiKeyID, log.APIKeyName, groupID, log.GroupName,
		log.Endpoint, log.Provider, log.Model, log.Mode, log.Action, log.Flagged, log.HighestCategory, log.HighestScore,
		string(categoryScores), string(thresholdSnapshot), log.InputExcerpt, nullableString(log.RequestBody), log.SessionMessageCount, latency, log.Error,
		log.ViolationCount, log.AutoBanned, log.EmailSent, nullableIntPtr(log.QueueDelayMS), log.MatchedKeyword,
	)
	if err != nil {
		return fmt.Errorf("insert content moderation log: %w", err)
	}
	if id, idErr := result.LastInsertId(); idErr == nil {
		log.ID = id
	}
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now()
	}
	return nil
}

func (r *contentModerationRepository) ListLogs(ctx context.Context, filter service.ContentModerationLogFilter) ([]service.ContentModerationLog, *pagination.PaginationResult, error) {
	where, args := buildContentModerationLogWhere(filter)
	whereSQL := "WHERE " + strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM content_moderation_logs l "+whereSQL, args...).Scan(&total); err != nil {
		return nil, nil, fmt.Errorf("count content moderation logs: %w", err)
	}

	params := filter.Pagination
	if params.Page <= 0 {
		params.Page = 1
	}
	if params.PageSize <= 0 {
		params.PageSize = 20
	}
	if params.PageSize > 100 {
		params.PageSize = 100
	}
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, params.Limit(), params.Offset())
	rows, err := r.db.QueryContext(ctx, `
SELECT
    l.id, l.request_id, l.user_id, l.user_email, l.api_key_id, l.api_key_name, l.group_id, l.group_name,
    l.endpoint, l.provider, l.model, l.mode, l.action, l.flagged, l.highest_category, l.highest_score,
    l.category_scores, l.threshold_snapshot, l.input_excerpt, l.upstream_latency_ms, l.error,
    l.violation_count, l.auto_banned, l.email_sent, COALESCE(u.status, ''), l.queue_delay_ms, l.matched_keyword, l.created_at,
    CASE WHEN l.request_body IS NULL OR l.request_body = '' THEN 0 ELSE OCTET_LENGTH(l.request_body) END,
    COALESCE(l.request_body_message_count, 0),
    COALESCE((
        SELECT ul.account_id
        FROM usage_logs ul
        WHERE (ul.request_id = l.request_id OR ul.request_id = CONCAT('local:', l.request_id))
          AND ul.api_key_id = l.api_key_id
        ORDER BY ul.id DESC
        LIMIT 1
    ), 0),
    COALESCE((
        SELECT a.name
        FROM usage_logs ul
        JOIN accounts a ON a.id = ul.account_id
        WHERE (ul.request_id = l.request_id OR ul.request_id = CONCAT('local:', l.request_id))
          AND ul.api_key_id = l.api_key_id
        ORDER BY ul.id DESC
        LIMIT 1
    ), '')
FROM content_moderation_logs l
LEFT JOIN users u ON u.id = l.user_id `+whereSQL+`
ORDER BY l.created_at DESC, l.id DESC
LIMIT ? OFFSET ?`,
		queryArgs...,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("list content moderation logs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]service.ContentModerationLog, 0)
	for rows.Next() {
		var item service.ContentModerationLog
		var userID, apiKeyID, accountID, groupID, latency, queueDelay sql.NullInt64
		var requestBodySize, sessionMessageCount sql.NullInt64
		var scoresRaw, thresholdsRaw []byte
		if err := rows.Scan(
			&item.ID,
			&item.RequestID,
			&userID,
			&item.UserEmail,
			&apiKeyID,
			&item.APIKeyName,
			&groupID,
			&item.GroupName,
			&item.Endpoint,
			&item.Provider,
			&item.Model,
			&item.Mode,
			&item.Action,
			&item.Flagged,
			&item.HighestCategory,
			&item.HighestScore,
			&scoresRaw,
			&thresholdsRaw,
			&item.InputExcerpt,
			&latency,
			&item.Error,
			&item.ViolationCount,
			&item.AutoBanned,
			&item.EmailSent,
			&item.UserStatus,
			&queueDelay,
			&item.MatchedKeyword,
			&item.CreatedAt,
			&requestBodySize,
			&sessionMessageCount,
			&accountID,
			&item.AccountName,
		); err != nil {
			return nil, nil, fmt.Errorf("scan content moderation log: %w", err)
		}
		if userID.Valid {
			v := userID.Int64
			item.UserID = &v
		}
		if apiKeyID.Valid {
			v := apiKeyID.Int64
			item.APIKeyID = &v
		}
		if accountID.Valid && accountID.Int64 > 0 {
			v := accountID.Int64
			item.AccountID = &v
		}
		if groupID.Valid {
			v := groupID.Int64
			item.GroupID = &v
		}
		if latency.Valid {
			v := int(latency.Int64)
			item.UpstreamLatencyMS = &v
		}
		if queueDelay.Valid {
			v := int(queueDelay.Int64)
			item.QueueDelayMS = &v
		}
		if requestBodySize.Valid && requestBodySize.Int64 > 0 {
			item.HasRequestBody = true
			item.RequestBodySize = int(requestBodySize.Int64)
		}
		if sessionMessageCount.Valid && sessionMessageCount.Int64 > 0 {
			item.SessionMessageCount = int(sessionMessageCount.Int64)
		}
		item.CategoryScores = map[string]float64{}
		_ = json.Unmarshal(scoresRaw, &item.CategoryScores)
		item.ThresholdSnapshot = map[string]float64{}
		_ = json.Unmarshal(thresholdsRaw, &item.ThresholdSnapshot)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate content moderation logs: %w", err)
	}
	return items, paginationResultFromTotal(total, params), nil
}

func (r *contentModerationRepository) GetLogRequestBody(ctx context.Context, id int64) (*service.ContentModerationLogRequestBody, error) {
	if id <= 0 {
		return nil, infraerrors.BadRequest("INVALID_CONTENT_MODERATION_LOG_ID", "风控记录 ID 无效")
	}
	var result service.ContentModerationLogRequestBody
	var body sql.NullString
	err := r.db.QueryRowContext(ctx, `
SELECT id, request_id, request_body, created_at
FROM content_moderation_logs
WHERE id = ? AND request_body IS NOT NULL AND request_body <> ''
LIMIT 1
`, id).Scan(&result.ID, &result.RequestID, &body, &result.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, infraerrors.NotFound("CONTENT_MODERATION_REQUEST_BODY_NOT_FOUND", "风控记录请求正文不存在")
		}
		return nil, fmt.Errorf("get content moderation request body: %w", err)
	}
	if !body.Valid || strings.TrimSpace(body.String) == "" {
		return nil, infraerrors.NotFound("CONTENT_MODERATION_REQUEST_BODY_NOT_FOUND", "风控记录请求正文不存在")
	}
	result.Content = body.String
	result.ContentType = "application/json;charset=utf-8"
	result.Size = len([]byte(body.String))
	result.Filename = contentModerationRequestBodyFilename(result.ID, result.RequestID, result.CreatedAt)
	return &result, nil
}

func (r *contentModerationRepository) CountFlaggedByUserSince(ctx context.Context, userID int64, since time.Time, excludeCyberPolicy bool) (int, error) {
	if userID <= 0 {
		return 0, nil
	}
	var count int
	err := r.db.QueryRowContext(ctx, `
WITH last_auto_ban AS (
    SELECT MAX(created_at) AS at
    FROM content_moderation_logs
    WHERE user_id = ? AND auto_banned = TRUE
)
SELECT COUNT(*)
FROM content_moderation_logs
WHERE user_id = ?
  AND flagged = TRUE
  AND action <> 'hash_block'
  AND (? = FALSE OR action <> 'cyber_policy')
  AND created_at >= ?
  AND created_at > COALESCE((SELECT at FROM last_auto_ban), '1970-01-01 00:00:00')
`, userID, userID, excludeCyberPolicy, since).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count user content moderation flagged logs: %w", err)
	}
	return count, nil
}

func (r *contentModerationRepository) UpdateLogEmailSent(ctx context.Context, id int64, sent bool) error {
	_, err := r.db.ExecContext(ctx, `UPDATE content_moderation_logs SET email_sent = ? WHERE id = ?`, sent, id)
	if err != nil {
		return fmt.Errorf("update content moderation log email_sent: %w", err)
	}
	return nil
}

func (r *contentModerationRepository) CleanupExpiredLogs(ctx context.Context, hitBefore time.Time, nonHitBefore time.Time) (*service.ContentModerationCleanupResult, error) {
	result := &service.ContentModerationCleanupResult{FinishedAt: time.Now()}
	if r == nil || r.db == nil {
		return result, nil
	}
	hitExec, err := r.db.ExecContext(ctx, `
DELETE FROM content_moderation_logs
WHERE flagged = TRUE AND created_at < ?
`, hitBefore)
	if err != nil {
		return nil, fmt.Errorf("delete expired hit content moderation logs: %w", err)
	}
	result.DeletedHit, _ = hitExec.RowsAffected()

	nonHitExec, err := r.db.ExecContext(ctx, `
DELETE FROM content_moderation_logs
WHERE flagged = FALSE AND created_at < ?
`, nonHitBefore)
	if err != nil {
		return nil, fmt.Errorf("delete expired non-hit content moderation logs: %w", err)
	}
	result.DeletedNonHit, _ = nonHitExec.RowsAffected()

	result.FinishedAt = time.Now()
	return result, nil
}

func nullableIntPtr(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func contentModerationRequestBodyFilename(id int64, requestID string, createdAt time.Time) string {
	stamp := createdAt.UTC().Format("20060102T150405Z")
	if createdAt.IsZero() {
		stamp = time.Now().UTC().Format("20060102T150405Z")
	}
	requestID = sanitizeFilenameComponent(requestID)
	if requestID == "" {
		requestID = fmt.Sprintf("log-%d", id)
	}
	return fmt.Sprintf("risk-control-%d-%s-%s.json", id, stamp, requestID)
}

func sanitizeFilenameComponent(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-.")
	if len(out) > 80 {
		out = out[:80]
	}
	return out
}

func buildContentModerationLogWhere(filter service.ContentModerationLogFilter) ([]string, []any) {
	where := []string{"l.id IS NOT NULL"}
	args := make([]any, 0)
	add := func(expr string, value any) {
		args = append(args, value)
		where = append(where, expr)
	}
	switch strings.ToLower(strings.TrimSpace(filter.Result)) {
	case "hit", "flagged":
		where = append(where, "l.flagged = TRUE")
	case "blocked", "block":
		where = append(where, "l.action = 'block'")
	case "pass", "allow":
		where = append(where, "l.flagged = FALSE AND l.error = ''")
	case "error":
		where = append(where, "l.error <> ''")
	}
	if filter.GroupID != nil {
		add("l.group_id = ?", *filter.GroupID)
	}
	if endpoint := strings.TrimSpace(filter.Endpoint); endpoint != "" {
		add("l.endpoint = ?", endpoint)
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		like := "%" + search + "%"
		args = append(args, like, like, like, like, like)
		where = append(where, "(l.request_id LIKE ? OR l.user_email LIKE ? OR l.api_key_name LIKE ? OR l.model LIKE ? OR l.input_excerpt LIKE ?)")
	}
	if filter.From != nil && !filter.From.IsZero() {
		add("l.created_at >= ?", *filter.From)
	}
	if filter.To != nil && !filter.To.IsZero() {
		add("l.created_at <= ?", *filter.To)
	}
	return where, args
}
