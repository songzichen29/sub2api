// Package claude provides constants and helpers for Claude API integration.
package claude

import "strings"

// Claude Code 客户端相关常量

// Beta header 常量
//
// 这里的常量对齐真实 Claude Code CLI 的最新流量（截至 2026-04）。
// 选型参考：与 Parrot (src/transform/cc_mimicry.py) 的 BETAS 保持一致，
// 原因：Anthropic 上游会基于 anthropic-beta 的完整集合判定请求来源；
// 缺少任何"官方 Claude Code 请求才会带"的 beta，都会被降级到第三方额度，
// 对应报错：`Third-party apps now draw from your extra usage, not your plan limits.`
const (
	BetaOAuth                    = "oauth-2025-04-20"
	BetaClaudeCode               = "claude-code-20250219"
	BetaInterleavedThinking      = "interleaved-thinking-2025-05-14"
	BetaFineGrainedToolStreaming = "fine-grained-tool-streaming-2025-05-14"
	BetaTokenCounting            = "token-counting-2024-11-01"
	BetaContext1M                = "context-1m-2025-08-07"
	BetaFastMode                 = "fast-mode-2026-02-01"
	BetaWebSearch                = "web-search-2025-03-05"

	// 新增（对齐官方 CLI 2.1.19x 以来的流量）
	BetaPromptCachingScope = "prompt-caching-scope-2026-01-05"
	BetaEffort             = "effort-2025-11-24"
	BetaRedactThinking     = "redact-thinking-2026-02-12"
	BetaContextManagement  = "context-management-2025-06-27"
	BetaExtendedCacheTTL   = "extended-cache-ttl-2025-04-11"

	// v2.1.197 binary / 真实 telemetry 中确认的 beta 令牌（参考用，按需加入 mimicry）
	BetaContextHint           = "context-hint-2026-04-09"
	BetaMidConversationSystem = "mid-conversation-system-2026-04-07"
	BetaConversationSystem    = BetaMidConversationSystem // backward-compatible alias
	BetaManagedAgents         = "managed-agents-2026-04-01"
	BetaStructuredOutputs     = "structured-outputs-2025-12-15"
	BetaTaskBudgets           = "task-budgets-2026-03-13"
	BetaThinkingTokenCount    = "thinking-token-count-2026-05-13"
	BetaTokenCount            = BetaThinkingTokenCount // backward-compatible alias
	BetaUserProfiles          = "user-profiles-2026-03-24"
	BetaSideFallback          = "side-fallback-2026-06-01"
	BetaFallbackCredit        = "fallback-credit-2026-06-01"
	BetaCodeExecution         = "code-execution-2025-08-25"
	BetaAdvisorTool           = "advisor-tool-2026-03-01"
	BetaAfkMode               = "afk-mode-2026-01-31"
)

// DroppedBetas 是转发时需要从 anthropic-beta header 中移除的 beta token 列表。
// 这些 token 是客户端特有的，不应透传给上游 API。
var DroppedBetas = []string{}

// DefaultBetaHeader Claude Code 客户端默认的 anthropic-beta header
const DefaultBetaHeader = BetaClaudeCode + "," + BetaOAuth + "," + BetaInterleavedThinking + "," + BetaFineGrainedToolStreaming

// MessageBetaHeaderNoTools /v1/messages 在无工具时的 beta header
//
// NOTE: Claude Code OAuth credentials are scoped to Claude Code. When we "mimic"
// Claude Code for non-Claude-Code clients, we must include the claude-code beta
// even if the request doesn't use tools, otherwise upstream may reject the
// request as a non-Claude-Code API request.
const MessageBetaHeaderNoTools = BetaClaudeCode + "," + BetaOAuth + "," + BetaInterleavedThinking

// MessageBetaHeaderWithTools /v1/messages 在有工具时的 beta header
const MessageBetaHeaderWithTools = BetaClaudeCode + "," + BetaOAuth + "," + BetaInterleavedThinking

// CountTokensBetaHeader count_tokens 请求使用的 anthropic-beta header
const CountTokensBetaHeader = BetaClaudeCode + "," + BetaOAuth + "," + BetaInterleavedThinking + "," + BetaTokenCounting

// HaikuBetaHeader Haiku 模型在 OAuth 真实客户端透传路径上的默认 anthropic-beta header。
// OAuth mimic 路径统一使用 FullClaudeCodeMimicryBetas。
const HaikuBetaHeader = BetaOAuth + "," + BetaInterleavedThinking

// APIKeyBetaHeader API-key 账号建议使用的 anthropic-beta header（不包含 oauth）
const APIKeyBetaHeader = BetaClaudeCode + "," + BetaInterleavedThinking + "," + BetaFineGrainedToolStreaming

// APIKeyHaikuBetaHeader Haiku 模型在 API-key 账号下使用的 anthropic-beta header（不包含 oauth / claude-code）
const APIKeyHaikuBetaHeader = BetaInterleavedThinking

