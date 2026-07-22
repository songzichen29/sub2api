package middleware

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/googleapi"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// APIKeyAuthGoogle is a Google-style error wrapper for API key auth.
func APIKeyAuthGoogle(apiKeyService *service.APIKeyService, cfg *config.Config) gin.HandlerFunc {
	return APIKeyAuthWithSubscriptionGoogle(apiKeyService, nil, cfg)
}

// APIKeyAuthWithSubscriptionGoogle behaves like ApiKeyAuthWithSubscription but returns Google-style errors:
// {"error":{"code":401,"message":"...","status":"UNAUTHENTICATED"}}
//
// It is intended for Gemini native endpoints (/v1beta) to match Gemini SDK expectations.
func APIKeyAuthWithSubscriptionGoogle(apiKeyService *service.APIKeyService, subscriptionService *service.SubscriptionService, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		if rejectInvalidAuthAbuse(c, apiKeyService) {
			abortWithGoogleError(c, 429, "Too many invalid authentication attempts; retry later")
			return
		}
		if apiKeyHeadersTooLarge(c) {
			recordInvalidAuthFailure(c, apiKeyService)
			MarkIngressRejected(c, IngressRejectInvalidAPIKey)
			abortWithGoogleError(c, 401, "Invalid API key")
			return
		}
		if v := strings.TrimSpace(c.Query("api_key")); v != "" {
			recordInvalidAuthFailure(c, apiKeyService)
			MarkIngressRejected(c, IngressRejectQueryAPIKeyDeprecated)
			abortWithGoogleError(c, 400, "不再支持 query 参数 api_key，请改用 Authorization 请求头或 key 参数。")
			return
		}
		apiKeySource, apiKeyString := extractAPIKeyForGoogleWithSource(c)
		if apiKeyString == "" {
			recordInvalidAuthFailure(c, apiKeyService)
			if hasAPIKeyCredentialInput(c) {
				MarkIngressRejected(c, IngressRejectInvalidAPIKey)
			} else {
				MarkIngressRejected(c, IngressRejectAPIKeyRequired)
			}
			abortWithGoogleError(c, 401, "缺少 API Key")
			return
		}
		if len(apiKeyString) > service.MaxAPIKeyCredentialBytes {
			recordInvalidAuthFailure(c, apiKeyService)
			MarkIngressRejected(c, IngressRejectInvalidAPIKey)
			abortWithGoogleError(c, 401, "Invalid API key")
			return
		}

		apiKey, err := apiKeyService.GetByKey(c.Request.Context(), apiKeyString)
		if err != nil {
			if errors.Is(err, service.ErrAPIKeyNotFound) {
				recordInvalidAuthFailure(c, apiKeyService)
				MarkIngressRejected(c, IngressRejectInvalidAPIKey)
				setAPIKeyAuthFailureContext(c, apiKeySource, apiKeyString)
				abortWithGoogleError(c, 401, "API Key 无效")
				return
			}
			if errors.Is(err, service.ErrAPIKeyAuthOverloaded) {
				MarkIngressRejected(c, IngressRejectAPIKeyAuthOverloaded)
				abortWithGoogleError(c, 503, "API key authentication is temporarily unavailable")
				return
			}
			abortWithGoogleError(c, 500, "API Key 校验失败")
			return
		}

		SetOpsFallbackAPIKey(c, apiKey)

		if !apiKey.IsActive() &&
			apiKey.Status != service.StatusAPIKeyExpired &&
			apiKey.Status != service.StatusAPIKeyQuotaExhausted {
			MarkIngressRejected(c, IngressRejectAPIKeyDisabled)
			abortWithGoogleError(c, 401, "API Key 已被禁用")
			return
		}
		if len(apiKey.IPWhitelist) > 0 || len(apiKey.IPBlacklist) > 0 {
			clientIP := ip.GetSecurityClientIP(c, cfg.TrustForwardedIPForAPIKeyACL())
			allowed, _ := ip.CheckIPRestrictionWithCompiledRules(clientIP, apiKey.CompiledIPWhitelist, apiKey.CompiledIPBlacklist)
			if !allowed {
				if clientIP == "" {
					clientIP = "unknown"
				}
				service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonIPRestriction)
				MarkIngressRejected(c, IngressRejectIPRestricted)
				abortWithGoogleError(c, 403, fmt.Sprintf("Access denied. Your IP is %s", clientIP))
				return
			}
		}
		if apiKey.User == nil {
			abortWithGoogleError(c, 401, "未找到与 API Key 关联的用户")
			return
		}
		if !apiKey.User.IsActive() {
			MarkIngressRejected(c, IngressRejectUserInactive)
			abortWithGoogleError(c, 401, "用户账号未激活")
			return
		}
		if code, message, ok := validateAPIKeyGroupAvailable(apiKey); !ok {
			service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonAPIKeyGroupUnavailable)
			if code == "GROUP_DELETED" {
				MarkIngressRejected(c, IngressRejectGroupDeleted)
			} else {
				MarkIngressRejected(c, IngressRejectGroupDisabled)
			}
			abortWithGoogleError(c, 403, message)
			return
		}
		if !validateAPIKeyGroupAllowed(apiKey) {
			service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonAPIKeyGroupUnavailable)
			MarkIngressRejected(c, IngressRejectGroupNotAllowed)
			abortWithGoogleError(c, 403, "API Key 所属专属分组不再允许当前用户使用")
			return
		}

		// 基础鉴权通过后立即写入上下文，保证后续订阅/余额限额在本中间件内拦截时，
		// 外层 OpsErrorLogger 仍可记录 user/api_key/group 归属。
		setAuthenticatedAPIKeyContext(c, apiKey)

		// 简易模式：跳过余额和订阅检查
		if cfg.RunMode == config.RunModeSimple {
			_ = apiKeyService.TouchLastUsed(c.Request.Context(), apiKey.ID)
			c.Next()
			return
		}

		switch apiKey.Status {
		case service.StatusAPIKeyQuotaExhausted:
			abortWithGoogleError(c, 429, "API key 额度已用完")
			return
		case service.StatusAPIKeyExpired:
			abortWithGoogleError(c, 403, "API key 已过期")
			return
		}
		if apiKey.IsExpired() {
			abortWithGoogleError(c, 403, "API key 已过期")
			return
		}
		if apiKey.IsQuotaExhausted() {
			abortWithGoogleError(c, 429, "API key 额度已用完")
			return
		}

		isSubscriptionType := apiKey.Group != nil && apiKey.Group.IsSubscriptionType()
		if isSubscriptionType && subscriptionService != nil {
			subscription, err := subscriptionService.GetActiveSubscription(
				c.Request.Context(),
				apiKey.User.ID,
				apiKey.Group.ID,
			)
			if err != nil {
				abortWithGoogleError(c, 403, "该分组下未找到有效订阅")
				return
			}

			c.Set(string(ContextKeySubscription), subscription)

			needsMaintenance, err := subscriptionService.ValidateAndCheckLimits(c.Request.Context(), subscription, apiKey.Group)
			if err != nil {
				status := 403
				if errors.Is(err, service.ErrDailyLimitExceeded) ||
					errors.Is(err, service.ErrWeeklyLimitExceeded) ||
					errors.Is(err, service.ErrMonthlyLimitExceeded) {
					status = 403
				}
				abortWithGoogleError(c, status, subscriptionValidateErrorMessageCN(err))
				return
			}

			if needsMaintenance {
				maintenanceCopy := *subscription
				subscriptionService.DoWindowMaintenance(&maintenanceCopy)
			}
		} else {
			if apiKey.User.Balance <= 0 {
				abortWithGoogleError(c, 403, "账户余额不足")
				return
			}
		}

		_ = apiKeyService.TouchLastUsed(c.Request.Context(), apiKey.ID)
		c.Next()
	}
}

