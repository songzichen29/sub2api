package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type CodexSessionImportRequest struct {
	Content             string `json:"content" binding:"required"`
	Name                string `json:"name"`
	GroupIDs            []int64 `json:"group_ids" binding:"required"`
	ProxyID             *int64 `json:"proxy_id"`
	Priority            int `json:"priority"`
	ExpiresAt           *int64 `json:"expires_at"`
	AutoPauseOnExpired  *bool `json:"auto_pause_on_expired"`
	UpdateExisting      bool `json:"update_existing"`
	Concurrency         *int `json:"concurrency"`
}

type CodexSessionImportResult struct {
	Total    int                           `json:"total"`
	Created  int                           `json:"created"`
	Updated  int                           `json:"updated"`
	Skipped  int                           `json:"skipped"`
	Failed   int                           `json:"failed"`
	Items    []CodexSessionImportResultItem `json:"items,omitempty"`
	Warnings []DataImportError             `json:"warnings,omitempty"`
	Errors   []DataImportError             `json:"errors,omitempty"`
}

type CodexSessionImportResultItem struct {
	ID      int64  `json:"id,omitempty"`
	Index   int    `json:"index"`
	Name    string `json:"name,omitempty"`
	Action  string `json:"action,omitempty"`
	Message string `json:"message,omitempty"`
}

type codexSessionImportContent struct {
	AccessToken string                   `json:"accessToken"`
	Email       string                   `json:"email"`
	Expires     string                   `json:"expires"`
	User        map[string]any           `json:"user"`
	Raw         map[string]json.RawMessage `json:"-"`
}

// ImportCodexSession handles importing a single ChatGPT/Codex session snapshot as an OpenAI OAuth account.
// POST /api/v1/admin/accounts/import/codex-session
func (h *AccountHandler) ImportCodexSession(c *gin.Context) {
	var req CodexSessionImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if len(req.GroupIDs) == 0 {
		response.BadRequest(c, "group_ids is required")
		return
	}
	if req.Concurrency != nil && *req.Concurrency < 1 {
		response.BadRequest(c, "concurrency must be >= 1")
		return
	}
	if req.ProxyID != nil && *req.ProxyID < 0 {
		response.BadRequest(c, "proxy_id must be >= 0")
		return
	}

	executeAdminIdempotentJSON(c, "admin.accounts.import_codex_session", req, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		return h.importCodexSession(ctx, req)
	})
}

