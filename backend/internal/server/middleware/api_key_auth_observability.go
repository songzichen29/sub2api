package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// APIKeyAuthFailureInfo 记录认证失败时可安全持久化的 Key 观测信息。
// 仅用于排障与聚合，不包含原始 Key。
type APIKeyAuthFailureInfo struct {
	Source      string `json:"source"`
	Fingerprint string `json:"fingerprint"`
	Hint        string `json:"hint"`
}

func setAPIKeyAuthFailureContext(c *gin.Context, source, key string) {
	if c == nil {
		return
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}

	info := APIKeyAuthFailureInfo{
		Source:      strings.ToLower(strings.TrimSpace(source)),
		Fingerprint: buildAPIKeyFingerprint(key),
		Hint:        buildAPIKeyHint(key),
	}
	c.Set(string(ContextKeyAPIKeyAuthFailure), info)
}

// GetAPIKeyAuthFailureInfo 从上下文中获取认证失败观测信息。
func GetAPIKeyAuthFailureInfo(c *gin.Context) (APIKeyAuthFailureInfo, bool) {
	if c == nil {
		return APIKeyAuthFailureInfo{}, false
	}
	value, exists := c.Get(string(ContextKeyAPIKeyAuthFailure))
	if !exists {
		return APIKeyAuthFailureInfo{}, false
	}
	info, ok := value.(APIKeyAuthFailureInfo)
	return info, ok
}

func buildAPIKeyFingerprint(key string) string {
	sum := sha256.Sum256([]byte(key))
	// 截断到前 12 字节（24 hex chars），足够用于聚合，同时避免存储完整摘要。
	return "sha256:" + hex.EncodeToString(sum[:12])
}

func buildAPIKeyHint(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	prefixLen := 3
	if len(key) < prefixLen {
		prefixLen = len(key)
	}
	return key[:prefixLen] + "…(len=" + strconv.Itoa(len(key)) + ")"
}