// DefaultCacheControlTTL 是网关代理为自己生成的 cache_control 块默认使用的 ttl。
// 真实 Claude Code CLI v2.1.197 当前使用 "1h"。
const DefaultCacheControlTTL = "1h"

// CLICurrentVersion 是 sub2api 当前对外伪装的 Claude Code CLI 版本号（三段 semver）。
// 用于 billing attribution block 中的 cc_version=X.Y.Z.{fp} 前缀以及 fingerprint 计算。
// 必须与 DefaultHeaders["User-Agent"] 中的版本号严格一致；不一致会被 Anthropic 判第三方。
const CLICurrentVersion = "2.1.220"

// CLIBuildTime 是从 Claude Code v2.1.197 native binary 中提取的真实 build_time。
const CLIBuildTime = "2026-06-29T19:08:42Z"

// HasContext1MMarker reports whether a model string is explicitly marked for
// Claude Code's 1M-context beta. Real Claude Code adds context-1m only when the
// selected model/env model contains the literal "[1m]" marker.
func HasContext1MMarker(model string) bool {
	return strings.Contains(strings.ToLower(model), "[1m]")
}

// claudeCodeHeaderWireOrder is the application-level HTTP/1.1 header order
// captured from Claude Code v2.1.197. Transport headers (Connection, Host,
// Accept-Encoding, Content-Length) are written after this list by the custom
// Anthropic H1 round tripper.
var claudeCodeHeaderWireOrder = []string{
	"Accept",
	"Authorization",
	"Content-Type",
	"User-Agent",
	"X-Claude-Code-Session-Id",
	"X-Stainless-Arch",
	"X-Stainless-Lang",
	"X-Stainless-OS",
	"X-Stainless-Package-Version",
	"X-Stainless-Retry-Count",
	"X-Stainless-Runtime",
	"X-Stainless-Runtime-Version",
	"X-Stainless-Timeout",
	"anthropic-beta",
	"anthropic-dangerous-direct-browser-access",
	"anthropic-version",
	"x-app",
}

// ClaudeCodeHeaderWireOrder returns a copy of the captured Claude Code
// application header order.
func ClaudeCodeHeaderWireOrder() []string {
	out := make([]string, len(claudeCodeHeaderWireOrder))
	copy(out, claudeCodeHeaderWireOrder)
	return out
}

// FullClaudeCodeMimicryBetas 返回最"像"真实 Claude Code CLI 的 beta 列表，
// 用于 telemetry event_data.betas。默认不包含 context-1m；该 beta 必须通过
// FullClaudeCodeMimicryBetasForModel 按模型名中的 [1m] 标记条件化加入。
func FullClaudeCodeMimicryBetas() []string {
	return FullClaudeCodeMimicryBetasForModel("")
}

// FullClaudeCodeMimicryBetasForModel 返回 telemetry event_data.betas；顺序与
// Claude Code v2.1.197 telemetry 样本一致，但 context-1m 仅在模型含 [1m] 时加入。
//
// 注意：真实 telemetry 的 betas 字段不包含 oauth-2025-04-20；API 请求 header
// 的 anthropic-beta 仍需要 OAuth token。请求 header 请使用
// ClaudeCodeOAuthMimicryRequestBetasForModel。
func FullClaudeCodeMimicryBetasForModel(model string) []string {
	betas := []string{BetaClaudeCode}
	if HasContext1MMarker(model) {
		betas = append(betas, BetaContext1M)
	}
	betas = append(betas,
		BetaInterleavedThinking,
		BetaRedactThinking,
		BetaThinkingTokenCount,
		BetaContextManagement,
		BetaPromptCachingScope,
		BetaMidConversationSystem,
	)
	return betas
}

// ClaudeCodeOAuthMimicryRequestBetas 返回 OAuth mimic 路径中 API 请求 header
// anthropic-beta 应使用的 beta 集合。默认不包含 context-1m；该 beta 必须通过
// ClaudeCodeOAuthMimicryRequestBetasForModel 按模型名中的 [1m] 标记条件化加入。
func ClaudeCodeOAuthMimicryRequestBetas() []string {
	return ClaudeCodeOAuthMimicryRequestBetasForModel("")
}

// ClaudeCodeOAuthMimicryRequestBetasForModel 返回 OAuth mimic 路径中 API 请求
// header anthropic-beta 应使用的 beta 集合。它刻意与 telemetry betas 区分：
// header 需要 oauth-2025-04-20，而 telemetry 的 betas 字段不包含该 token。
func ClaudeCodeOAuthMimicryRequestBetasForModel(model string) []string {
	betas := []string{BetaClaudeCode, BetaOAuth}
	if HasContext1MMarker(model) {
		betas = append(betas, BetaContext1M)
	}
	betas = append(betas,
		BetaInterleavedThinking,
		BetaRedactThinking,
		BetaThinkingTokenCount,
		BetaContextManagement,
		BetaPromptCachingScope,
		BetaMidConversationSystem,
		BetaExtendedCacheTTL,
	)
	return betas
}

