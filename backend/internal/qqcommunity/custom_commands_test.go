package qqcommunity

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCustomCommandValidation(t *testing.T) {
	valid := CustomCommand{Command: "/a", Reply: "第一行\n第二行", Enabled: true}
	for _, tt := range []struct {
		name      string
		commands  []CustomCommand
		wantError string
	}{
		{name: "empty"},
		{name: "multiline", commands: []CustomCommand{valid}},
		{name: "unicode and punctuation", commands: []CustomCommand{{Command: "/使用教程_1-帮助", Reply: "教程"}}},
		{name: "case sensitive", commands: []CustomCommand{valid, {Command: "/A", Reply: "大写"}}},
		{name: "reply length boundary", commands: []CustomCommand{{Command: "/a", Reply: strings.Repeat("字", 2000)}}},
		{name: "name length boundary", commands: []CustomCommand{{Command: "/" + strings.Repeat("字", 32), Reply: "好"}}},
		{name: "duplicate including disabled", commands: []CustomCommand{valid, {Command: "/a", Reply: "重复"}}, wantError: "重复"},
		{name: "built in", commands: []CustomCommand{{Command: "/签到", Reply: "覆盖"}}, wantError: "内置指令"},
		{name: "built in alias", commands: []CustomCommand{{Command: "/画图", Reply: "覆盖"}}, wantError: "内置指令"},
		{name: "help alias", commands: []CustomCommand{{Command: "/help", Reply: "覆盖"}}, wantError: "内置指令"},
		{name: "unnormalized", commands: []CustomCommand{{Command: "a", Reply: "好"}}, wantError: "名称须为"},
		{name: "arguments", commands: []CustomCommand{{Command: "/a 参数", Reply: "好"}}, wantError: "名称须为"},
		{name: "mention markup", commands: []CustomCommand{{Command: "/<@bot>", Reply: "好"}}, wantError: "名称须为"},
		{name: "long name", commands: []CustomCommand{{Command: "/" + strings.Repeat("字", 33), Reply: "好"}}, wantError: "名称须为"},
		{name: "blank reply", commands: []CustomCommand{{Command: "/a", Reply: " \n\t"}}, wantError: "回复内容"},
		{name: "long reply", commands: []CustomCommand{{Command: "/a", Reply: strings.Repeat("字", 2001)}}, wantError: "回复内容"},
		{name: "too many", commands: make([]CustomCommand, 21), wantError: "最多 20 条"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := DefaultConfig()
			c.CustomCommands = tt.commands
			err := validateConfig(c)
			if tt.wantError == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.wantError)
			}
		})
	}
	input := []CustomCommand{{Command: " a ", Reply: "\n第一行\n第二行\n", Enabled: true}}
	normalized := normalizeCustomCommands(input)
	require.Equal(t, []CustomCommand{{Command: "/a", Reply: "第一行\n第二行", Enabled: true}}, normalized)
	require.Equal(t, " a ", input[0].Command, "normalization must not mutate the caller's slice")
	require.NotNil(t, normalizeCustomCommands(nil))
}
