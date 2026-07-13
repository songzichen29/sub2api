package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestContentModerationRepositoryListLogs_UsesMySQLPlaceholders(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := &contentModerationRepository{db: db}
	filter := service.ContentModerationLogFilter{
		Pagination: pagination.PaginationParams{
			Page:      1,
			PageSize:  20,
			SortOrder: pagination.SortOrderDesc,
		},
		Search: "abc",
	}

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM content_moderation_logs l WHERE l.id IS NOT NULL AND (l.request_id LIKE ? OR l.user_email LIKE ? OR l.api_key_name LIKE ? OR l.model LIKE ? OR l.input_excerpt LIKE ?)")).
		WithArgs("%abc%", "%abc%", "%abc%", "%abc%", "%abc%").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT
    l.id, l.request_id, l.user_id, l.user_email, l.api_key_id, l.api_key_name, l.group_id, l.group_name,
    l.endpoint, l.provider, l.model, l.mode, l.action, l.flagged, l.highest_category, l.highest_score,
    l.category_scores, l.threshold_snapshot, l.input_excerpt, l.upstream_latency_ms, l.error,
    l.violation_count, l.auto_banned, l.email_sent, COALESCE(u.status, ''), l.queue_delay_ms, l.matched_keyword, l.created_at,
    CASE WHEN l.request_body IS NULL OR l.request_body = '' THEN 0 ELSE OCTET_LENGTH(l.request_body) END,
    COALESCE(l.request_body_message_count, 0)
FROM content_moderation_logs l
LEFT JOIN users u ON u.id = l.user_id WHERE l.id IS NOT NULL AND (l.request_id LIKE ? OR l.user_email LIKE ? OR l.api_key_name LIKE ? OR l.model LIKE ? OR l.input_excerpt LIKE ?)
ORDER BY l.created_at DESC, l.id DESC
LIMIT ? OFFSET ?`)).
		WithArgs("%abc%", "%abc%", "%abc%", "%abc%", "%abc%", 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "request_id", "user_id", "user_email", "api_key_id", "api_key_name", "group_id", "group_name",
			"endpoint", "provider", "model", "mode", "action", "flagged", "highest_category", "highest_score",
			"category_scores", "threshold_snapshot", "input_excerpt", "upstream_latency_ms", "error",
			"violation_count", "auto_banned", "email_sent", "status", "queue_delay_ms", "matched_keyword", "created_at",
			"request_body_size", "request_body_message_count",
		}).AddRow(
			int64(1), "req-abc", int64(2), "user@example.com", int64(3), "key-abc", int64(4), "default",
			"/v1/responses", "openai", "gpt-5", "observe", "allow", false, "", float64(0),
			[]byte(`{}`), []byte(`{}`), "abc", nil, "", 0, false, false, "active", nil, "", time.Now(),
			0, 0,
		))

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT ul.id, ul.request_id, ul.api_key_id, ul.account_id, COALESCE(a.name, '')
FROM usage_logs ul
LEFT JOIN accounts a ON a.id = ul.account_id
WHERE ul.request_id IN (?,?)
ORDER BY ul.id DESC`)).
		WithArgs("req-abc", "local:req-abc").
		WillReturnRows(sqlmock.NewRows([]string{"id", "request_id", "api_key_id", "account_id", "account_name"}).
			AddRow(int64(10), "local:req-abc", int64(3), int64(42), "legacy-account").
			AddRow(int64(9), "req-abc", int64(3), int64(43), "upstream-account"))

	items, pageResult, err := repo.ListLogs(context.Background(), filter)
	if err != nil {
		t.Fatalf("ListLogs error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].AccountID == nil || *items[0].AccountID != 43 || items[0].AccountName != "upstream-account" {
		t.Fatalf("unexpected account fields: %+v", items[0])
	}
	if pageResult == nil || pageResult.Total != 1 || pageResult.Page != 1 || pageResult.PageSize != 20 {
		t.Fatalf("unexpected page result: %+v", pageResult)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestContentModerationRepositoryCreateLog_UsesExecInsert(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := &contentModerationRepository{db: db}
	logEntry := &service.ContentModerationLog{
		RequestID:         "req-1",
		UserEmail:         "u@example.com",
		APIKeyName:        "key-a",
		GroupName:         "default",
		Endpoint:          "/v1/chat/completions",
		Provider:          "openai",
		Model:             "gpt-4.1",
		Mode:              "observe",
		Action:            "allow",
		Flagged:           false,
		HighestCategory:   "",
		HighestScore:      0,
		CategoryScores:    map[string]float64{},
		ThresholdSnapshot: map[string]float64{},
		InputExcerpt:      "hello",
		Error:             "",
		ViolationCount:    0,
		AutoBanned:        false,
		EmailSent:         false,
	}

	mock.ExpectExec(regexp.QuoteMeta(`
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
)`)).
		WithArgs(
			"req-1", nil, "u@example.com", nil, "key-a", nil, "default",
			"/v1/chat/completions", "openai", "gpt-4.1", "observe", "allow", false, "", float64(0),
			"{}", "{}", "hello", nil, 0, nil, "",
			0, false, false, nil, "",
		).
		WillReturnResult(sqlmock.NewResult(12, 1))

	if err := repo.CreateLog(context.Background(), logEntry); err != nil {
		t.Fatalf("CreateLog error: %v", err)
	}
	if logEntry.ID != 12 {
		t.Fatalf("expected log ID 12, got %d", logEntry.ID)
	}
	if logEntry.CreatedAt.IsZero() {
		t.Fatalf("expected CreatedAt to be set")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestContentModerationRepositoryGetLogRequestBody(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := &contentModerationRepository{db: db}
	createdAt := time.Date(2026, 6, 21, 10, 30, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, request_id, request_body, created_at
FROM content_moderation_logs
WHERE id = ? AND request_body IS NOT NULL AND request_body <> ''
LIMIT 1
`)).
		WithArgs(int64(12)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "request_id", "request_body", "created_at"}).
			AddRow(int64(12), "req-1", `{"kind":"content_moderation_request_session"}`, createdAt))

	result, err := repo.GetLogRequestBody(context.Background(), 12)
	if err != nil {
		t.Fatalf("GetLogRequestBody error: %v", err)
	}
	if result == nil || result.ID != 12 || result.RequestID != "req-1" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Content == "" || result.Size != len([]byte(result.Content)) {
		t.Fatalf("unexpected content metadata: %+v", result)
	}
	if result.Filename == "" {
		t.Fatalf("expected filename")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestContentModerationRepositoryCountFlaggedByUserSince_UsesMySQLFallbackTimestamp(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	repo := &contentModerationRepository{db: db}
	since := time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(`
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
`)).
		WithArgs(int64(7), int64(7), false, since).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	count, err := repo.CountFlaggedByUserSince(context.Background(), 7, since, false)
	if err != nil {
		t.Fatalf("CountFlaggedByUserSince error: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected count 3, got %d", count)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}
