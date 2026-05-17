package middleware

import (
	"context"
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// NewAPIKeyAuthMiddleware 创建 API Key 认证中间件
func NewAPIKeyAuthMiddleware(apiKeyService *service.APIKeyService, subscriptionService *service.SubscriptionService, cfg *config.Config) APIKeyAuthMiddleware {
	return APIKeyAuthMiddleware(apiKeyAuthWithSubscription(apiKeyService, subscriptionService, cfg))
}

// apiKeyAuthWithSubscription API Key认证中间件（支持订阅验证）
//
// 中间件职责分为两层：
//   - 鉴权（Authentication）：验证 Key 有效性、用户状态、IP 限制 —— 始终执行
//   - 计费执行（Billing Enforcement）：过期/配额/订阅/余额检查 —— skipBilling 时整块跳过
//
// /v1/usage 端点只需鉴权，不需要计费执行（允许过期/配额耗尽的 Key 查询自身用量）。
func apiKeyAuthWithSubscription(apiKeyService *service.APIKeyService, subscriptionService *service.SubscriptionService, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// ── 1. 提取 API Key ──────────────────────────────────────────

		queryKey := strings.TrimSpace(c.Query("key"))
		queryApiKey := strings.TrimSpace(c.Query("api_key"))
		if queryKey != "" || queryApiKey != "" {
			AbortWithError(c, 400, "api_key_in_query_deprecated", "不再支持通过 query 参数传递 API Key，请改用 Authorization 请求头。")
			return
		}

		// 尝试从Authorization header中提取API key (Bearer scheme)
		authHeader := c.GetHeader("Authorization")
		var apiKeyString string
		var apiKeySource string

		if authHeader != "" {
			// 验证Bearer scheme
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
				apiKeyString = strings.TrimSpace(parts[1])
				if apiKeyString != "" {
					apiKeySource = "authorization"
				}
			}
		}

		// 如果Authorization header中没有，尝试从x-api-key header中提取
		if apiKeyString == "" {
			apiKeyString = strings.TrimSpace(c.GetHeader("x-api-key"))
			if apiKeyString != "" {
				apiKeySource = "x-api-key"
			}
		}

		// 如果x-api-key header中没有，尝试从x-goog-api-key header中提取（Gemini CLI兼容）
		if apiKeyString == "" {
			apiKeyString = strings.TrimSpace(c.GetHeader("x-goog-api-key"))
			if apiKeyString != "" {
				apiKeySource = "x-goog-api-key"
			}
		}

		// 如果所有header都没有API key
		if apiKeyString == "" {
			AbortWithError(c, 401, "API_KEY_REQUIRED", "缺少 API Key，请在 Authorization（Bearer）、x-api-key 或 x-goog-api-key 中提供。")
			return
		}

		// ── 2. 验证 Key 存在 ─────────────────────────────────────────

		apiKey, err := apiKeyService.GetByKey(c.Request.Context(), apiKeyString)
		if err != nil {
			if errors.Is(err, service.ErrAPIKeyNotFound) {
				setAPIKeyAuthFailureContext(c, apiKeySource, apiKeyString)
				AbortWithError(c, 401, "INVALID_API_KEY", "API Key 无效")
				return
			}
			AbortWithError(c, 500, "INTERNAL_ERROR", "API Key 校验失败")
			return
		}

		// ── 3. 基础鉴权（始终执行） ─────────────────────────────────

		// disabled / 未知状态 → 无条件拦截（expired 和 quota_exhausted 留给计费阶段）
		if !apiKey.IsActive() &&
			apiKey.Status != service.StatusAPIKeyExpired &&
			apiKey.Status != service.StatusAPIKeyQuotaExhausted {
			AbortWithError(c, 401, "API_KEY_DISABLED", "API Key 已被禁用")
			return
		}

		// 检查 IP 限制（白名单/黑名单）
		// 注意：错误信息故意模糊，避免暴露具体的 IP 限制机制
		if len(apiKey.IPWhitelist) > 0 || len(apiKey.IPBlacklist) > 0 {
			clientIP := ip.GetTrustedClientIP(c)
			allowed, _ := ip.CheckIPRestrictionWithCompiledRules(clientIP, apiKey.CompiledIPWhitelist, apiKey.CompiledIPBlacklist)
			if !allowed {
				AbortWithError(c, 403, "ACCESS_DENIED", "访问被拒绝")
				return
			}
		}

		// 检查关联的用户
		if apiKey.User == nil {
			AbortWithError(c, 401, "USER_NOT_FOUND", "未找到与 API Key 关联的用户")
			return
		}

		// 检查用户状态
		if !apiKey.User.IsActive() {
			AbortWithError(c, 401, "USER_INACTIVE", "用户账号未激活")
			return
		}

		// ── 4. SimpleMode → early return ─────────────────────────────

		if cfg.RunMode == config.RunModeSimple {
			c.Set(string(ContextKeyAPIKey), apiKey)
			c.Set(string(ContextKeyUser), AuthSubject{
				UserID:      apiKey.User.ID,
				Concurrency: apiKey.User.Concurrency,
			})
			c.Set(string(ContextKeyUserRole), apiKey.User.Role)
			setGroupContext(c, apiKey.Group)
			_ = apiKeyService.TouchLastUsed(c.Request.Context(), apiKey.ID)
			c.Next()
			return
		}

		// ── 5. 加载订阅（订阅模式时始终加载） ───────────────────────

		// skipBilling: /v1/usage 只需鉴权，跳过所有计费执行
		skipBilling := c.Request.URL.Path == "/v1/usage"

		var subscription *service.UserSubscription
		isSubscriptionType := apiKey.Group != nil && apiKey.Group.IsSubscriptionType()

		if isSubscriptionType && subscriptionService != nil {
			sub, subErr := subscriptionService.GetActiveSubscription(
				c.Request.Context(),
				apiKey.User.ID,
				apiKey.Group.ID,
			)
			if subErr != nil {
				if !skipBilling {
					// 通过宽松查询诊断真实错误原因：撤销/到期/暂停/未生效/不存在
					resolved := subscriptionService.ResolveSubscriptionError(
						c.Request.Context(),
						apiKey.User.ID,
						apiKey.Group.ID,
					)
					code, msg := mapSubscriptionLookupError(resolved)
					AbortWithError(c, 403, code, msg)
					return
				}
				// skipBilling: 订阅不存在也放行，handler 会返回可用的数据
			} else {
				subscription = sub
			}
		}

		// ── 6. 计费执行（skipBilling 时整块跳过） ────────────────────

		if !skipBilling {
			// Key 状态检查
			switch apiKey.Status {
			case service.StatusAPIKeyQuotaExhausted:
				AbortWithError(c, 403, "API_KEY_QUOTA_EXHAUSTED", "当前 API Key 配额已用完，请充值或联系管理员。")
				return
			case service.StatusAPIKeyExpired:
				AbortWithError(c, 403, "API_KEY_EXPIRED", "API Key 已过期")
				return
			}

			// 运行时过期/配额检查（即使状态是 active，也要检查时间和用量）
			if apiKey.IsExpired() {
				AbortWithError(c, 403, "API_KEY_EXPIRED", "API Key 已过期")
				return
			}
			if apiKey.IsQuotaExhausted() {
				AbortWithError(c, 403, "API_KEY_QUOTA_EXHAUSTED", "当前 API Key 配额已用完，请充值或联系管理员。")
				return
			}

			// 订阅模式：验证订阅限额
			if subscription != nil {
				needsMaintenance, validateErr := subscriptionService.ValidateAndCheckLimits(c.Request.Context(), subscription, apiKey.Group)
				if validateErr != nil {
					var code, msg string
					switch {
					case errors.Is(validateErr, service.ErrDailyLimitExceeded),
						errors.Is(validateErr, service.ErrWeeklyLimitExceeded),
						errors.Is(validateErr, service.ErrMonthlyLimitExceeded):
						code = "USAGE_LIMIT_EXCEEDED"
						msg = subscriptionValidateErrorMessageCN(validateErr)
					default:
						// 订阅状态类错误（过期/暂停/未生效）：与 GetActiveSubscription 失败时
						// 的错误码/文案保持一致，避免缓存命中路径与 DB 直查路径返回不一致。
						code, msg = mapSubscriptionLookupError(validateErr)
					}
					AbortWithError(c, 403, code, msg)
					return
				}

				// 窗口维护异步化（不阻塞请求）
				if needsMaintenance {
					maintenanceCopy := *subscription
					subscriptionService.DoWindowMaintenance(&maintenanceCopy)
				}
			} else {
				// 非订阅模式 或 订阅模式但 subscriptionService 未注入：回退到余额检查
				if apiKey.User.Balance <= 0 {
					AbortWithError(c, 403, "INSUFFICIENT_BALANCE", "账户余额不足")
					return
				}
			}
		}

		// ── 7. 设置上下文 → Next ─────────────────────────────────────

		if subscription != nil {
			c.Set(string(ContextKeySubscription), subscription)
		}
		c.Set(string(ContextKeyAPIKey), apiKey)
		c.Set(string(ContextKeyUser), AuthSubject{
			UserID:      apiKey.User.ID,
			Concurrency: apiKey.User.Concurrency,
		})
		c.Set(string(ContextKeyUserRole), apiKey.User.Role)
		setGroupContext(c, apiKey.Group)
		_ = apiKeyService.TouchLastUsed(c.Request.Context(), apiKey.ID)

		c.Next()
	}
}

