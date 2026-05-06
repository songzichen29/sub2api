package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type dataResponse struct {
	Code int         `json:"code"`
	Data dataPayload `json:"data"`
}

type dataPayload struct {
	Type     string        `json:"type"`
	Version  int           `json:"version"`
	Proxies  []dataProxy   `json:"proxies"`
	Accounts []dataAccount `json:"accounts"`
}

type dataProxy struct {
	ProxyKey string `json:"proxy_key"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Status   string `json:"status"`
}

type dataAccount struct {
	Name        string         `json:"name"`
	Platform    string         `json:"platform"`
	Type        string         `json:"type"`
	Credentials map[string]any `json:"credentials"`
	Extra       map[string]any `json:"extra"`
	ProxyKey    *string        `json:"proxy_key"`
	Concurrency int            `json:"concurrency"`
	Priority    int            `json:"priority"`
	Tags        []string       `json:"tags"`
	GroupIDs    []int64        `json:"group_ids"`
}

func setupAccountDataRouter() (*gin.Engine, *stubAdminService) {
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

	router.GET("/api/v1/admin/accounts/data", h.ExportData)
	router.POST("/api/v1/admin/accounts/data", h.ImportData)
	return router, adminSvc
}

func TestExportDataIncludesSecrets(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()

	proxyID := int64(11)
	adminSvc.proxies = []service.Proxy{
		{
			ID:       proxyID,
			Name:     "proxy",
			Protocol: "http",
			Host:     "127.0.0.1",
			Port:     8080,
			Username: "user",
			Password: "pass",
			Status:   service.StatusActive,
		},
		{
			ID:       12,
			Name:     "orphan",
			Protocol: "https",
			Host:     "10.0.0.1",
			Port:     443,
			Username: "o",
			Password: "p",
			Status:   service.StatusActive,
		},
	}
	adminSvc.accounts = []service.Account{
		{
			ID:          21,
			Name:        "account",
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeOAuth,
			Credentials: map[string]any{"token": "secret"},
			Extra:       map[string]any{"note": "x"},
			ProxyID:     &proxyID,
			Concurrency: 3,
			Priority:    50,
			Status:      service.StatusDisabled,
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/data", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp dataResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Empty(t, resp.Data.Type)
	require.Equal(t, 0, resp.Data.Version)
	require.Len(t, resp.Data.Proxies, 1)
	require.Equal(t, "pass", resp.Data.Proxies[0].Password)
	require.Len(t, resp.Data.Accounts, 1)
	require.Equal(t, "secret", resp.Data.Accounts[0].Credentials["token"])
}

func TestExportDataWithoutProxies(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()

	proxyID := int64(11)
	adminSvc.proxies = []service.Proxy{
		{
			ID:       proxyID,
			Name:     "proxy",
			Protocol: "http",
			Host:     "127.0.0.1",
			Port:     8080,
			Username: "user",
			Password: "pass",
			Status:   service.StatusActive,
		},
	}
	adminSvc.accounts = []service.Account{
		{
			ID:          21,
			Name:        "account",
			Platform:    service.PlatformOpenAI,
			Type:        service.AccountTypeOAuth,
			Credentials: map[string]any{"token": "secret"},
			ProxyID:     &proxyID,
			Concurrency: 3,
			Priority:    50,
			Status:      service.StatusDisabled,
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/data?include_proxies=false", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp dataResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Len(t, resp.Data.Proxies, 0)
	require.Len(t, resp.Data.Accounts, 1)
	require.Nil(t, resp.Data.Accounts[0].ProxyKey)
}

func TestExportDataPassesAccountFiltersAndSort(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()
	adminSvc.accounts = []service.Account{
		{ID: 1, Name: "acc-1", Status: service.StatusActive},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/admin/accounts/data?platform=openai&type=oauth&status=active&group=12&privacy_mode=blocked&search=keyword&sort_by=priority&sort_order=desc",
		nil,
	)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Equal(t, 1, adminSvc.lastListAccounts.calls)
	require.Equal(t, "openai", adminSvc.lastListAccounts.platform)
	require.Equal(t, "oauth", adminSvc.lastListAccounts.accountType)
	require.Equal(t, "active", adminSvc.lastListAccounts.status)
	require.Equal(t, int64(12), adminSvc.lastListAccounts.groupID)
	require.Equal(t, "blocked", adminSvc.lastListAccounts.privacyMode)
	require.Equal(t, "keyword", adminSvc.lastListAccounts.search)
	require.Equal(t, "priority", adminSvc.lastListAccounts.sortBy)
	require.Equal(t, "desc", adminSvc.lastListAccounts.sortOrder)
}

func TestExportDataSelectedIDsOverrideFilters(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/admin/accounts/data?ids=1,2&platform=openai&search=keyword&sort_by=priority&sort_order=desc",
		nil,
	)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp dataResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Len(t, resp.Data.Accounts, 2)
	require.Equal(t, 0, adminSvc.lastListAccounts.calls)
}

func TestImportDataReusesProxyAndSkipsDefaultGroup(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()

	adminSvc.proxies = []service.Proxy{
		{
			ID:       1,
			Name:     "proxy",
			Protocol: "socks5",
			Host:     "1.2.3.4",
			Port:     1080,
			Username: "u",
			Password: "p",
			Status:   service.StatusActive,
		},
	}

	dataPayload := map[string]any{
		"data": map[string]any{
			"type":    dataType,
			"version": dataVersion,
			"proxies": []map[string]any{
				{
					"proxy_key": "socks5|1.2.3.4|1080|u|p",
					"name":      "proxy",
					"protocol":  "socks5",
					"host":      "1.2.3.4",
					"port":      1080,
					"username":  "u",
					"password":  "p",
					"status":    "active",
				},
			},
			"accounts": []map[string]any{
				{
					"name":        "acc",
					"platform":    service.PlatformOpenAI,
					"type":        service.AccountTypeOAuth,
					"credentials": map[string]any{"token": "x"},
					"proxy_key":   "socks5|1.2.3.4|1080|u|p",
					"concurrency": 3,
					"priority":    50,
				},
			},
		},
		"skip_default_group_bind": true,
	}

	body, _ := json.Marshal(dataPayload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Len(t, adminSvc.createdProxies, 0)
	require.Len(t, adminSvc.createdAccounts, 1)
	require.True(t, adminSvc.createdAccounts[0].SkipDefaultGroupBind)
}

// ===== feature 2026-05-06-account-import-apply 整链路测试 =====

// 工具：构造一份只含 1 条账号的最简 import payload，apply 由调用方注入。
func buildImportPayloadWithApply(t *testing.T, apply map[string]any) []byte {
	t.Helper()
	payload := map[string]any{
		"data": map[string]any{
			"type":    dataType,
			"version": dataVersion,
			"proxies": []map[string]any{},
			"accounts": []map[string]any{
				{
					"name":        "acc-1",
					"platform":    service.PlatformAnthropic,
					"type":        service.AccountTypeOAuth,
					"credentials": map[string]any{"access_token": "tok"},
					"concurrency": 3,
					"priority":    50,
					"tags":        []string{"legacy"},
				},
			},
		},
		"skip_default_group_bind": true,
	}
	if apply != nil {
		payload["apply"] = apply
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	return body
}

func TestImportData_ApplyTags_AllAccountsTagged(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()

	body := buildImportPayloadWithApply(t, map[string]any{
		"tags": []string{"vip", "prod"},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Len(t, adminSvc.createdAccounts, 1)
	// Apply.Tags 必须覆盖文件原 ["legacy"]
	require.Equal(t, []string{"vip", "prod"}, adminSvc.createdAccounts[0].Tags)
}

func TestImportData_ApplyProxyID_OverridesFileProxyKey(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()

	// 文件里指定 proxy_key 指向某个不存在的代理（按现状会报 proxy_key not found）；
	// Apply.ProxyID=42 应该绕开这条路径，直接使用 ID 42。
	payload := map[string]any{
		"data": map[string]any{
			"type":    dataType,
			"version": dataVersion,
			"proxies": []map[string]any{},
			"accounts": []map[string]any{
				{
					"name":        "acc-1",
					"platform":    service.PlatformAnthropic,
					"type":        service.AccountTypeOAuth,
					"credentials": map[string]any{"access_token": "tok"},
					"proxy_key":   "http|missing|9999|x|y",
					"concurrency": 3,
					"priority":    50,
				},
			},
		},
		"skip_default_group_bind": true,
		"apply": map[string]any{
			"proxy_id": 42,
		},
	}
	body, _ := json.Marshal(payload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Len(t, adminSvc.createdAccounts, 1)
	require.NotNil(t, adminSvc.createdAccounts[0].ProxyID)
	require.Equal(t, int64(42), *adminSvc.createdAccounts[0].ProxyID)
}

func TestImportData_ApplyGroupIDs_BindsToGroups(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()

	body := buildImportPayloadWithApply(t, map[string]any{
		"group_ids": []int{5, 7},
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Len(t, adminSvc.createdAccounts, 1)
	require.Equal(t, []int64{5, 7}, adminSvc.createdAccounts[0].GroupIDs)
	// SkipDefaultGroupBind 仍然 true（由前端硬编码传入，与本 feature 行为正交）
	require.True(t, adminSvc.createdAccounts[0].SkipDefaultGroupBind)
}

func TestImportData_LegacyFileWithoutTags_ApplyTagsWorks(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()

	// 旧版导出文件不带 tags / group_ids 字段，但 Apply.Tags 启用时仍要生效
	payload := map[string]any{
		"data": map[string]any{
			"type":    dataType,
			"version": dataVersion,
			"proxies": []map[string]any{},
			"accounts": []map[string]any{
				{
					"name":        "acc-legacy",
					"platform":    service.PlatformAnthropic,
					"type":        service.AccountTypeOAuth,
					"credentials": map[string]any{"access_token": "tok"},
					"concurrency": 3,
					"priority":    50,
				},
			},
		},
		"skip_default_group_bind": true,
		"apply": map[string]any{
			"tags": []string{"vip"},
		},
	}
	body, _ := json.Marshal(payload)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Len(t, adminSvc.createdAccounts, 1)
	require.Equal(t, []string{"vip"}, adminSvc.createdAccounts[0].Tags)
}

func TestImportData_NilApply_BehavesLikeBefore(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()

	// 完全不传 apply 字段——必须等价于旧版本行为：所有字段沿用文件值
	body := buildImportPayloadWithApply(t, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Len(t, adminSvc.createdAccounts, 1)
	// 文件里 tags=["legacy"]，未启用 Apply 应保留原值
	require.Equal(t, []string{"legacy"}, adminSvc.createdAccounts[0].Tags)
	// 未启用 Apply.GroupIDs 时 input.GroupIDs 沿用文件原值（这里文件没传 group_ids，
	// JSON 解码出来是 nil）
	require.Nil(t, adminSvc.createdAccounts[0].GroupIDs)
	require.Equal(t, 3, adminSvc.createdAccounts[0].Concurrency)
	require.Equal(t, 50, adminSvc.createdAccounts[0].Priority)
}

func TestExportData_IncludesTagsAndGroupIDs(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()

	adminSvc.accounts = []service.Account{
		{
			ID:          21,
			Name:        "acc-1",
			Platform:    service.PlatformAnthropic,
			Type:        service.AccountTypeOAuth,
			Credentials: map[string]any{"access_token": "tok"},
			Concurrency: 3,
			Priority:    50,
			Status:      service.StatusActive,
			Tags:        []string{"vip", "prod"},
			Groups: []*service.Group{
				{ID: 5, Name: "g-5"},
				{ID: 7, Name: "g-7"},
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/data", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp dataResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Data.Accounts, 1)
	require.Equal(t, []string{"vip", "prod"}, resp.Data.Accounts[0].Tags)
	require.Equal(t, []int64{5, 7}, resp.Data.Accounts[0].GroupIDs)
}

func TestExportData_EmptyTagsAndGroups_OmittedFromJSON(t *testing.T) {
	router, adminSvc := setupAccountDataRouter()

	// 账号 tags / Groups 都为空（旧账号或未设置标签/分组）
	adminSvc.accounts = []service.Account{
		{
			ID:          22,
			Name:        "acc-empty",
			Platform:    service.PlatformAnthropic,
			Type:        service.AccountTypeOAuth,
			Credentials: map[string]any{"access_token": "tok"},
			Concurrency: 3,
			Priority:    50,
			Status:      service.StatusActive,
			Tags:        nil,
			Groups:      nil,
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/data", nil)
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	dataField := raw["data"].(map[string]any)
	accounts := dataField["accounts"].([]any)
	require.Len(t, accounts, 1)
	first := accounts[0].(map[string]any)
	// omitempty 应该让空 tags / group_ids 完全不出现在 JSON 里，
	// 而不是序列化成 null（避免下游客户端 type 不一致）
	_, hasTags := first["tags"]
	_, hasGroupIDs := first["group_ids"]
	require.False(t, hasTags, "空 tags 应被 omitempty 省略，不出现在 JSON 里")
	require.False(t, hasGroupIDs, "空 group_ids 应被 omitempty 省略，不出现在 JSON 里")
}
