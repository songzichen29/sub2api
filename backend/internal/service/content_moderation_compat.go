package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

func prepareContentModerationRequestAuditBundle(_ ContentModerationCheckInput, _ string, rawBody []byte, flagged bool) (string, int, int) {
	if !flagged || len(rawBody) == 0 {
		return "", 0, 0
	}
	var value any
	if json.Unmarshal(rawBody, &value) != nil {
		return "", 0, 0
	}
	messageCount := 0
	if root, ok := value.(map[string]any); ok {
		if messages, ok := root["messages"].([]any); ok {
			messageCount = len(messages)
			root["boundary_source"] = "messages"
			root["session_start_index"] = 0
			root["session_end_index"] = len(messages) - 1
		}
		redactContentModerationAuditValue(root)
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", 0, 0
	}
	// The raw request size is part of the retention boundary. Redaction can
	// greatly shrink a payload, but that must not hide that the original body
	// exceeded the audit limit.
	if len(rawBody) > maxContentModerationRequestBodyAuditBytes || len(body) > maxContentModerationRequestBodyAuditBytes {
		body = []byte(`{"truncated": true, "boundary_source": "messages", "session_start_index": 0, "session_end_index": 0}`)
		messageCount = 1
	}
	return string(body), len(body), messageCount
}

func redactContentModerationAuditValue(value any) {
	switch item := value.(type) {
	case map[string]any:
		for key, child := range item {
			if text, ok := child.(string); ok {
				item[key] = redactContentModerationSecrets(text)
			} else {
				redactContentModerationAuditValue(child)
			}
		}
	case []any:
		for i := range item {
			redactContentModerationAuditValue(item[i])
		}
	}
}

func (s *ContentModerationService) NewAccountBinding(requestID string, apiKeyID int64) *ContentModerationAccountBinding {
	return &ContentModerationAccountBinding{requestID: strings.TrimSpace(requestID), apiKeyID: apiKeyID}
}

func (s *ContentModerationService) BindSelectedAccount(ctx context.Context, binding *ContentModerationAccountBinding, accountID int64, accountName string) {
	if s == nil || s.repo == nil || binding == nil || accountID <= 0 {
		return
	}
	binding.mu.Lock()
	defer binding.mu.Unlock()
	binding.accountID = accountID
	binding.accountName = strings.TrimSpace(accountName)
	if !binding.persisted || binding.requestID == "" || binding.apiKeyID <= 0 {
		return
	}
	repo, ok := s.repo.(interface {
		UpdateLogAccount(context.Context, string, int64, int64, string) error
	})
	if !ok {
		return
	}
	if err := repo.UpdateLogAccount(ctx, binding.requestID, binding.apiKeyID, accountID, binding.accountName); err != nil {
		return
	}
}

func (s *ContentModerationService) createContentModerationLog(ctx context.Context, log *ContentModerationLog) error {
	if s == nil || s.repo == nil || log == nil {
		return infraerrors.InternalServer("CONTENT_MODERATION_REPOSITORY_UNAVAILABLE", "风控记录仓储不可用")
	}
	if err := s.repo.CreateLog(ctx, log); err != nil {
		return err
	}
	if log.accountBinding != nil {
		log.accountBinding.mu.Lock()
		log.accountBinding.persisted = true
		binding := log.accountBinding
		accountID := binding.accountID
		accountName := binding.accountName
		binding.mu.Unlock()
		if accountID > 0 {
			s.BindSelectedAccount(ctx, binding, accountID, accountName)
		}
	}
	return nil
}

func (s *ContentModerationService) GetLogRequestBody(ctx context.Context, id int64) (*ContentModerationLogRequestBody, error) {
	if id <= 0 {
		return nil, infraerrors.BadRequest("INVALID_CONTENT_MODERATION_LOG_ID", "风控记录 ID 无效")
	}
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("CONTENT_MODERATION_REPOSITORY_UNAVAILABLE", "风控记录仓储不可用")
	}
	repo, ok := s.repo.(interface {
		GetLogRequestBody(context.Context, int64) (*ContentModerationLogRequestBody, error)
	})
	if !ok {
		return nil, infraerrors.NotFound("CONTENT_MODERATION_REQUEST_BODY_NOT_FOUND", "风控记录请求正文不存在")
	}
	result, err := repo.GetLogRequestBody(ctx, id)
	if err != nil {
		return nil, err
	}
	if result == nil || strings.TrimSpace(result.Content) == "" {
		return nil, infraerrors.NotFound("CONTENT_MODERATION_REQUEST_BODY_NOT_FOUND", "风控记录请求正文不存在")
	}
	if result.ContentType == "" {
		result.ContentType = "application/json;charset=utf-8"
	}
	if result.Size <= 0 {
		result.Size = len([]byte(result.Content))
	}
	if strings.TrimSpace(result.Filename) == "" {
		result.Filename = contentModerationRequestBodyFilename(result.ID, result.RequestID, result.CreatedAt)
	}
	return result, nil
}

func contentModerationRequestBodyFilename(id int64, requestID string, createdAt time.Time) string {
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		requestID = fmt.Sprintf("log-%d", id)
	}
	var b strings.Builder
	for _, r := range requestID {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return fmt.Sprintf("risk-control-%d-%s-%s.json", id, createdAt.UTC().Format("20060102T150405Z"), b.String())
}
