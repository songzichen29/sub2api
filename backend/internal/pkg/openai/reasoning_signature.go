package openai

import (
	"encoding/base64"
	"strings"
)

// MaxReasoningEncryptedContentLen 限制 encrypted_content 的最大长度（32MB）。
const MaxReasoningEncryptedContentLen = 32 * 1024 * 1024

// IsValidReasoningEncryptedContent 校验 encrypted_content 是否具备合法的
// Fernet-like 传输格式。仅做外层结构校验，不验证可解密性。
func IsValidReasoningEncryptedContent(raw string) bool {
	sig := strings.TrimSpace(raw)
	if sig == "" {
		return false
	}
	if sig != raw {
		return false
	}
	if len(sig) > MaxReasoningEncryptedContentLen {
		return false
	}
	if !strings.HasPrefix(sig, "gAAAA") {
		return false
	}
	if hasInvalidBase64URLChar(sig) {
		return false
	}
	decoded, ok := decodeReasoningSignature(sig)
	if !ok {
		return false
	}
	// Fernet 最小长度：version(1) + timestamp(8) + IV(16) + HMAC(32) + 至少 1 个 AES 块(16) = 73
	if len(decoded) < 73 {
		return false
	}
	if decoded[0] != 0x80 {
		return false
	}
	ciphertextLen := len(decoded) - 1 - 8 - 16 - 32
	if ciphertextLen <= 0 || ciphertextLen%16 != 0 {
		return false
	}
	return true
}

func decodeReasoningSignature(sig string) ([]byte, bool) {
	if decoded, err := base64.RawURLEncoding.DecodeString(sig); err == nil {
		return decoded, true
	}
	if decoded, err := base64.URLEncoding.DecodeString(sig); err == nil {
		return decoded, true
	}
	return nil, false
}

func hasInvalidBase64URLChar(sig string) bool {
	for _, r := range sig {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_' || r == '=':
		default:
			return true
		}
	}
	return false
}