func (h *AccountHandler) importCodexSession(ctx context.Context, req CodexSessionImportRequest) (CodexSessionImportResult, error) {
	result := CodexSessionImportResult{Total: 1}

	session, err := parseCodexSessionImportContent(req.Content)
	if err != nil {
		return result, infraerrors.BadRequest("INVALID_CODEX_SESSION_CONTENT", err.Error())
	}
	accountName := strings.TrimSpace(req.Name)
	if accountName == "" {
		accountName = session.ResolveAccountName()
	}
	if accountName == "" {
		accountName = "codex-session-import"
	}

	credentials, extra := buildCodexSessionAccountPayload(session)
	item := DataAccount{
		Name:               accountName,
		Platform:           service.PlatformOpenAI,
		Type:               service.AccountTypeOAuth,
		Credentials:        credentials,
		Extra:              extra,
		Concurrency:        10,
		Priority:           req.Priority,
		GroupIDs:           normalizePositiveInt64s(req.GroupIDs),
		ExpiresAt:          req.ExpiresAt,
		AutoPauseOnExpired: req.AutoPauseOnExpired,
	}
	if req.Concurrency != nil {
		item.Concurrency = *req.Concurrency
	}
	enrichCredentialsFromIDToken(&item)
	if err := validateDataAccount(item); err != nil {
		result.Failed = 1
		result.Errors = append(result.Errors, DataImportError{
			Kind:    "account",
			Name:    accountName,
			Message: err.Error(),
		})
		result.Items = append(result.Items, CodexSessionImportResultItem{
			Index:   1,
			Name:    accountName,
			Action:  "failed",
			Message: err.Error(),
		})
		return result, nil
	}

	var proxyID *int64
	if req.ProxyID != nil && *req.ProxyID > 0 {
		id := *req.ProxyID
		proxyID = &id
	}
	if req.UpdateExisting {
		if updated, ok, updateErr := h.tryUpdateExistingCodexSessionAccount(ctx, accountName, item, proxyID); updateErr != nil {
			result.Failed = 1
			result.Errors = append(result.Errors, DataImportError{
				Kind:    "account",
				Name:    accountName,
				Message: updateErr.Error(),
			})
			result.Items = append(result.Items, CodexSessionImportResultItem{
				Index:   1,
				Name:    accountName,
				Action:  "failed",
				Message: updateErr.Error(),
			})
			return result, nil
		} else if ok {
			result.Updated = 1
			result.Items = append(result.Items, CodexSessionImportResultItem{
				ID:      updated.ID,
				Index:   1,
				Name:    accountName,
				Action:  "updated",
				Message: fmt.Sprintf("updated account #%d", updated.ID),
			})
			return result, nil
		}
	}

	created, err := h.adminService.CreateAccount(ctx, &service.CreateAccountInput{
		Name:                 item.Name,
		Platform:             item.Platform,
		Type:                 item.Type,
		Credentials:          item.Credentials,
		Extra:                item.Extra,
		ProxyID:              proxyID,
		Concurrency:          item.Concurrency,
		Priority:             item.Priority,
		GroupIDs:             item.GroupIDs,
		ExpiresAt:            item.ExpiresAt,
		AutoPauseOnExpired:   item.AutoPauseOnExpired,
		SkipDefaultGroupBind: true,
	})
	if err != nil {
		result.Failed = 1
		result.Errors = append(result.Errors, DataImportError{
			Kind:    "account",
			Name:    accountName,
			Message: err.Error(),
		})
		result.Items = append(result.Items, CodexSessionImportResultItem{
			Index:   1,
			Name:    accountName,
			Action:  "failed",
			Message: err.Error(),
		})
		return result, nil
	}

	h.adminService.ForceOpenAIPrivacy(ctx, created)
	result.Created = 1
	result.Items = append(result.Items, CodexSessionImportResultItem{
		ID:      created.ID,
		Index:   1,
		Name:    accountName,
		Action:  "created",
		Message: fmt.Sprintf("created account #%d", created.ID),
	})
	return result, nil
}

func (h *AccountHandler) tryUpdateExistingCodexSessionAccount(ctx context.Context, accountName string, item DataAccount, proxyID *int64) (*service.Account, bool, error) {
	matches, total, err := h.adminService.ListAccounts(ctx, 1, dataPageCap, service.PlatformOpenAI, service.AccountTypeOAuth, "", accountName, 0, "", "id", "desc", nil)
	if err != nil {
		return nil, false, err
	}
	if total == 0 || len(matches) == 0 {
		return nil, false, nil
	}

	var candidate *service.Account
	for i := range matches {
		acc := matches[i]
		if strings.EqualFold(strings.TrimSpace(acc.Name), strings.TrimSpace(accountName)) {
			candidate = &acc
			break
		}
	}
	if candidate == nil {
		return nil, false, nil
	}

	groupIDs := append([]int64(nil), item.GroupIDs...)
	concurrency := item.Concurrency
	priority := item.Priority
	updateInput := &service.UpdateAccountInput{
		Name:               item.Name,
		Type:               item.Type,
		Credentials:        item.Credentials,
		Extra:              item.Extra,
		ProxyID:            proxyIDOrZero(proxyID),
		Concurrency:        &concurrency,
		Priority:           &priority,
		GroupIDs:           &groupIDs,
		ExpiresAt:          item.ExpiresAt,
		AutoPauseOnExpired: item.AutoPauseOnExpired,
		SkipMixedChannelCheck: true,
	}
	updated, err := h.adminService.UpdateAccount(ctx, candidate.ID, updateInput)
	if err != nil {
		return nil, false, err
	}
	h.adminService.ForceOpenAIPrivacy(ctx, updated)
	return updated, true, nil
}