// DefaultHeaders 是 Claude Code 客户端默认请求头。
var DefaultHeaders = map[string]string{
	// Keep these in sync with recent Claude CLI traffic to reduce the chance
	// that Claude Code-scoped OAuth credentials are rejected as "non-CLI" usage.
	// 版本参考：对齐 Parrot (src/transform/cc_mimicry.py:49) 的 CLI_USER_AGENT。
	"User-Agent":                                "claude-cli/" + CLICurrentVersion + " (external, cli)",
	"X-Stainless-Lang":                          "js",
	"X-Stainless-Package-Version":               "0.94.0",
	"X-Stainless-OS":                            "Linux",
	"X-Stainless-Arch":                          "x64",
	"X-Stainless-Runtime":                       "node",
	"X-Stainless-Runtime-Version":               "v26.3.0",
	"X-Stainless-Retry-Count":                   "0",
	"X-Stainless-Timeout":                       "600",
	"X-App":                                     "cli",
	"Anthropic-Dangerous-Direct-Browser-Access": "true",
	"Accept-Encoding":                           "gzip, deflate, br, zstd",
}

// Model 表示一个 Claude 模型
type Model struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
	CreatedAt   string `json:"created_at"`
}

// DefaultModels Claude Code 客户端支持的默认模型列表
var DefaultModels = []Model{
	{
		ID:          "claude-fable-5",
		Type:        "model",
		DisplayName: "Claude Fable 5",
		CreatedAt:   "2026-06-09T00:00:00Z",
	},
	{
		ID:          "claude-opus-4-5-20251101",
		Type:        "model",
		DisplayName: "Claude Opus 4.5",
		CreatedAt:   "2025-11-01T00:00:00Z",
	},
	{
		ID:          "claude-opus-4-6",
		Type:        "model",
		DisplayName: "Claude Opus 4.6",
		CreatedAt:   "2026-02-06T00:00:00Z",
	},
	{
		ID:          "claude-opus-4-7",
		Type:        "model",
		DisplayName: "Claude Opus 4.7",
		CreatedAt:   "2026-04-17T00:00:00Z",
	},
	{
		ID:          "claude-opus-4-8",
		Type:        "model",
		DisplayName: "Claude Opus 4.8",
		CreatedAt:   "2026-05-29T00:00:00Z",
	},
	{
		ID:          "claude-opus-5",
		Type:        "model",
		DisplayName: "Claude Opus 5",
		CreatedAt:   "2026-07-25T00:00:00Z",
	},
	{
		ID:          "claude-sonnet-5",
		Type:        "model",
		DisplayName: "Claude Sonnet 5",
		CreatedAt:   "2026-07-01T00:00:00Z",
	},
	{
		ID:          "claude-sonnet-4-6",
		Type:        "model",
		DisplayName: "Claude Sonnet 4.6",
		CreatedAt:   "2026-02-18T00:00:00Z",
	},
	{
		ID:          "claude-sonnet-4-5-20250929",
		Type:        "model",
		DisplayName: "Claude Sonnet 4.5",
		CreatedAt:   "2025-09-29T00:00:00Z",
	},
	{
		ID:          "claude-haiku-4-5-20251001",
		Type:        "model",
		DisplayName: "Claude Haiku 4.5",
		CreatedAt:   "2025-10-01T00:00:00Z",
	},
}

// DefaultModelIDs 返回默认模型的 ID 列表
func DefaultModelIDs() []string {
	ids := make([]string, len(DefaultModels))
	for i, m := range DefaultModels {
		ids[i] = m.ID
	}
	return ids
}

// DefaultTestModel 测试时使用的默认模型
const DefaultTestModel = "claude-sonnet-4-5-20250929"

// ModelIDOverrides Claude OAuth 请求需要的模型 ID 映射
var ModelIDOverrides = map[string]string{
	"claude-sonnet-4-5": "claude-sonnet-4-5-20250929",
	"claude-opus-4-5":   "claude-opus-4-5-20251101",
	"claude-haiku-4-5":  "claude-haiku-4-5-20251001",
}

// ModelIDReverseOverrides 用于将上游模型 ID 还原为短名
var ModelIDReverseOverrides = map[string]string{
	"claude-sonnet-4-5-20250929": "claude-sonnet-4-5",
	"claude-opus-4-5-20251101":   "claude-opus-4-5",
	"claude-haiku-4-5-20251001":  "claude-haiku-4-5",
}

// NormalizeModelID 根据 Claude OAuth 规则映射模型
func NormalizeModelID(id string) string {
	if id == "" {
		return id
	}
	if mapped, ok := ModelIDOverrides[id]; ok {
		return mapped
	}
	return id
}

// DenormalizeModelID 将上游模型 ID 转换为短名
func DenormalizeModelID(id string) string {
	if id == "" {
		return id
	}
	if mapped, ok := ModelIDReverseOverrides[id]; ok {
		return mapped
	}
	return id
}
