package admin

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type codexSessionImportResponse struct {
	Code int                      `json:"code"`
	Data CodexSessionImportResult `json:"data"`
}

func setupCodexSessionImportRouter() (*gin.Engine, *stubAdminService) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	adminSvc := newStubAdminService()

	h := NewAccountHandler(
		adminSvc,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	router.POST("/api/v1/admin/accounts/import/codex-session", h.ImportCodexSession)
	return router, adminSvc
}

func TestImportCodexSession_Create(t *testing.T) {
	router, adminSvc := setupCodexSessionImportRouter()

	body, _ := json.Marshal(map[string]any{
		"content":            buildJWTWithPayload(t, map[string]any{"email": "codex@example.com", "https://api.openai.com/auth": map[string]any{"chatgpt_account_id": "acct-1", "chatgpt_user_id": "user-1", "chatgpt_plan_type": "plus", "organizations": []map[string]any{{"id": "org-1", "is_default": true}}}}),
		"name":               "codex@example.com",
		"group_ids":          []int64{9, 7, 9},
		"priority":           1,
		"auto_pause_on_expired": true,
		"update_existing":    true,
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/import/codex-session", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, adminSvc.createdAccounts, 1)
	created := adminSvc.createdAccounts[0]
	require.Equal(t, "codex@example.com", created.Name)
	require.Equal(t, "openai", created.Platform)
	require.Equal(t, "oauth", created.Type)
	require.Equal(t, 10, created.Concurrency)
	require.Equal(t, 1, created.Priority)
	require.Equal(t, []int64{7, 9}, created.GroupIDs)
	require.True(t, created.SkipDefaultGroupBind)
	require.Equal(t, openai.ClientID, created.Credentials["client_id"])
	require.Equal(t, "codex@example.com", created.Credentials["email"])
	require.Equal(t, "acct-1", created.Credentials["chatgpt_account_id"])
	require.Equal(t, "user-1", created.Credentials["chatgpt_user_id"])
	require.Equal(t, "plus", created.Credentials["plan_type"])
	require.Equal(t, "org-1", created.Credentials["organization_id"])
	require.Equal(t, "codex-tui/0.125.0 (Ubuntu 22.4.0; x86_64) xterm-256color (codex-tui; 0.125.0)", created.Credentials["user_agent"])

	var resp codexSessionImportResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 1, resp.Data.Total)
	require.Equal(t, 1, resp.Data.Created)
	require.Equal(t, 0, resp.Data.Updated)
	require.Equal(t, 0, resp.Data.Failed)
	require.Len(t, resp.Data.Items, 1)
	require.Equal(t, "created", resp.Data.Items[0].Action)
}

func TestImportCodexSession_UpdateExistingByName(t *testing.T) {
	router, adminSvc := setupCodexSessionImportRouter()
	adminSvc.accounts = []service.Account{
		{ID: 88, Name: "existing@example.com", Platform: "openai", Type: "oauth"},
	}

	content, _ := json.Marshal(map[string]any{
		"accessToken": buildJWTWithPayload(t, map[string]any{"email": "existing@example.com"}),
		"email":       "existing@example.com",
		"expires":     "2026-05-22T08:00:00Z",
		"user": map[string]any{
			"session_id": "sess-1",
			"device_id":  "dev-1",
		},
	})

	body, _ := json.Marshal(map[string]any{
		"content":         string(content),
		"name":            "existing@example.com",
		"group_ids":       []int64{12},
		"proxy_id":        4,
		"priority":        3,
		"concurrency":     6,
		"update_existing": true,
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/import/codex-session", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Empty(t, adminSvc.createdAccounts)
	require.Len(t, adminSvc.updatedAccountIDs, 1)
	require.Equal(t, int64(88), adminSvc.updatedAccountIDs[0])
	require.Len(t, adminSvc.updatedAccounts, 1)
	update := adminSvc.updatedAccounts[0]
	require.NotNil(t, update.Concurrency)
	require.Equal(t, 6, *update.Concurrency)
	require.NotNil(t, update.Priority)
	require.Equal(t, 3, *update.Priority)
	require.NotNil(t, update.GroupIDs)
	require.Equal(t, []int64{12}, *update.GroupIDs)
	require.NotNil(t, update.ProxyID)
	require.Equal(t, int64(4), *update.ProxyID)
	require.Equal(t, "existing@example.com", update.Name)
	require.Equal(t, "oauth", update.Type)
	require.Equal(t, "sess-1", update.Extra["openai_session_id"])
	require.Equal(t, "dev-1", update.Extra["openai_device_id"])

	var resp codexSessionImportResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 1, resp.Data.Updated)
	require.Equal(t, 0, resp.Data.Created)
	require.Equal(t, "updated", resp.Data.Items[0].Action)
}

func TestImportCodexSession_InvalidContent(t *testing.T) {
	router, _ := setupCodexSessionImportRouter()

	body := []byte(`{"content":"not-a-jwt","group_ids":[1]}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/import/codex-session", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func buildJWTWithPayload(t *testing.T, payload map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	enc := base64.RawURLEncoding.EncodeToString(raw)
	return "x." + enc + ".y"
}
