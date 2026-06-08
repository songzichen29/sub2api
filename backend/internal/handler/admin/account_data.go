package admin

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"log/slog"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	dataType       = "sub2api-data"
	legacyDataType = "sub2api-bundle"
	dataVersion    = 1
	dataPageCap    = 1000
)

type DataPayload struct {
	Type       string        `json:"type,omitempty"`
	Version    int           `json:"version,omitempty"`
	ExportedAt string        `json:"exported_at"`
	Proxies    []DataProxy   `json:"proxies"`
	Accounts   []DataAccount `json:"accounts"`
}

type DataProxy struct {
	ProxyKey        string `json:"proxy_key"`
	Name            string `json:"name"`
	Protocol        string `json:"protocol"`
	Host            string `json:"host"`
	Port            int    `json:"port"`
	Username        string `json:"username,omitempty"`
	Password        string `json:"password,omitempty"`
	Status          string `json:"status"`
	ExpiresAt       *int64 `json:"expires_at,omitempty"`        // unix 秒，与 DataAccount.ExpiresAt 风格一致
	FallbackMode    string `json:"fallback_mode,omitempty"`     // none/direct/proxy
	BackupProxyName string `json:"backup_proxy_name,omitempty"` // 备用代理 name（跨实例按 name 反查）
	ExpiryWarnDays  int    `json:"expiry_warn_days,omitempty"`
}

// DataAccount 是管理员显式备份导出使用的账号结构，故意不走 dto.Account 的脱敏路径，
// Credentials 原文返回。这是"管理员备份"这一显式行为的一部分；如未来需要导出脱敏版本，
// 应新增独立结构而非修改这里。
type DataAccount struct {
	Name               string         `json:"name"`
	Notes              *string        `json:"notes,omitempty"`
	Platform           string         `json:"platform"`
	Type               string         `json:"type"`
	Credentials        map[string]any `json:"credentials"`
	Extra              map[string]any `json:"extra,omitempty"`
	ProxyKey           *string        `json:"proxy_key,omitempty"`
	Concurrency        int            `json:"concurrency"`
	Priority           int            `json:"priority"`
	RateMultiplier     *float64       `json:"rate_multiplier,omitempty"`
	ExpiresAt          *int64         `json:"expires_at,omitempty"`
	AutoPauseOnExpired *bool          `json:"auto_pause_on_expired,omitempty"`

	// Tags 管理员标签集合（feature 2026-05-04-account-tags 引入），导出时序列化、
	// 导入时按 NormalizeAccountTags 规范化后写入 accounts.tags。omitempty 保证旧版导出
	// 文件不带此字段时解析为 nil，与"未提供"语义一致。
	Tags []string `json:"tags,omitempty"`

	// GroupIDs 账号绑定的分组 ID 列表（feature 2026-05-06-account-import-apply 引入）。
	// 导出时把账号当前绑定的分组 ID 写出，导入时透传给 CreateAccountInput.GroupIDs；
	// omitempty 保证旧版导出文件不带此字段时解析为 nil，等价"未提供"。
	GroupIDs []int64 `json:"group_ids,omitempty"`
}

type DataImportRequest struct {
	Data                 DataPayload `json:"data"`
	SkipDefaultGroupBind *bool       `json:"skip_default_group_bind"`

	// Apply 导入时统一应用到所有 account 的字段集合（feature 2026-05-06-account-import-apply）。
	// nil 或全字段 nil → 行为等价旧版本（不覆盖任何字段，沿用文件原值）。
	// 任一字段非 nil → 该字段值在循环里覆盖每条 account 对应字段；具体语义由
	// applyImportApplyToAccount 实现。
	Apply *DataImportApply `json:"apply,omitempty"`
}

