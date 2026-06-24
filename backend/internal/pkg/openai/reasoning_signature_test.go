package openai

import (
	"encoding/base64"
	"strings"
	"testing"
)

// buildValidEncryptedContent 构造符合 Fernet 格式的测试用 encrypted_content。
// version(1) + timestamp(8) + IV(16) + ciphertext(16*n) + HMAC(32) = 73+ 字节
func buildValidEncryptedContent(ciphertextBlocks int) string {
	if ciphertextBlocks < 1 {
		ciphertextBlocks = 1
	}
	payloadLen := 1 + 8 + 16 + 16*ciphertextBlocks + 32
	payload := make([]byte, payloadLen)
	payload[0] = 0x80
	return base64.RawURLEncoding.EncodeToString(payload)
}

func TestIsValidReasoningEncryptedContent_Valid(t *testing.T) {
	ec := buildValidEncryptedContent(1)
	if !strings.HasPrefix(ec, "gAAAA") {
		t.Fatalf("测试数据前缀不正确: %s", ec[:10])
	}
	if !IsValidReasoningEncryptedContent(ec) {
		t.Errorf("合法 encrypted_content 应通过校验")
	}
}

func TestIsValidReasoningEncryptedContent_ValidMultiBlock(t *testing.T) {
	ec := buildValidEncryptedContent(4)
	if !IsValidReasoningEncryptedContent(ec) {
		t.Errorf("多块密文的 encrypted_content 应通过校验")
	}
}

func TestIsValidReasoningEncryptedContent_Empty(t *testing.T) {
	if IsValidReasoningEncryptedContent("") {
		t.Errorf("空字符串不应通过校验")
	}
}

func TestIsValidReasoningEncryptedContent_Whitespace(t *testing.T) {
	ec := buildValidEncryptedContent(1)
	if IsValidReasoningEncryptedContent(" " + ec) {
		t.Errorf("前导空格不应通过校验")
	}
	if IsValidReasoningEncryptedContent(ec + " ") {
		t.Errorf("尾部空格不应通过校验")
	}
}

func TestIsValidReasoningEncryptedContent_WrongPrefix(t *testing.T) {
	payload := make([]byte, 73)
	payload[0] = 0x80
	ec := base64.RawURLEncoding.EncodeToString(payload)
	if strings.HasPrefix(ec, "gAAAA") {
		t.Skip("碰巧以 gAAAA 开头，跳过")
	}
	if IsValidReasoningEncryptedContent(ec) {
		t.Errorf("无 gAAAA 前缀不应通过校验")
	}
}

func TestIsValidReasoningEncryptedContent_InvalidBase64Char(t *testing.T) {
	if IsValidReasoningEncryptedContent("gAAAA!!!invalid") {
		t.Errorf("含非法 base64url 字符不应通过校验")
	}
}

func TestIsValidReasoningEncryptedContent_TooShort(t *testing.T) {
	if IsValidReasoningEncryptedContent("gAAAA") {
		t.Errorf("过短的内容不应通过校验")
	}
}

func TestIsValidReasoningEncryptedContent_WrongVersion(t *testing.T) {
	payload := make([]byte, 73)
	payload[0] = 0x01
	ec := base64.RawURLEncoding.EncodeToString(payload)
	if strings.HasPrefix(ec, "gAAAA") {
		t.Skip("碰巧以 gAAAA 开头，跳过")
	}
	if IsValidReasoningEncryptedContent(ec) {
		t.Errorf("版本号非 0x80 不应通过校验")
	}
}

func TestIsValidReasoningEncryptedContent_NonStringTypes(t *testing.T) {
	if IsValidReasoningEncryptedContent("null") {
		t.Errorf("字面量 null 不应通过校验")
	}
	if IsValidReasoningEncryptedContent("12345") {
		t.Errorf("数字字符串不应通过校验")
	}
}

func TestIsValidReasoningEncryptedContent_ExceedsMaxLen(t *testing.T) {
	long := "gAAAA" + strings.Repeat("A", MaxReasoningEncryptedContentLen+1)
	if IsValidReasoningEncryptedContent(long) {
		t.Errorf("超过最大长度限制不应通过校验")
	}
}
