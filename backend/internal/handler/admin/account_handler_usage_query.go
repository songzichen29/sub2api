package admin

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/service/usage_provider"
)

// errUsageQueryAccessTokenRequired 用户首次开启用量查询但未填 access_token 时返回。
var errUsageQueryAccessTokenRequired = errors.New("usage_query.access_token is required")

// encryptUsageQueryToken 处理 account.extra.usage_query 中的 access_token 字段：
//
//   - 若字段缺失 / 空字符串：直接跳过
//   - 若值是 dto.UsageQueryAccessTokenMask（前端编辑场景下用户未修改原值）：
//     从 prevExtra 中取出已存在的密文回填；若 prev 中也没有则返回 errUsageQueryAccessTokenRequired
//   - 若值是其它非空字符串：视为新明文 token，调 SecretEncryptor 加密后写回
//
// extra 会被原地修改。enc 必须非 nil（在 handler 构造时由 wire 注入）。
func encryptUsageQueryToken(extra map[string]any, prevExtra map[string]any, enc service.SecretEncryptor) error {
	if extra == nil {
		return nil
	}
	rawUQ, ok := extra["usage_query"]
	if !ok {
		return nil
	}
	uq, ok := rawUQ.(map[string]any)
	if !ok {
		return nil
	}
	provider, _ := uq["provider"].(string)
	if usage_provider.ProviderType(strings.TrimSpace(provider)) == usage_provider.ProviderSub2API {
		// Sub2API 必须始终复用账号 credentials，避免在 extra 中复制 API key。
		delete(uq, "base_url")
		delete(uq, "access_token")
		delete(uq, "user_id")
		extra["usage_query"] = uq
		return nil
	}
	tokenRaw, hasToken := uq["access_token"]
	if !hasToken {
		// 没填 token：仅在 enabled=true 且 prev 也没有 token 时报错
		if isUsageQueryEnabled(uq) && prevTokenCipher(prevExtra) == "" {
			return errUsageQueryAccessTokenRequired
		}
		return nil
	}
	tokenStr, _ := tokenRaw.(string)

	switch tokenStr {
	case "":
		// 用户清空 token：保留原密文，避免误删
		if cipher := prevTokenCipher(prevExtra); cipher != "" {
			uq["access_token"] = cipher
			extra["usage_query"] = uq
			return nil
		}
		// 真的是空：仅在 enabled=true 时报错；否则允许保存（用户可能临时关闭开关）
		if isUsageQueryEnabled(uq) {
			return errUsageQueryAccessTokenRequired
		}
		return nil

	case dto.UsageQueryAccessTokenMask:
		// 用户未修改：回填原密文
		cipher := prevTokenCipher(prevExtra)
		if cipher == "" {
			return errUsageQueryAccessTokenRequired
		}
		uq["access_token"] = cipher
		extra["usage_query"] = uq
		return nil

	default:
		// 新明文：加密后存储
		if enc == nil {
			return errors.New("secret encryptor is not configured")
		}
		cipher, err := enc.Encrypt(tokenStr)
		if err != nil {
			return fmt.Errorf("encrypt usage_query.access_token failed: %w", err)
		}
		uq["access_token"] = cipher
		extra["usage_query"] = uq
		return nil
	}
}

// isUsageQueryEnabled 读取 extra.usage_query.enabled，不存在或类型不匹配时返回 false。
func isUsageQueryEnabled(uq map[string]any) bool {
	v, _ := uq["enabled"].(bool)
	return v
}

// prevTokenCipher 从 prevExtra 中取出已加密的 access_token（若有）。
func prevTokenCipher(prevExtra map[string]any) string {
	if prevExtra == nil {
		return ""
	}
	rawUQ, ok := prevExtra["usage_query"]
	if !ok {
		return ""
	}
	uq, ok := rawUQ.(map[string]any)
	if !ok {
		return ""
	}
	v, _ := uq["access_token"].(string)
	return v
}
