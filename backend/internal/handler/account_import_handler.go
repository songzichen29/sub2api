package handler

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	accountImportTokenHeader = "X-Account-Import-Token"
	accountImportTokenTTL    = 30 * time.Minute
)

type AccountImportHandler struct {
	accountHandler *admin.AccountHandler
	settingService *service.SettingService
	adminService   service.AdminService

	mu     sync.Mutex
	tokens map[string]accountImportTokenRecord
}

type accountImportVerifyRequest struct {
	Password string `json:"password"`
}

type accountImportTokenRecord struct {
	expiresAt    time.Time
	passwordHash string
}

func NewAccountImportHandler(accountHandler *admin.AccountHandler, settingService *service.SettingService, adminService service.AdminService) *AccountImportHandler {
	return &AccountImportHandler{
		accountHandler: accountHandler,
		settingService: settingService,
		adminService:   adminService,
		tokens:         make(map[string]accountImportTokenRecord),
	}
}

func (h *AccountImportHandler) GetStatus(c *gin.Context) {
	cfg, err := h.settingService.GetStandaloneAccountImportConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{
		"enabled":             cfg.Enabled,
		"password_configured": cfg.PasswordConfigured,
	})
}

func (h *AccountImportHandler) Verify(c *gin.Context) {
	cfg, ok := h.requireEnabledConfig(c)
	if !ok {
		return
	}

	var req accountImportVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if !service.CheckStandaloneAccountImportPassword(cfg.PasswordHash, req.Password) {
		response.Unauthorized(c, "Invalid password")
		return
	}

	token, err := generateAccountImportToken()
	if err != nil {
		response.InternalError(c, "Failed to create token")
		return
	}
	expiresAt := time.Now().Add(accountImportTokenTTL)
	h.mu.Lock()
	h.pruneExpiredTokensLocked(time.Now())
	h.tokens[token] = accountImportTokenRecord{
		expiresAt:    expiresAt,
		passwordHash: cfg.PasswordHash,
	}
	h.mu.Unlock()

	response.Success(c, gin.H{
		"token":      token,
		"expires_at": expiresAt.UTC().Format(time.RFC3339),
	})
}

func (h *AccountImportHandler) GetTemplates(c *gin.Context) {
	if !h.requireToken(c) {
		return
	}

	templates, err := h.settingService.GetAccountImportApplyTemplates(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"templates": templates})
}

func (h *AccountImportHandler) GetOptions(c *gin.Context) {
	if !h.requireToken(c) {
		return
	}

	groups, err := h.adminService.GetAllGroups(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	proxies, err := h.adminService.GetAllProxies(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	tags, err := h.adminService.ListAllAccountTags(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if tags == nil {
		tags = []string{}
	}

	outGroups := make([]dto.Group, 0, len(groups))
	for i := range groups {
		if group := dto.GroupFromServiceShallow(&groups[i]); group != nil {
			outGroups = append(outGroups, *group)
		}
	}
	outProxies := make([]dto.Proxy, 0, len(proxies))
	for i := range proxies {
		if proxy := dto.ProxyFromService(&proxies[i]); proxy != nil {
			outProxies = append(outProxies, *proxy)
		}
	}

	response.Success(c, gin.H{
		"groups":  outGroups,
		"proxies": outProxies,
		"tags":    tags,
	})
}

func (h *AccountImportHandler) ImportData(c *gin.Context) {
	if !h.requireToken(c) {
		return
	}

	var req admin.DataImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	result, err := h.accountHandler.ImportDataPayload(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *AccountImportHandler) requireEnabledConfig(c *gin.Context) (*service.StandaloneAccountImportConfig, bool) {
	cfg, err := h.settingService.GetStandaloneAccountImportConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return nil, false
	}
	if cfg == nil || !cfg.Enabled || !cfg.PasswordConfigured {
		response.Forbidden(c, "Standalone account import is not enabled")
		return nil, false
	}
	return cfg, true
}

func (h *AccountImportHandler) requireToken(c *gin.Context) bool {
	cfg, ok := h.requireEnabledConfig(c)
	if !ok {
		return false
	}

	token := strings.TrimSpace(c.GetHeader(accountImportTokenHeader))
	if token == "" {
		response.Unauthorized(c, "Missing account import token")
		return false
	}

	now := time.Now()
	h.mu.Lock()
	defer h.mu.Unlock()

	var matched string
	for storedToken, record := range h.tokens {
		if !record.expiresAt.After(now) {
			delete(h.tokens, storedToken)
			continue
		}
		if !sameSecretString(record.passwordHash, cfg.PasswordHash) {
			delete(h.tokens, storedToken)
			continue
		}
		if subtle.ConstantTimeCompare([]byte(storedToken), []byte(token)) == 1 {
			matched = storedToken
		}
	}
	if matched == "" {
		response.Unauthorized(c, "Invalid or expired account import token")
		return false
	}
	record := h.tokens[matched]
	record.expiresAt = now.Add(accountImportTokenTTL)
	h.tokens[matched] = record
	return true
}

func (h *AccountImportHandler) pruneExpiredTokensLocked(now time.Time) {
	for token, record := range h.tokens {
		if !record.expiresAt.After(now) {
			delete(h.tokens, token)
		}
	}
}

func sameSecretString(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func generateAccountImportToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate account import token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
