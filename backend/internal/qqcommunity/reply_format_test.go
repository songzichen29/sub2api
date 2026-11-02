package qqcommunity

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestNormalizeReplyLineBreaks(t *testing.T) {
	for _, tc := range []struct {
		name, input, want string
	}{
		{name: "HTML break", input: "第一行<br />\n第二行", want: "第一行\n第二行"},
		{name: "spaced case insensitive break", input: "第一行 < BR   / > 第二行", want: "第一行 \n第二行"},
		{name: "existing Markdown newlines", input: "第一行\r\n\r\n第二行", want: "第一行\n\n第二行"},
		{name: "br without a trailing newline", input: "第一行<br/>第二行", want: "第一行\n第二行"},
		{name: "inline code", input: "使用 `<br />` 换行<br/>下一行", want: "使用 `<br />` 换行\n下一行"},
		{name: "fenced HTML", input: "```html\n<p>第一行<br />第二行</p>\n```\n正文<br/>下一行", want: "```html\n<p>第一行<br />第二行</p>\n```\n正文\n下一行"},
		{name: "tilde fence", input: "~~~html\n<br />\n~~~\n正文<br>下一行", want: "~~~html\n<br />\n~~~\n正文\n下一行"},
		{name: "math unchanged", input: "$x^2$\n$$y=2x$$", want: "$x^2$\n$$y=2x$$"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, normalizeReplyLineBreaks(tc.input))
		})
	}
}

func TestSplitReplyPreservesEntireLongResponse(t *testing.T) {
	for _, input := range []string{
		strings.Repeat("中文🙂", 7000) + "最后一段也必须发送。",
		strings.Repeat(strings.Repeat("中", 800)+"\n", 30) + "结尾",
		strings.Repeat("a", 1501),
	} {
		chunks := splitReply(input)
		require.Equal(t, input, strings.Join(chunks, ""))
		for _, chunk := range chunks {
			require.LessOrEqual(t, utf8.RuneCountInString(chunk), 1500)
			require.True(t, utf8.ValidString(chunk))
		}
	}
	require.Equal(t, []string{"暂无内容。"}, splitReply(""))
}

func TestSplitReplyPrefersWholeLinesWhenSpaceAllows(t *testing.T) {
	first := strings.Repeat("中", 1000) + "\n"
	second := strings.Repeat("文", 700)
	require.Equal(t, []string{first, second}, splitReply(first+second))
}
