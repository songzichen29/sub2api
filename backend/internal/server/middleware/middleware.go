package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/googleapi"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ContextKey 定义上下文键类型
type ContextKey string

const (
	// ContextKeyUser 用户上下文键
	ContextKeyUser ContextKey = "user"
	// ContextKeyUserRole 当前用户角色（string）
	ContextKeyUserRole ContextKey = "user_role"
	// ContextKeyAPIKey API密钥上下文键
	ContextKeyAPIKey ContextKey = "api_key"
	// ContextKeyOpsFallbackAPIKey Ops 错误日志专用回退 API Key（不代表已通过鉴权）
	ContextKeyOpsFallbackAPIKey ContextKey = "ops_fallback_api_key"
	// ContextKeyAPIKeyAuthFailure API Key 认证失败观测信息（仅用于日志/排障）
	ContextKeyAPIKeyAuthFailure ContextKey = "api_key_auth_failure"
	// ContextKeySubscription 订阅上下文键
	ContextKeySubscription ContextKey = "subscription"
	// ContextKeyForcePlatform 强制平台（用于 /antigravity 路由）
	ContextKeyForcePlatform ContextKey = "force_platform"
)

// ForcePlatform 返回设置强制平台的中间件
// 同时设置 request.Context（供 Service 使用）和 gin.Context（供 Handler 快速检查）
func ForcePlatform(platform string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 设置到 request.Context，使用 ctxkey.ForcePlatform 供 Service 层读取
		ctx := context.WithValue(c.Request.Context(), ctxkey.ForcePlatform, platform)
		c.Request = c.Request.WithContext(ctx)
		// 同时设置到 gin.Context，供 Handler 快速检查
		c.Set(string(ContextKeyForcePlatform), platform)
		c.Next()
	}
}

// HasForcePlatform 检查是否有强制平台（用于 Handler 跳过分组检查）
func HasForcePlatform(c *gin.Context) bool {
	_, exists := c.Get(string(ContextKeyForcePlatform))
	return exists
}

// GetForcePlatformFromContext 从 gin.Context 获取强制平台
func GetForcePlatformFromContext(c *gin.Context) (string, bool) {
	value, exists := c.Get(string(ContextKeyForcePlatform))
	if !exists {
		return "", false
	}
	platform, ok := value.(string)
	return platform, ok
}

// ErrorResponse 标准错误响应结构
type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NewErrorResponse 创建错误响应
func NewErrorResponse(code, message string) ErrorResponse {
	return ErrorResponse{
		Code:    code,
		Message: message,
	}
}

// AbortWithError 中断请求并返回JSON错误
func AbortWithError(c *gin.Context, statusCode int, code, message string) {
	statusCode, payload := buildProtocolAwareErrorResponse(c, statusCode, code, message)
	c.JSON(statusCode, payload)
	c.Abort()
}

// abortWithOpenAIQuotaError writes the OpenAI-compatible insufficient quota response.
func abortWithOpenAIQuotaError(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"message": message,
			"type":    "insufficient_quota",
			"param":   nil,
			"code":    "insufficient_quota",
		},
	})
	c.Abort()
}

type protocolErrorFormat int

const (
	protocolErrorFormatDefault protocolErrorFormat = iota
	protocolErrorFormatResponses
	protocolErrorFormatChatCompletions
	protocolErrorFormatAnthropic
)

func buildProtocolAwareErrorResponse(c *gin.Context, statusCode int, code, message string) (int, any) {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return statusCode, NewErrorResponse(code, message)
	}

	format := detectProtocolErrorFormat(c.Request.URL.Path)
	if format == protocolErrorFormatDefault {
		return statusCode, NewErrorResponse(code, message)
	}

	mappedStatus, mappedCode := mapGatewayProtocolError(statusCode, code)
	switch format {
	case protocolErrorFormatResponses:
		return mappedStatus, gin.H{
			"error": gin.H{
				"code":    mappedCode,
				"message": message,
			},
		}
	case protocolErrorFormatChatCompletions:
		return mappedStatus, gin.H{
			"error": gin.H{
				"type":    mappedCode,
				"message": message,
			},
		}
	case protocolErrorFormatAnthropic:
		return mappedStatus, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    mappedCode,
				"message": message,
			},
		}
	default:
		return statusCode, NewErrorResponse(code, message)
	}
}