// DataImportApply 是导入时由 admin 在弹窗 UI 里勾选并填值的"应用块"。
//
// 每个字段都是指针/可选，三态语义：
//   - 字段在 JSON 里整体省略（指针 nil）→ "未启用应用"，保留文件原值
//   - 字段为 null / [] / {} / 0 → "启用应用且显式清空 / 清除"
//   - 字段为非空具体值 → "启用应用且覆盖文件原值"
//
// 与 service.BulkUpdateAccountsInput 的 *[]string / *int 语义口径一致；前端通过
// enable checkbox 控制构造 payload，未勾选的字段从 JSON 里整体省略。
type DataImportApply struct {
	Tags         *[]string          `json:"tags,omitempty"`          // nil 不应用；[] 显式清空；非空 全量替换
	GroupIDs     *[]int64           `json:"group_ids,omitempty"`     // nil 不应用；[] 显式不绑任何分组
	ProxyID      *int64             `json:"proxy_id,omitempty"`      // nil 不应用；0 显式清除代理；>0 设代理
	Concurrency  *int               `json:"concurrency,omitempty"`   // nil 不应用
	Priority     *int               `json:"priority,omitempty"`      // nil 不应用
	ModelMapping *map[string]string `json:"model_mapping,omitempty"` // nil 不应用；{} 显式清空白名单/映射
}

type DataImportResult struct {
	ProxyCreated   int               `json:"proxy_created"`
	ProxyReused    int               `json:"proxy_reused"`
	ProxyFailed    int               `json:"proxy_failed"`
	AccountCreated int               `json:"account_created"`
	AccountFailed  int               `json:"account_failed"`
	Errors         []DataImportError `json:"errors,omitempty"`
}

type DataImportError struct {
	Kind     string `json:"kind"`
	Name     string `json:"name,omitempty"`
	ProxyKey string `json:"proxy_key,omitempty"`
	Message  string `json:"message"`
}

func buildProxyKey(protocol, host string, port int, username, password string) string {
	return fmt.Sprintf("%s|%s|%d|%s|%s", strings.TrimSpace(protocol), strings.TrimSpace(host), port, strings.TrimSpace(username), strings.TrimSpace(password))
}

