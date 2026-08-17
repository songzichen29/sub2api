package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type contentModerationHandlerRepo struct {
	body *service.ContentModerationLogRequestBody
	err  error
}

func (r *contentModerationHandlerRepo) CreateLog(ctx context.Context, log *service.ContentModerationLog) error {
	return nil
}

func (r *contentModerationHandlerRepo) UpdateLogAccount(ctx context.Context, requestID string, apiKeyID, accountID int64, accountName string) error {
	return nil
}

func (r *contentModerationHandlerRepo) ListLogs(ctx context.Context, filter service.ContentModerationLogFilter) ([]service.ContentModerationLog, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (r *contentModerationHandlerRepo) GetLogRequestBody(ctx context.Context, id int64) (*service.ContentModerationLogRequestBody, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.body, nil
}

func (r *contentModerationHandlerRepo) CountFlaggedByUserSince(ctx context.Context, userID int64, since time.Time, excludeCyberPolicy bool) (int, error) {
	return 0, nil
}

func (r *contentModerationHandlerRepo) CleanupExpiredLogs(ctx context.Context, hitBefore time.Time, nonHitBefore time.Time) (*service.ContentModerationCleanupResult, error) {
	return &service.ContentModerationCleanupResult{}, nil
}

func (r *contentModerationHandlerRepo) UpdateLogEmailSent(ctx context.Context, id int64, sent bool) error {
	return nil
}

func TestContentModerationHandlerGetLogRequestBodySuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &contentModerationHandlerRepo{body: &service.ContentModerationLogRequestBody{
		ID:          12,
		RequestID:   "req-1",
		Content:     `{"kind":"content_moderation_request_session"}`,
		ContentType: "application/json;charset=utf-8",
		Filename:    "risk-control-12.json",
		Size:        43,
		CreatedAt:   time.Date(2026, 6, 21, 10, 30, 0, 0, time.UTC),
	}}
	svc := service.NewContentModerationService(nil, repo, nil, nil, nil, nil, nil, nil)
	handler := NewContentModerationHandler(svc)

	router := gin.New()
	router.GET("/logs/:id/request-body", handler.GetLogRequestBody)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/logs/12/request-body", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"code":0`)
	require.Contains(t, rec.Body.String(), `"content":"{\"kind\":\"content_moderation_request_session\"}"`)
	require.Contains(t, rec.Body.String(), `"filename":"risk-control-12.json"`)
}

func TestContentModerationHandlerGetLogRequestBodyErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Run("invalid id", func(t *testing.T) {
		svc := service.NewContentModerationService(nil, &contentModerationHandlerRepo{}, nil, nil, nil, nil, nil, nil)
		handler := NewContentModerationHandler(svc)
		router := gin.New()
		router.GET("/logs/:id/request-body", handler.GetLogRequestBody)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/logs/not-a-number/request-body", nil)
		router.ServeHTTP(rec, req)

		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.Contains(t, rec.Body.String(), `"reason":"INVALID_CONTENT_MODERATION_LOG_ID"`)
	})

	t.Run("missing body", func(t *testing.T) {
		repo := &contentModerationHandlerRepo{
			err: infraerrors.NotFound("CONTENT_MODERATION_REQUEST_BODY_NOT_FOUND", "风控记录请求正文不存在"),
		}
		svc := service.NewContentModerationService(nil, repo, nil, nil, nil, nil, nil, nil)
		handler := NewContentModerationHandler(svc)
		router := gin.New()
		router.GET("/logs/:id/request-body", handler.GetLogRequestBody)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/logs/12/request-body", nil)
		router.ServeHTTP(rec, req)

		require.Equal(t, http.StatusNotFound, rec.Code)
		require.Contains(t, rec.Body.String(), `"reason":"CONTENT_MODERATION_REQUEST_BODY_NOT_FOUND"`)
	})
}
