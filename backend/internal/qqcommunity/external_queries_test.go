package qqcommunity

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestLiveXLookupDoesNotPretendToHaveSearch(t *testing.T) {
	require.True(t, isLiveXLookup("帮我查下 推特上面 tibo 最新一个推文是什么"))
	for _, text := range []string{"帮我写一个推特客户端", "翻译这段推文", "推特是做什么的", "最新模型价格是多少"} {
		require.False(t, isLiveXLookup(text), text)
	}
	require.Equal(t, "抽奖次数", naturalAccountCommand("我抽中了几次"))
}