func (h *AccountHandler) ExportData(c *gin.Context) {
	ctx := c.Request.Context()

	selectedIDs, err := parseAccountIDs(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	accounts, err := h.resolveExportAccounts(ctx, selectedIDs, c)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	includeProxies, err := parseIncludeProxies(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	var proxies []service.Proxy
	if includeProxies {
		proxies, err = h.resolveExportProxies(ctx, accounts)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
	} else {
		proxies = []service.Proxy{}
	}

	// 构建 id→name 映射，用于导出备用代理 name
	proxyNameByID := make(map[int64]string, len(proxies))
	for i := range proxies {
		proxyNameByID[proxies[i].ID] = proxies[i].Name
	}

	proxyKeyByID := make(map[int64]string, len(proxies))
	dataProxies := make([]DataProxy, 0, len(proxies))
	for i := range proxies {
		p := proxies[i]
		key := buildProxyKey(p.Protocol, p.Host, p.Port, p.Username, p.Password)
		proxyKeyByID[p.ID] = key

		var expiresAt *int64
		if p.ExpiresAt != nil {
			v := p.ExpiresAt.Unix()
			expiresAt = &v
		}
		var backupProxyName string
		if p.BackupProxyID != nil {
			backupProxyName = proxyNameByID[*p.BackupProxyID]
		}
		dataProxies = append(dataProxies, DataProxy{
			ProxyKey:        key,
			Name:            p.Name,
			Protocol:        p.Protocol,
			Host:            p.Host,
			Port:            p.Port,
			Username:        p.Username,
			Password:        p.Password,
			Status:          p.Status,
			ExpiresAt:       expiresAt,
			FallbackMode:    p.FallbackMode,
			BackupProxyName: backupProxyName,
			ExpiryWarnDays:  p.ExpiryWarnDays,
		})
	}

	dataAccounts := make([]DataAccount, 0, len(accounts))
	for i := range accounts {
		acc := accounts[i]
		var proxyKey *string
		if acc.ProxyID != nil {
			if key, ok := proxyKeyByID[*acc.ProxyID]; ok {
				proxyKey = &key
			}
		}
		var expiresAt *int64
		if acc.ExpiresAt != nil {
			v := acc.ExpiresAt.Unix()
			expiresAt = &v
		}
		dataAccounts = append(dataAccounts, DataAccount{
			Name:               acc.Name,
			Notes:              acc.Notes,
			Platform:           acc.Platform,
			Type:               acc.Type,
			Credentials:        acc.Credentials,
			Extra:              acc.Extra,
			ProxyKey:           proxyKey,
			Concurrency:        acc.Concurrency,
			Priority:           acc.Priority,
			RateMultiplier:     acc.RateMultiplier,
			ExpiresAt:          expiresAt,
			AutoPauseOnExpired: &acc.AutoPauseOnExpired,
			Tags:               acc.Tags,
			GroupIDs:           collectGroupIDs(acc.Groups),
		})
	}

	payload := DataPayload{
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Proxies:    dataProxies,
		Accounts:   dataAccounts,
	}

	response.Success(c, payload)
}

func (h *AccountHandler) ImportData(c *gin.Context) {
	var req DataImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if err := validateDataHeader(req.Data); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	executeAdminIdempotentJSON(c, "admin.accounts.import_data", req, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		return h.importData(ctx, req)
	})
}

func (h *AccountHandler) ImportDataPayload(ctx context.Context, req DataImportRequest) (DataImportResult, error) {
	if err := validateDataHeader(req.Data); err != nil {
		return DataImportResult{}, infraerrors.BadRequest("INVALID_IMPORT_DATA", err.Error())
	}
	return h.importData(ctx, req)
}

func (h *AccountHandler) importData(ctx context.Context, req DataImportRequest) (DataImportResult, error) {
	skipDefaultGroupBind := true
	if req.SkipDefaultGroupBind != nil {
		skipDefaultGroupBind = *req.SkipDefaultGroupBind
	}

	dataPayload := req.Data
	result := DataImportResult{}

	existingProxies, err := h.listAllProxies(ctx)
	if err != nil {
		return result, err
	}

	proxyKeyToID := make(map[string]int64, len(existingProxies))
	// proxyNameToID 用于 backup_proxy_name 反查：DB 已有 + 本批次新建均会写入
	proxyNameToID := make(map[string]int64, len(existingProxies))
	for i := range existingProxies {
		p := existingProxies[i]
		key := buildProxyKey(p.Protocol, p.Host, p.Port, p.Username, p.Password)
		proxyKeyToID[key] = p.ID
		if p.Name != "" {
			proxyNameToID[p.Name] = p.ID
		}
	}

	for i := range dataPayload.Proxies {
		item := dataPayload.Proxies[i]
		key := item.ProxyKey
		if key == "" {
			key = buildProxyKey(item.Protocol, item.Host, item.Port, item.Username, item.Password)
		}
		if err := validateDataProxy(item); err != nil {
			result.ProxyFailed++
			result.Errors = append(result.Errors, DataImportError{
				Kind:     "proxy",
				Name:     item.Name,
				ProxyKey: key,
				Message:  err.Error(),
			})
			continue
		}
		normalizedStatus := normalizeProxyStatus(item.Status)
		if existingID, ok := proxyKeyToID[key]; ok {
			proxyKeyToID[key] = existingID
			result.ProxyReused++
			if normalizedStatus != "" {
				if proxy, getErr := h.adminService.GetProxy(ctx, existingID); getErr == nil && proxy != nil && proxy.Status != normalizedStatus {
					// 同步 status 时传入完整字段，避免零值覆盖已存在代理的有效期/fallback 配置。
					var existingExpiresAt *time.Time
					if item.ExpiresAt != nil {
						t := time.Unix(*item.ExpiresAt, 0).UTC()
						existingExpiresAt = &t
					}
					existingFallbackMode := item.FallbackMode
					if existingFallbackMode == "" {
						existingFallbackMode = service.FallbackModeNone
					}
					var existingBackupProxyID *int64
					if item.BackupProxyName != "" {
						if bid, ok := proxyNameToID[item.BackupProxyName]; ok {
							existingBackupProxyID = &bid
						}
					}
					_, _ = h.adminService.UpdateProxy(ctx, existingID, &service.UpdateProxyInput{
						Status:         normalizedStatus,
						ExpiresAt:      existingExpiresAt,
						FallbackMode:   existingFallbackMode,
						BackupProxyID:  existingBackupProxyID,
						ExpiryWarnDays: item.ExpiryWarnDays,
						Name:           proxy.Name,
						Protocol:       proxy.Protocol,
						Host:           proxy.Host,
						Port:           proxy.Port,
						Username:       proxy.Username,
						Password:       proxy.Password,
					})
				}
			}
			continue
		}

		// 解析 expires_at（unix 秒 → *time.Time）
		var expiresAt *time.Time
		if item.ExpiresAt != nil {
			t := time.Unix(*item.ExpiresAt, 0).UTC()
			expiresAt = &t
		}

		// 解析 backup_proxy_name → backup_proxy_id
		fallbackMode := item.FallbackMode
		var backupProxyID *int64
		if item.BackupProxyName != "" {
			if bid, ok := proxyNameToID[item.BackupProxyName]; ok {
				backupProxyID = &bid
			} else {
				// 查不到备用代理：降级 fallback_mode=none，记录 warning
				fallbackMode = service.FallbackModeNone
				result.Errors = append(result.Errors, DataImportError{
					Kind:     "proxy",
					Name:     item.Name,
					ProxyKey: key,
					Message:  fmt.Sprintf("backup_proxy_name %q not found, fallback_mode downgraded to none", item.BackupProxyName),
				})
			}
		}

		created, createErr := h.adminService.CreateProxy(ctx, &service.CreateProxyInput{
			Name:           defaultProxyName(item.Name),
			Protocol:       item.Protocol,
			Host:           item.Host,
			Port:           item.Port,
			Username:       item.Username,
			Password:       item.Password,
			ExpiresAt:      expiresAt,
			FallbackMode:   fallbackMode,
			BackupProxyID:  backupProxyID,
			ExpiryWarnDays: item.ExpiryWarnDays,
		})
		if createErr != nil {
			result.ProxyFailed++
			result.Errors = append(result.Errors, DataImportError{
				Kind:     "proxy",
				Name:     item.Name,
				ProxyKey: key,
				Message:  createErr.Error(),
			})
			continue
		}
		proxyKeyToID[key] = created.ID
		// 把新建代理的 name 也加入反查表，供后续批内代理引用
		if created.Name != "" {
			proxyNameToID[created.Name] = created.ID
		}
		result.ProxyCreated++

		if normalizedStatus != "" && normalizedStatus != created.Status {
			// 新建后同步 status 时，传入完整字段，避免零值覆盖刚创建的有效期/fallback 配置。
			_, _ = h.adminService.UpdateProxy(ctx, created.ID, &service.UpdateProxyInput{
				Status:         normalizedStatus,
				ExpiresAt:      expiresAt,
				FallbackMode:   fallbackMode,
				BackupProxyID:  backupProxyID,
				ExpiryWarnDays: item.ExpiryWarnDays,
				Name:           created.Name,
				Protocol:       created.Protocol,
				Host:           created.Host,
				Port:           created.Port,
				Username:       created.Username,
				Password:       created.Password,
			})
		}
	}

	// 收集需要异步设置隐私的 Antigravity OAuth 账号
	var privacyAccounts []*service.Account

	for i := range dataPayload.Accounts {
		item := dataPayload.Accounts[i]

		// 应用 Apply 块（feature 2026-05-06-account-import-apply）：把 admin 在导入弹窗
		// 里勾选并填值的字段统一覆盖到本条 account 上；未勾选的字段保留文件原值。
		// resolvedProxyID 用于绕开文件 proxy_key 流程，直接按 UI 选的代理 ID 入库。
		resolvedProxyID := applyImportApplyToAccount(&item, req.Apply)

		if err := validateDataAccount(item); err != nil {
			result.AccountFailed++
			result.Errors = append(result.Errors, DataImportError{
				Kind:    "account",
				Name:    item.Name,
				Message: err.Error(),
			})
			continue
		}

		var proxyID *int64
		if resolvedProxyID != nil {
			// Apply 启用了 ProxyID：0 表示"不绑代理"，>0 表示绑该 ID 代理。
			if *resolvedProxyID > 0 {
				id := *resolvedProxyID
				proxyID = &id
			}
		} else if item.ProxyKey != nil && *item.ProxyKey != "" {
			// 沿用旧逻辑：按文件 proxy_key 反查代理 ID。
			if id, ok := proxyKeyToID[*item.ProxyKey]; ok {
				proxyID = &id
			} else {
				result.AccountFailed++
				result.Errors = append(result.Errors, DataImportError{
					Kind:     "account",
					Name:     item.Name,
					ProxyKey: *item.ProxyKey,
					Message:  "proxy_key not found",
				})
				continue
			}
		}

		enrichCredentialsFromIDToken(&item)

		accountInput := &service.CreateAccountInput{
			Name:                 item.Name,
			Notes:                item.Notes,
			Platform:             item.Platform,
			Type:                 item.Type,
			Credentials:          item.Credentials,
			Extra:                item.Extra,
			ProxyID:              proxyID,
			Concurrency:          item.Concurrency,
			Priority:             item.Priority,
			RateMultiplier:       item.RateMultiplier,
			GroupIDs:             item.GroupIDs,
			Tags:                 item.Tags,
			ExpiresAt:            item.ExpiresAt,
			AutoPauseOnExpired:   item.AutoPauseOnExpired,
			SkipDefaultGroupBind: skipDefaultGroupBind,
		}

		created, err := h.adminService.CreateAccount(ctx, accountInput)
		if err != nil {
			result.AccountFailed++
			result.Errors = append(result.Errors, DataImportError{
				Kind:    "account",
				Name:    item.Name,
				Message: err.Error(),
			})
			continue
		}
		// 收集 Antigravity OAuth 账号，稍后异步设置隐私
		if created.Platform == service.PlatformAntigravity && created.Type == service.AccountTypeOAuth {
			privacyAccounts = append(privacyAccounts, created)
		}
		result.AccountCreated++
	}

	// 异步设置 Antigravity 隐私，避免大量导入时阻塞请求
	if len(privacyAccounts) > 0 {
		adminSvc := h.adminService
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("import_antigravity_privacy_panic", "recover", r)
				}
			}()
			bgCtx := context.Background()
			for _, acc := range privacyAccounts {
				adminSvc.ForceAntigravityPrivacy(bgCtx, acc)
			}
			slog.Info("import_antigravity_privacy_done", "count", len(privacyAccounts))
		}()
	}

	return result, nil
}

