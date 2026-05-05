package service

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// 账号标签字段相关的常量与规范化规则。
//
// 标签是管理员维度的轻量分类属性——仅用于列表筛选和视觉识别，
// 不参与调度 / 权限 / 计费链路。详见 feature design
// easysdd/features/2026-05-04-account-tags/account-tags-design.md。
//
// 规范化规则（前后端必须一致）：
//   - trim：去除首尾空白
//   - 统一小写：避免 "VIP" / "vip" / "Vip" 在 UI 上并列出现成为伪重复
//   - 去重：保留首次出现位置；最终输出按字典序排序便于稳定对比
//   - 单标签长度 ≤ AccountTagMaxLength
//   - 单账号标签数量 ≤ AccountTagMaxCount
//   - 字符集：中文（Unicode Han）+ ASCII 字母数字 + `-` + `_`
const (
	// AccountTagMaxLength 单个标签最长字符数（按 rune 计，不是字节）。
	AccountTagMaxLength = 30
	// AccountTagMaxCount 单账号最多挂多少个标签。
	AccountTagMaxCount = 20
)

// 标签规范化失败的错误码。前端通过 code 字段做 i18n 文案分发。
var (
	ErrAccountTagLengthExceeded = infraerrors.BadRequest(
		"INVALID_ACCOUNT_TAG_LENGTH",
		fmt.Sprintf("tag length must be <= %d", AccountTagMaxLength),
	)
	ErrAccountTagCountExceeded = infraerrors.BadRequest(
		"TOO_MANY_ACCOUNT_TAGS",
		fmt.Sprintf("each account allows at most %d tags", AccountTagMaxCount),
	)
	ErrAccountTagInvalidCharset = infraerrors.BadRequest(
		"INVALID_ACCOUNT_TAG_CHARSET",
		"tag may only contain CJK characters, ASCII letters/digits, hyphen and underscore",
	)
)

// NormalizeAccountTags 把外部输入的标签数组规范化为可落库的形式。
//
// 输入可以是 nil / 空数组 / 包含空字符串 / 大小写混杂；输出永远是非 nil
// 的字符串数组（可能为空），元素全部小写、去重、按字典序排序。
//
// 任一标签违反长度 / 数量 / 字符集规则时返回错误，调用方应直接把错误透
// 给上游（不做兜底），让 handler 转成对应 HTTP 4xx 响应。
func NormalizeAccountTags(input []string) ([]string, error) {
	if len(input) == 0 {
		return []string{}, nil
	}

	seen := make(map[string]struct{}, len(input))
	out := make([]string, 0, len(input))

	for _, raw := range input {
		tag := strings.ToLower(strings.TrimSpace(raw))
		if tag == "" {
			continue
		}
		if runeLen := countRunes(tag); runeLen > AccountTagMaxLength {
			return nil, fmt.Errorf("%w: %q", ErrAccountTagLengthExceeded, tag)
		}
		if !isValidAccountTagCharset(tag) {
			return nil, fmt.Errorf("%w: %q", ErrAccountTagInvalidCharset, tag)
		}
		if _, dup := seen[tag]; dup {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}

	if len(out) > AccountTagMaxCount {
		return nil, ErrAccountTagCountExceeded
	}

	sort.Strings(out)
	return out, nil
}

// countRunes 返回字符串的 rune 数（用于校验长度，避免把一个汉字算成 3 个字节）。
func countRunes(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

// isValidAccountTagCharset 判断标签字符集是否合法。
// 允许：Unicode Han（CJK 统一表意文字）、ASCII 字母、ASCII 数字、`-`、`_`。
func isValidAccountTagCharset(s string) bool {
	for _, r := range s {
		switch {
		case r == '-' || r == '_':
			continue
		case r >= '0' && r <= '9':
			continue
		case r >= 'a' && r <= 'z':
			continue
		case unicode.Is(unicode.Han, r):
			continue
		default:
			return false
		}
	}
	return true
}
