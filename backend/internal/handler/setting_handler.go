package handler

import (
	"context"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

const customPageProbeTimeout = 4 * time.Second

// SettingHandler 公开设置处理器（无需认证）
type SettingHandler struct {
	settingService           *service.SettingService
	notificationEmailService *service.NotificationEmailService
	version                  string
}

type customPageStatusResponse struct {
	Available  bool   `json:"available"`
	Reason     string `json:"reason,omitempty"`
	StatusCode int    `json:"status_code,omitempty"`
}

// NewSettingHandler 创建公开设置处理器
func NewSettingHandler(settingService *service.SettingService, version string) *SettingHandler {
	return &SettingHandler{
		settingService: settingService,
		version:        version,
	}
}

// SetNotificationEmailService attaches the public notification email service without
// changing the constructor signature used by existing tests.
func (h *SettingHandler) SetNotificationEmailService(notificationEmailService *service.NotificationEmailService) {
	h.notificationEmailService = notificationEmailService
}

// GetPublicSettings 获取公开设置
// GET /api/v1/settings/public
func (h *SettingHandler) GetPublicSettings(c *gin.Context) {
	settings, err := h.settingService.GetPublicSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.PublicSettings{
		RegistrationEnabled:              settings.RegistrationEnabled,
		EmailVerifyEnabled:               settings.EmailVerifyEnabled,
		ForceEmailOnThirdPartySignup:     settings.ForceEmailOnThirdPartySignup,
		RegistrationEmailSuffixWhitelist: settings.RegistrationEmailSuffixWhitelist,
		PromoCodeEnabled:                 settings.PromoCodeEnabled,
		PasswordResetEnabled:             settings.PasswordResetEnabled,
		InvitationCodeEnabled:            settings.InvitationCodeEnabled,
		TotpEnabled:                      settings.TotpEnabled,
		TurnstileEnabled:                 settings.TurnstileEnabled,
		TurnstileSiteKey:                 settings.TurnstileSiteKey,
		SiteName:                         settings.SiteName,
		SiteLogo:                         settings.SiteLogo,
		SiteSubtitle:                     settings.SiteSubtitle,
		APIBaseURL:                       settings.APIBaseURL,
		ContactInfo:                      settings.ContactInfo,
		DocURL:                           settings.DocURL,
		HomeContent:                      settings.HomeContent,
		HideCcsImportButton:              settings.HideCcsImportButton,
		PurchaseSubscriptionEnabled:      settings.PurchaseSubscriptionEnabled,
		PurchaseSubscriptionURL:          settings.PurchaseSubscriptionURL,
		TableDefaultPageSize:             settings.TableDefaultPageSize,
		TablePageSizeOptions:             settings.TablePageSizeOptions,
		CustomMenuItems:                  dto.ParseUserVisibleMenuItems(settings.CustomMenuItems),
		CustomEndpoints:                  dto.ParseCustomEndpoints(settings.CustomEndpoints),
		LinuxDoOAuthEnabled:              settings.LinuxDoOAuthEnabled,
		WeChatOAuthEnabled:               settings.WeChatOAuthEnabled,
		WeChatOAuthOpenEnabled:           settings.WeChatOAuthOpenEnabled,
		WeChatOAuthMPEnabled:             settings.WeChatOAuthMPEnabled,
		WeChatOAuthMobileEnabled:         settings.WeChatOAuthMobileEnabled,
		OIDCOAuthEnabled:                 settings.OIDCOAuthEnabled,
		OIDCOAuthProviderName:            settings.OIDCOAuthProviderName,
		BackendModeEnabled:               settings.BackendModeEnabled,
		PaymentEnabled:                   settings.PaymentEnabled,
		Version:                          h.version,
		BalanceLowNotifyEnabled:          settings.BalanceLowNotifyEnabled,
		AccountQuotaNotifyEnabled:        settings.AccountQuotaNotifyEnabled,
		BalanceLowNotifyThreshold:        settings.BalanceLowNotifyThreshold,
		BalanceLowNotifyRechargeURL:      settings.BalanceLowNotifyRechargeURL,

		ChannelMonitorEnabled:                settings.ChannelMonitorEnabled,
		ChannelMonitorDefaultIntervalSeconds: settings.ChannelMonitorDefaultIntervalSeconds,

		AvailableChannelsEnabled: settings.AvailableChannelsEnabled,

		AffiliateEnabled: settings.AffiliateEnabled,

		RiskControlEnabled: settings.RiskControlEnabled,
	})
}