func (h *AccountHandler) listAllProxies(ctx context.Context) ([]service.Proxy, error) {
	page := 1
	pageSize := dataPageCap
	var out []service.Proxy
	for {
		items, total, err := h.adminService.ListProxies(ctx, page, pageSize, "", "", "", "created_at", "desc")
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
		if len(out) >= int(total) || len(items) == 0 {
			break
		}
		page++
	}
	return out, nil
}

func (h *AccountHandler) listAccountsFiltered(ctx context.Context, platform, accountType, status, search string, groupID int64, privacyMode, sortBy, sortOrder string, tags []string) ([]service.Account, error) {
	page := 1
	pageSize := dataPageCap
	var out []service.Account
	for {
		items, total, err := h.adminService.ListAccounts(ctx, page, pageSize, platform, accountType, status, search, groupID, privacyMode, sortBy, sortOrder, tags)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
		if len(out) >= int(total) || len(items) == 0 {
			break
		}
		page++
	}
	return out, nil
}

func (h *AccountHandler) resolveExportAccounts(ctx context.Context, ids []int64, c *gin.Context) ([]service.Account, error) {
	if len(ids) > 0 {
		accounts, err := h.adminService.GetAccountsByIDs(ctx, ids)
		if err != nil {
			return nil, err
		}
		out := make([]service.Account, 0, len(accounts))
		for _, acc := range accounts {
			if acc == nil {
				continue
			}
			out = append(out, *acc)
		}
		return out, nil
	}

	platform := c.Query("platform")
	accountType := c.Query("type")
	status := c.Query("status")
	privacyMode := strings.TrimSpace(c.Query("privacy_mode"))
	search := strings.TrimSpace(c.Query("search"))
	sortBy := c.DefaultQuery("sort_by", "last_used_at")
	sortOrder := c.DefaultQuery("sort_order", "desc")
	if len(search) > 100 {
		search = search[:100]
	}

	groupID := int64(0)
	if groupIDStr := c.Query("group"); groupIDStr != "" {
		if groupIDStr == accountListGroupUngroupedQueryValue {
			groupID = service.AccountListGroupUngrouped
		} else {
			parsedGroupID, parseErr := strconv.ParseInt(groupIDStr, 10, 64)
			if parseErr != nil || parsedGroupID <= 0 {
				return nil, infraerrors.BadRequest("INVALID_GROUP_FILTER", "invalid group filter")
			}
			groupID = parsedGroupID
		}
	}

	rawTags := c.QueryArray("tags")
	tagsFilter, tagsErr := normalizeTagsForListFilter(rawTags)
	if tagsErr != nil {
		return nil, tagsErr
	}

	return h.listAccountsFiltered(ctx, platform, accountType, status, search, groupID, privacyMode, sortBy, sortOrder, tagsFilter)
}