func parseCodexSessionImportContent(content string) (*codexSessionImportContent, error) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil, fmt.Errorf("content is required")
	}
	if strings.HasPrefix(trimmed, "{") {
		raw := make(map[string]json.RawMessage)
		if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
			return nil, fmt.Errorf("parse session json: %w", err)
		}
		session := &codexSessionImportContent{Raw: raw}
		_ = json.Unmarshal(raw["accessToken"], &session.AccessToken)
		_ = json.Unmarshal(raw["email"], &session.Email)
		_ = json.Unmarshal(raw["expires"], &session.Expires)
		_ = json.Unmarshal(raw["user"], &session.User)
		session.AccessToken = strings.TrimSpace(session.AccessToken)
		session.Email = strings.TrimSpace(session.Email)
		session.Expires = strings.TrimSpace(session.Expires)
		if session.AccessToken == "" {
			return nil, fmt.Errorf("session content missing accessToken")
		}
		return session, nil
	}

	claims, err := openai.DecodeIDToken(trimmed)
	if err != nil {
		return nil, fmt.Errorf("content is neither valid session json nor jwt: %w", err)
	}
	email := strings.TrimSpace(claims.Email)
	if info := claims.GetUserInfo(); info != nil && email == "" {
		email = strings.TrimSpace(info.Email)
	}
	return &codexSessionImportContent{
		AccessToken: strings.TrimSpace(trimmed),
		Email:       email,
	}, nil
}

func (s *codexSessionImportContent) ResolveAccountName() string {
	if s == nil {
		return ""
	}
	if email := strings.TrimSpace(s.Email); email != "" {
		return email
	}
	if s.User != nil {
		if email, _ := s.User["email"].(string); strings.TrimSpace(email) != "" {
			return strings.TrimSpace(email)
		}
	}
	if claims, err := openai.DecodeIDToken(s.AccessToken); err == nil {
		if info := claims.GetUserInfo(); info != nil && strings.TrimSpace(info.Email) != "" {
			return strings.TrimSpace(info.Email)
		}
		if strings.TrimSpace(claims.Email) != "" {
			return strings.TrimSpace(claims.Email)
		}
	}
	return ""
}

func buildCodexSessionAccountPayload(session *codexSessionImportContent) (map[string]any, map[string]any) {
	credentials := map[string]any{
		"access_token": session.AccessToken,
		"client_id": openai.ClientID,
		"user_agent": service.DefaultOpenAICodexUserAgent,
	}
	extra := map[string]any{}

	if strings.TrimSpace(session.Expires) != "" {
		credentials["expires_at"] = strings.TrimSpace(session.Expires)
	}
	if claims, err := openai.DecodeIDToken(session.AccessToken); err == nil {
		if info := claims.GetUserInfo(); info != nil {
			if strings.TrimSpace(info.Email) != "" {
				credentials["email"] = strings.TrimSpace(info.Email)
			}
			if strings.TrimSpace(info.ChatGPTAccountID) != "" {
				credentials["chatgpt_account_id"] = strings.TrimSpace(info.ChatGPTAccountID)
			}
			if strings.TrimSpace(info.ChatGPTUserID) != "" {
				credentials["chatgpt_user_id"] = strings.TrimSpace(info.ChatGPTUserID)
			}
			if strings.TrimSpace(info.PlanType) != "" {
				credentials["plan_type"] = strings.TrimSpace(info.PlanType)
			}
			if strings.TrimSpace(info.OrganizationID) != "" {
				credentials["organization_id"] = strings.TrimSpace(info.OrganizationID)
			}
		}
	}
	if email := session.ResolveAccountName(); email != "" {
		if _, ok := credentials["email"]; !ok {
			credentials["email"] = email
		}
	}
	if session.User != nil {
		if sessionID, ok := session.User["session_id"].(string); ok && strings.TrimSpace(sessionID) != "" {
			extra["openai_session_id"] = strings.TrimSpace(sessionID)
		}
		if deviceID, ok := session.User["device_id"].(string); ok && strings.TrimSpace(deviceID) != "" {
			extra["openai_device_id"] = strings.TrimSpace(deviceID)
		}
	}
	if len(extra) == 0 {
		extra = nil
	}
	return credentials, extra
}

func normalizePositiveInt64s(input []int64) []int64 {
	if len(input) == 0 {
		return nil
	}
	out := make([]int64, 0, len(input))
	seen := make(map[int64]struct{}, len(input))
	for _, id := range input {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func proxyIDOrZero(proxyID *int64) *int64 {
	if proxyID != nil {
		return proxyID
	}
	v := int64(0)
	return &v
}