func detectProtocolErrorFormat(path string) protocolErrorFormat {
	p := strings.ToLower(strings.TrimSpace(path))
	switch {
	case strings.HasPrefix(p, "/v1/messages"),
		strings.HasPrefix(p, "/antigravity/v1/messages"):
		return protocolErrorFormatAnthropic
	case strings.HasPrefix(p, "/v1/chat/completions"),
		strings.HasPrefix(p, "/chat/completions"):
		return protocolErrorFormatChatCompletions
	case strings.HasPrefix(p, "/v1/responses"),
		strings.HasPrefix(p, "/responses"),
		strings.HasPrefix(p, "/backend-api/codex/responses"):
		return protocolErrorFormatResponses
	default:
		return protocolErrorFormatDefault
	}
}

func mapGatewayProtocolError(statusCode int, code string) (int, string) {
	switch code {
	case "API_KEY_QUOTA_EXHAUSTED", "INSUFFICIENT_BALANCE", "USAGE_LIMIT_EXCEEDED":
		return http.StatusForbidden, "billing_error"
	}

	switch statusCode {
	case http.StatusBadRequest:
		return statusCode, "invalid_request_error"
	case http.StatusUnauthorized:
		return statusCode, "authentication_error"
	case http.StatusForbidden:
		return statusCode, "permission_error"
	case http.StatusNotFound:
		return statusCode, "not_found_error"
	case http.StatusTooManyRequests:
		return statusCode, "rate_limit_error"
	case http.StatusServiceUnavailable:
		return statusCode, "overloaded_error"
	default:
		return statusCode, "api_error"
	}
}

func AbortForUserLookupError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	if errors.Is(err, service.ErrUserNotFound) {
		AbortWithError(c, http.StatusUnauthorized, "USER_NOT_FOUND", "用户不存在")
		return
	}
	AbortWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "加载用户信息失败")
}

// ──────────────────────────────────────────────────────────
// RequireGroupAssignment — 未分组 Key 拦截中间件
// ──────────────────────────────────────────────────────────

// GatewayErrorWriter 定义网关错误响应格式（不同协议使用不同格式）
type GatewayErrorWriter func(c *gin.Context, status int, message string)

// AnthropicErrorWriter 按 Anthropic API 规范输出错误
func AnthropicErrorWriter(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{
		"type":  "error",
		"error": gin.H{"type": "permission_error", "message": message},
	})
}

// GoogleErrorWriter 按 Google API 规范输出错误
func GoogleErrorWriter(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":    status,
			"message": message,
			"status":  googleapi.HTTPStatusToGoogleStatus(status),
		},
	})
}

// RequireGroupAssignment 检查 API Key 是否已分配到分组，
// 如果未分组且系统设置不允许未分组 Key 调度则返回 403。
func RequireGroupAssignment(settingService *service.SettingService, writeError GatewayErrorWriter) gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey, ok := GetAPIKeyFromContext(c)
		if !ok || apiKey.GroupID != nil {
			c.Next()
			return
		}
		// 未分组 Key — 检查系统设置
		if settingService.IsUngroupedKeySchedulingAllowed(c.Request.Context()) {
			c.Next()
			return
		}
		service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonAPIKeyGroupUnassigned)
		MarkIngressRejected(c, IngressRejectGroupUnassigned)
		writeError(c, http.StatusForbidden, "该 API Key 未分配分组，当前不可使用。请联系管理员分配分组后再试。")
		c.Abort()
	}
}