func (h *AccountHandler) resolveExportProxies(ctx context.Context, accounts []service.Account) ([]service.Proxy, error) {
	if len(accounts) == 0 {
		return []service.Proxy{}, nil
	}

	seen := make(map[int64]struct{})
	ids := make([]int64, 0)
	for i := range accounts {
		if accounts[i].ProxyID == nil {
			continue
		}
		id := *accounts[i].ProxyID
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return []service.Proxy{}, nil
	}

	return h.adminService.GetProxiesByIDs(ctx, ids)
}

func parseAccountIDs(c *gin.Context) ([]int64, error) {
	values := c.QueryArray("ids")
	if len(values) == 0 {
		raw := strings.TrimSpace(c.Query("ids"))
		if raw != "" {
			values = []string{raw}
		}
	}
	if len(values) == 0 {
		return nil, nil
	}

	ids := make([]int64, 0, len(values))
	for _, item := range values {
		for _, part := range strings.Split(item, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			id, err := strconv.ParseInt(part, 10, 64)
			if err != nil || id <= 0 {
				return nil, fmt.Errorf("invalid account id: %s", part)
			}
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func parseIncludeProxies(c *gin.Context) (bool, error) {
	raw := strings.TrimSpace(strings.ToLower(c.Query("include_proxies")))
	if raw == "" {
		return true, nil
	}
	switch raw {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return true, fmt.Errorf("invalid include_proxies value: %s", raw)
	}
}

// applyImportApplyToAccount 把 Apply 块覆盖到一条 DataAccount 上。
//
// 语义：
//   - apply == nil → 直接返回 nil，item 不变（等价旧版本行为）
//   - apply.Tags / GroupIDs / Concurrency / Priority / ModelMapping 任一非 nil →
//     该字段值覆盖 item 对应字段；为 nil 则保留 item 原值
//   - apply.ProxyID 非 nil → 显式置空 item.ProxyKey（绕开文件 proxy_key 反查），
//     并通过返回值 resolvedProxyID 把 ID 透出给上层 handler。返回值 0 表示"显式
//     不绑代理"，>0 表示"绑指定代理"，nil 表示"按文件 ProxyKey 流程查"
//
// ModelMapping 写到 item.Credentials["model_mapping"] 时保留 credentials 其他键
// （access_token / api_key 等核心字段不能被破坏）；item.Credentials 为 nil 时先
// 初始化为空 map 再赋值，避免下游访问 nil map panic。
func applyImportApplyToAccount(item *DataAccount, apply *DataImportApply) (resolvedProxyID *int64) {
	if apply == nil {
		return nil
	}
	if apply.Tags != nil {
		item.Tags = *apply.Tags
	}
	if apply.GroupIDs != nil {
		item.GroupIDs = *apply.GroupIDs
	}
	if apply.Concurrency != nil {
		item.Concurrency = *apply.Concurrency
	}
	if apply.Priority != nil {
		item.Priority = *apply.Priority
	}
	if apply.ModelMapping != nil {
		if item.Credentials == nil {
			item.Credentials = map[string]any{}
		}
		// 注意：是给 model_mapping 单 key 赋值，不要整体替换 credentials map。
		item.Credentials["model_mapping"] = *apply.ModelMapping
	}
	if apply.ProxyID != nil {
		// 绕开文件 ProxyKey 流程：上层 handler 看到 resolvedProxyID 非 nil 时
		// 不再查 proxyKeyToID，避免文件 proxy_key 与 UI 选择冲突时行为不确定。
		item.ProxyKey = nil
		proxyID := *apply.ProxyID
		return &proxyID
	}
	return nil
}

func validateDataHeader(payload DataPayload) error {
	if payload.Type != "" && payload.Type != dataType && payload.Type != legacyDataType {
		return fmt.Errorf("unsupported data type: %s", payload.Type)
	}
	if payload.Version != 0 && payload.Version != dataVersion {
		return fmt.Errorf("unsupported data version: %d", payload.Version)
	}
	if payload.Proxies == nil {
		return errors.New("proxies is required")
	}
	if payload.Accounts == nil {
		return errors.New("accounts is required")
	}
	return nil
}

func validateDataProxy(item DataProxy) error {
	if strings.TrimSpace(item.Protocol) == "" {
		return errors.New("proxy protocol is required")
	}
	if strings.TrimSpace(item.Host) == "" {
		return errors.New("proxy host is required")
	}
	if item.Port <= 0 || item.Port > 65535 {
		return errors.New("proxy port is invalid")
	}
	switch item.Protocol {
	case "http", "https", "socks5", "socks5h":
	default:
		return fmt.Errorf("proxy protocol is invalid: %s", item.Protocol)
	}
	if item.Status != "" {
		normalizedStatus := normalizeProxyStatus(item.Status)
		if normalizedStatus != service.StatusActive && normalizedStatus != "inactive" {
			return fmt.Errorf("proxy status is invalid: %s", item.Status)
		}
	}
	return nil
}

func validateDataAccount(item DataAccount) error {
	if strings.TrimSpace(item.Name) == "" {
		return errors.New("account name is required")
	}
	if strings.TrimSpace(item.Platform) == "" {
		return errors.New("account platform is required")
	}
	if strings.TrimSpace(item.Type) == "" {
		return errors.New("account type is required")
	}
	if len(item.Credentials) == 0 {
		return errors.New("account credentials is required")
	}
	switch item.Type {
	case service.AccountTypeOAuth, service.AccountTypeSetupToken, service.AccountTypeAPIKey, service.AccountTypeUpstream:
	default:
		return fmt.Errorf("account type is invalid: %s", item.Type)
	}
	if item.RateMultiplier != nil && *item.RateMultiplier < 0 {
		return errors.New("rate_multiplier must be >= 0")
	}
	if item.Concurrency < 0 {
		return errors.New("concurrency must be >= 0")
	}
	if item.Priority < 0 {
		return errors.New("priority must be >= 0")
	}
	return nil
}

func defaultProxyName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "imported-proxy"
	}
	return name
}

// collectGroupIDs 从预加载的 Groups slice 中抽取 ID 列表用于导出。
// nil / 空 slice 都返回 nil（让上层 omitempty 把字段从 JSON 里整体省略）；
// 跳过 nil 元素和无效 ID（<= 0），保持顺序与原 slice 一致。
func collectGroupIDs(groups []*service.Group) []int64 {
	if len(groups) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(groups))
	for _, g := range groups {
		if g == nil || g.ID <= 0 {
			continue
		}
		ids = append(ids, g.ID)
	}
	if len(ids) == 0 {
		return nil
	}
	return ids
}

