//go:build unit

package service_test

import (
	"errors"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// TestNormalizeAccountTags_TrimAndLowercase 验证 trim + 统一小写正例。
func TestNormalizeAccountTags_TrimAndLowercase(t *testing.T) {
	got, err := service.NormalizeAccountTags([]string{"  VIP  ", "Prod", "  west  "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"prod", "vip", "west"} // 字典序输出
	assertStringSliceEqual(t, got, want)
}

// TestNormalizeAccountTags_DeduplicatesPreservingFirst 验证大小写归一化后的去重。
func TestNormalizeAccountTags_DeduplicatesPreservingFirst(t *testing.T) {
	got, err := service.NormalizeAccountTags([]string{"VIP", " vip ", "Vip", "vip"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"vip"}
	assertStringSliceEqual(t, got, want)
}

// TestNormalizeAccountTags_HandlesNilAndEmpty 验证 nil/空输入兜底。
func TestNormalizeAccountTags_HandlesNilAndEmpty(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   []string
	}{
		{"nil", nil},
		{"empty", []string{}},
		{"only whitespace", []string{"  ", "\t", ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := service.NormalizeAccountTags(tc.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got == nil {
				t.Fatal("expected non-nil slice (use empty slice tilde nil to avoid JSON null)")
			}
			if len(got) != 0 {
				t.Fatalf("expected empty slice, got %v", got)
			}
		})
	}
}

// TestNormalizeAccountTags_AllowsCJK 验证字符集正例（中文 + ASCII + - + _）。
func TestNormalizeAccountTags_AllowsCJK(t *testing.T) {
	got, err := service.NormalizeAccountTags([]string{"生产", "vip-1", "test_a", "abc"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"abc", "test_a", "vip-1", "生产"}
	assertStringSliceEqual(t, got, want)
}

// TestNormalizeAccountTags_RejectsTooLongTag 验证单标签长度反例。
// AccountTagMaxLength 是 30，构造一个 31 字符的标签。
func TestNormalizeAccountTags_RejectsTooLongTag(t *testing.T) {
	long := ""
	for i := 0; i < 31; i++ {
		long += "a"
	}
	_, err := service.NormalizeAccountTags([]string{"vip", long})
	if err == nil {
		t.Fatal("expected error for too-long tag, got nil")
	}
	if !errors.Is(err, service.ErrAccountTagLengthExceeded) {
		t.Fatalf("expected ErrAccountTagLengthExceeded, got %v", err)
	}
	// 通过 errors.As 提取 ApplicationError 验证错误码可被 handler 用
	var appErr *infraerrors.ApplicationError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected ApplicationError in chain, got %v", err)
	}
	if appErr.Reason != "INVALID_ACCOUNT_TAG_LENGTH" {
		t.Fatalf("unexpected reason: %s", appErr.Reason)
	}
}

// TestNormalizeAccountTags_RejectsTooManyTags 验证标签数量反例。
func TestNormalizeAccountTags_RejectsTooManyTags(t *testing.T) {
	tags := make([]string, service.AccountTagMaxCount+1)
	for i := range tags {
		tags[i] = "tag" + string(rune('a'+i))
	}
	_, err := service.NormalizeAccountTags(tags)
	if err == nil {
		t.Fatal("expected error for too many tags, got nil")
	}
	if !errors.Is(err, service.ErrAccountTagCountExceeded) {
		t.Fatalf("expected ErrAccountTagCountExceeded, got %v", err)
	}
}

// TestNormalizeAccountTags_RejectsInvalidCharset 验证字符集反例。
func TestNormalizeAccountTags_RejectsInvalidCharset(t *testing.T) {
	for _, badTag := range []string{
		"vip!",       // 感叹号不在白名单
		"hello world", // 空格（非首尾）
		"x@y",         // @ 符号
		"a/b",         // 斜杠
	} {
		t.Run(badTag, func(t *testing.T) {
			_, err := service.NormalizeAccountTags([]string{badTag})
			if err == nil {
				t.Fatalf("expected error for invalid charset %q, got nil", badTag)
			}
			if !errors.Is(err, service.ErrAccountTagInvalidCharset) {
				t.Fatalf("expected ErrAccountTagInvalidCharset, got %v", err)
			}
		})
	}
}

// TestNormalizeAccountTags_AcceptsBoundaryLength 验证恰好 30 字符的边界正例。
func TestNormalizeAccountTags_AcceptsBoundaryLength(t *testing.T) {
	exactly30 := ""
	for i := 0; i < 30; i++ {
		exactly30 += "a"
	}
	got, err := service.NormalizeAccountTags([]string{exactly30})
	if err != nil {
		t.Fatalf("expected boundary length 30 to be accepted, got error: %v", err)
	}
	if len(got) != 1 || got[0] != exactly30 {
		t.Fatalf("expected [%q], got %v", exactly30, got)
	}
}

// TestNormalizeAccountTags_AcceptsBoundaryCount 验证恰好 20 个标签的边界正例。
func TestNormalizeAccountTags_AcceptsBoundaryCount(t *testing.T) {
	tags := make([]string, service.AccountTagMaxCount)
	for i := range tags {
		// 用 rune 避免重复
		tags[i] = "tag" + string(rune('a'+i))
	}
	got, err := service.NormalizeAccountTags(tags)
	if err != nil {
		t.Fatalf("expected exactly %d tags to be accepted, got error: %v", service.AccountTagMaxCount, err)
	}
	if len(got) != service.AccountTagMaxCount {
		t.Fatalf("expected %d tags, got %d", service.AccountTagMaxCount, len(got))
	}
}

func assertStringSliceEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("length mismatch: got=%v want=%v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("element %d mismatch: got=%q want=%q (full got=%v)", i, got[i], want[i], got)
		}
	}
}