// extractAPIKeyForGoogle extracts API key for Google/Gemini endpoints.
// Priority: x-goog-api-key > Authorization: Bearer > x-api-key > query key
// This allows OpenClaw and other clients using Bearer auth to work with Gemini endpoints.
//
//nolint:unused // retained as a small compatibility wrapper for callers that only need the key.
func extractAPIKeyForGoogle(c *gin.Context) string {
	_, key := extractAPIKeyForGoogleWithSource(c)
	return key
}

func extractAPIKeyForGoogleWithSource(c *gin.Context) (string, string) {
	// 1) preferred: Gemini native header
	if k := strings.TrimSpace(c.GetHeader("x-goog-api-key")); k != "" {
		return "x-goog-api-key", k
	}

	// 2) fallback: Authorization: Bearer <key>
	auth := strings.TrimSpace(c.GetHeader("Authorization"))
	if auth != "" {
		parts := strings.SplitN(auth, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			if k := strings.TrimSpace(parts[1]); k != "" {
				return "authorization", k
			}
		}
	}

	// 3) x-api-key header (backward compatibility)
	if k := strings.TrimSpace(c.GetHeader("x-api-key")); k != "" {
		return "x-api-key", k
	}

	// 4) query parameter key (for specific paths)
	if allowGoogleQueryKey(c.Request.URL.Path) {
		if v := strings.TrimSpace(c.Query("key")); v != "" {
			return "query:key", v
		}
	}

	return "", ""
}

func allowGoogleQueryKey(path string) bool {
	return strings.HasPrefix(path, "/v1beta") || strings.HasPrefix(path, "/antigravity/v1beta")
}

func abortWithGoogleError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":    status,
			"message": message,
			"status":  googleapi.HTTPStatusToGoogleStatus(status),
		},
	})
	c.Abort()
}