// enrichCredentialsFromIDToken performs best-effort extraction of user info fields
// (email, plan_type, chatgpt_account_id, etc.) from id_token in credentials.
// Only applies to OpenAI OAuth accounts. Skips expired token errors silently.
// Existing credential values are never overwritten — only missing fields are filled.
func enrichCredentialsFromIDToken(item *DataAccount) {
	if item.Credentials == nil {
		return
	}
	// Only enrich OpenAI OAuth accounts
	platform := strings.ToLower(strings.TrimSpace(item.Platform))
	if platform != service.PlatformOpenAI {
		return
	}
	if strings.ToLower(strings.TrimSpace(item.Type)) != service.AccountTypeOAuth {
		return
	}

	idToken, _ := item.Credentials["id_token"].(string)
	if strings.TrimSpace(idToken) == "" {
		return
	}

	// DecodeIDToken skips expiry validation — safe for imported data
	claims, err := openai.DecodeIDToken(idToken)
	if err != nil {
		slog.Debug("import_enrich_id_token_decode_failed", "account", item.Name, "error", err)
		return
	}

	userInfo := claims.GetUserInfo()
	if userInfo == nil {
		return
	}

	// Fill missing fields only (never overwrite existing values)
	setIfMissing := func(key, value string) {
		if value == "" {
			return
		}
		if existing, _ := item.Credentials[key].(string); existing == "" {
			item.Credentials[key] = value
		}
	}

	setIfMissing("email", userInfo.Email)
	setIfMissing("plan_type", userInfo.PlanType)
	setIfMissing("chatgpt_account_id", userInfo.ChatGPTAccountID)
	setIfMissing("chatgpt_user_id", userInfo.ChatGPTUserID)
	setIfMissing("organization_id", userInfo.OrganizationID)
}

func normalizeProxyStatus(status string) string {
	normalized := strings.TrimSpace(strings.ToLower(status))
	switch normalized {
	case "":
		return ""
	case service.StatusActive:
		return service.StatusActive
	case "inactive", service.StatusDisabled:
		return "inactive"
	case "expired":
		// 导入 expired 代理按 inactive 处理，避免导入即触发到期改投逻辑
		return "inactive"
	default:
		return normalized
	}
}