// GetAPIKeyFromContext 从上下文中获取API key
func GetAPIKeyFromContext(c *gin.Context) (*service.APIKey, bool) {
	value, exists := c.Get(string(ContextKeyAPIKey))
	if !exists {
		return nil, false
	}
	apiKey, ok := value.(*service.APIKey)
	return apiKey, ok
}

// GetSubscriptionFromContext 从上下文中获取订阅信息
func GetSubscriptionFromContext(c *gin.Context) (*service.UserSubscription, bool) {
	value, exists := c.Get(string(ContextKeySubscription))
	if !exists {
		return nil, false
	}
	subscription, ok := value.(*service.UserSubscription)
	return subscription, ok
}

func setGroupContext(c *gin.Context, group *service.Group) {
	if !service.IsGroupContextValid(group) {
		return
	}
	if existing, ok := c.Request.Context().Value(ctxkey.Group).(*service.Group); ok && existing != nil && existing.ID == group.ID && service.IsGroupContextValid(existing) {
		return
	}
	ctx := context.WithValue(c.Request.Context(), ctxkey.Group, group)
	c.Request = c.Request.WithContext(ctx)
}

func subscriptionValidateErrorMessageCN(err error) string {
	switch {
	case errors.Is(err, service.ErrSubscriptionNotFound):
		return "未找到有效订阅"
	case errors.Is(err, service.ErrSubscriptionNotStarted):
		return "订阅尚未生效"
	case errors.Is(err, service.ErrSubscriptionExpired):
		return "订阅已过期"
	case errors.Is(err, service.ErrSubscriptionSuspended):
		return "订阅已暂停"
	case errors.Is(err, service.ErrDailyLimitExceeded):
		return "已超过每日使用限额"
	case errors.Is(err, service.ErrWeeklyLimitExceeded):
		return "已超过每周使用限额"
	case errors.Is(err, service.ErrMonthlyLimitExceeded):
		return "已超过每月使用限额"
	default:
		return "订阅状态无效"
	}
}

// mapSubscriptionLookupError 将订阅查找/诊断阶段的错误翻译为给 API 调用方看的错误码与文案。
// 区别于 subscriptionValidateErrorMessageCN（仅返回文案），此函数同时给出 code，
// 用于中间件 GetActiveSubscription 失败后的兜底诊断分支。
func mapSubscriptionLookupError(err error) (code string, msg string) {
	switch {
	case errors.Is(err, service.ErrSubscriptionExpired):
		return "SUBSCRIPTION_EXPIRED", "订阅已到期，请续订或联系管理员。"
	case errors.Is(err, service.ErrSubscriptionSuspended):
		return "SUBSCRIPTION_SUSPENDED", "订阅已暂停，请联系管理员。"
	case errors.Is(err, service.ErrSubscriptionNotStarted):
		return "SUBSCRIPTION_NOT_STARTED", "订阅尚未生效，请稍后再试。"
	case errors.Is(err, service.ErrSubscriptionNotFound):
		return "SUBSCRIPTION_NOT_FOUND", "未找到该分组下的有效订阅，可能尚未订阅或订阅已被撤销。"
	default:
		return "SUBSCRIPTION_INVALID", "订阅状态无效，请联系管理员。"
	}
}