// UnsubscribeNotificationEmail handles optional notification email opt-outs.
// GET /api/v1/settings/email-unsubscribe?token=...
func (h *SettingHandler) UnsubscribeNotificationEmail(c *gin.Context) {
	if h.notificationEmailService == nil {
		response.InternalError(c, "notification email service is not configured")
		return
	}
	token := strings.TrimSpace(c.Query("token"))
	if token == "" {
		response.BadRequest(c, "token is required")
		return
	}
	result, err := h.notificationEmailService.Unsubscribe(c.Request.Context(), token)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	body := `<!doctype html><html><head><meta charset="utf-8"><title>Unsubscribed</title></head><body style="font-family:-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif;padding:32px;"><h1>Unsubscribed</h1><p>You have unsubscribed <strong>` + html.EscapeString(result.Email) + `</strong> from <strong>` + html.EscapeString(result.Event) + `</strong> emails.</p></body></html>`
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(body))
}

// GetCustomPageStatus checks whether an authenticated user's configured custom page target is reachable.
// GET /api/v1/settings/custom-pages/:id/status
func (h *SettingHandler) GetCustomPageStatus(c *gin.Context) {
	pageID := strings.TrimSpace(c.Param("id"))
	if pageID == "" {
		response.BadRequest(c, "custom page id is required")
		return
	}
	settings, err := h.settingService.GetAllSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	role, _ := middleware2.GetUserRoleFromContext(c)
	item, ok := findVisibleCustomMenuItem(dto.ParseCustomMenuItems(settings.CustomMenuItems), pageID, role == service.RoleAdmin)
	if !ok {
		response.NotFound(c, "Custom page not found")
		return
	}

	rawURL := strings.TrimSpace(item.URL)
	if !isHTTPURL(rawURL) {
		response.Success(c, customPageStatusResponse{
			Available: false,
			Reason:    "invalid_url",
		})
		return
	}

	available, statusCode, reason := probeCustomPageURL(c.Request.Context(), rawURL)
	response.Success(c, customPageStatusResponse{
		Available:  available,
		Reason:     reason,
		StatusCode: statusCode,
	})
}

func findVisibleCustomMenuItem(items []dto.CustomMenuItem, id string, isAdmin bool) (dto.CustomMenuItem, bool) {
	for _, item := range items {
		if item.ID != id {
			continue
		}
		if item.Visibility == "admin" && !isAdmin {
			return dto.CustomMenuItem{}, false
		}
		return item, true
	}
	return dto.CustomMenuItem{}, false
}

func isHTTPURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

func probeCustomPageURL(ctx context.Context, rawURL string) (available bool, statusCode int, reason string) {
	ctx, cancel := context.WithTimeout(ctx, customPageProbeTimeout)
	defer cancel()

	client := &http.Client{
		Timeout: customPageProbeTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := doCustomPageProbe(ctx, client, http.MethodHead, rawURL)
	if err == nil && resp != nil {
		statusCode := resp.StatusCode
		_ = resp.Body.Close()
		if statusCode != http.StatusMethodNotAllowed && statusCode != http.StatusNotImplemented {
			return customPageStatusFromHTTPStatus(statusCode)
		}
	}

	resp, err = doCustomPageProbe(ctx, client, http.MethodGet, rawURL)
	if err != nil {
		return false, 0, "network_error"
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	return customPageStatusFromHTTPStatus(resp.StatusCode)
}

func doCustomPageProbe(ctx context.Context, client *http.Client, method, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Sub2API custom-page-health-check")
	if method == http.MethodGet {
		req.Header.Set("Range", "bytes=0-0")
	}
	return client.Do(req)
}

func customPageStatusFromHTTPStatus(statusCode int) (bool, int, string) {
	if statusCode >= http.StatusInternalServerError {
		return false, statusCode, "http_status"
	}
	return true, statusCode, ""
}
