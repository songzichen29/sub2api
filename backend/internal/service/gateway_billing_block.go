package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"unicode/utf16"

	"github.com/tidwall/gjson"
)

// fingerprintSalt 是计算 cc_version 后缀指纹的盐值。
//
// 来源：与 Parrot src/transform/cc_mimicry.py 的 FINGERPRINT_SALT 完全一致；
// 这是真实 Claude Code CLI 抓包推导出的常量，改动会导致 fp 与 CLI 不一致，
// 进一步触发 Anthropic 的第三方检测。
const fingerprintSalt = "59cf53e54c78"

// computeClaudeCodeFingerprint 复刻真实 Claude Code CLI 的 cc_version 指纹算法：
//
//  1. 取 messages 中第一条 role=user 的纯文本（首块 text）
//  2. 按 JS 字符串索引语义取第 4、7、20 个 UTF-16 code unit（不足以 '0' 补齐）
//  3. SHA256(SALT + chars + cc_version) 取 hex 前 3 字符
//
// 算法来自 Claude Code src/utils/fingerprint.ts:computeFingerprint。注意 JS 的
// messageText[i] 是 UTF-16 code unit 索引，不是 UTF-8 byte 或 Unicode code point；
// 当命中 emoji 的半个 surrogate 时，Node crypto.update(string) 会按 UTF-8 将该
// lone surrogate 编码为 U+FFFD。任何偏差都会导致 cc_version=X.Y.Z.{fp} 不一致。
func computeClaudeCodeFingerprint(body []byte, version string) string {
	firstText := extractFirstUserText(body)
	indices := []int{4, 7, 20}
	units := utf16.Encode([]rune(firstText))

	h := sha256.New()
	_, _ = h.Write([]byte(fingerprintSalt))
	for _, i := range indices {
		cu := uint16('0')
		if i < len(units) {
			cu = units[i]
		}
		_, _ = h.Write(jsUTF8BytesForCodeUnit(cu))
	}
	_, _ = h.Write([]byte(version))
	sum := h.Sum(nil)
	return hex.EncodeToString(sum)[:3]
}

// jsUTF8BytesForCodeUnit returns the bytes Node's crypto.update(string) sees
// for a one-code-unit JS string. Lone surrogate code units are encoded as the
// UTF-8 replacement character, matching Node/V8/Bun string-to-UTF8 behavior.
func jsUTF8BytesForCodeUnit(cu uint16) []byte {
	if cu >= 0xD800 && cu <= 0xDFFF {
		return []byte("�")
	}
	return []byte(string(rune(cu)))
}

// extractFirstUserText 提取 messages 中第一条 user 消息的首段 text 内容。
// 兼容 string 和 []block 两种 content 格式。
func extractFirstUserText(body []byte) string {
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return ""
	}
	first := ""
	messages.ForEach(func(_, msg gjson.Result) bool {
		if msg.Get("role").String() != "user" {
			return true
		}
		content := msg.Get("content")
		if content.Type == gjson.String {
			first = content.String()
			return false
		}
		if content.IsArray() {
			content.ForEach(func(_, block gjson.Result) bool {
				if block.Get("type").String() == "text" {
					first = block.Get("text").String()
					return false
				}
				return true
			})
			return false
		}
		return false
	})
	return first
}

// buildBillingAttributionText 构造 system 数组的 billing attribution 文本。
//
// 形态严格对齐真实 Claude Code CLI：
//
//	x-anthropic-billing-header: cc_version=2.1.197.{fp}; cc_entrypoint=cli; cch=00000;
//
// Claude Code v2.1.197 的 native binary 在 attribution block 中包含
// cch=00000; 占位符；网关在 buildUpstreamRequest 的最终 body 阶段用
// signBillingHeaderCCH 执行实验性替换（默认关闭）。若管理员未开启 CCH signing，则由
// stripCCHPlaceholder 移除，确保不会原样发出未签名的 cch=00000。
//
// 此 block 不带 cache_control（与真实 CLI 一致；cache breakpoint 由后续的
// Claude Code prompt block 承担）。
func buildBillingAttributionText(body []byte, cliVersion string) (string, error) {
	if cliVersion == "" {
		return "", fmt.Errorf("cliVersion required")
	}
	fp := computeClaudeCodeFingerprint(body, cliVersion)
	return fmt.Sprintf(
		"x-anthropic-billing-header: cc_version=%s.%s; cc_entrypoint=cli; cch=00000;",
		cliVersion, fp,
	), nil
}
