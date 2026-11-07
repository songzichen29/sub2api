package qqcommunity

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSiteQueryRejectsInvalidArgumentsBeforeReading(t *testing.T) {
	s := New(nil, nil, false, nil)
	for _, q := range []siteQuery{
		{Kind: "balance"}, {Kind: "transfer"}, {Kind: "model_price"},
		{Kind: "model_price", Model: "gpt-5\n/转账"},
		{Kind: "model_price", Model: strings.Repeat("x", 201)},
		{Kind: "channel_status", Page: -1}, {Kind: "channel_status", Page: 10001},
		{Kind: "activity_rules", Activity: "settle"},
		{Kind: "model_price", Model: "gpt-5", Activity: "all"},
		{Kind: "channel_status", Model: "private-token"},
		{Kind: "channel_status", Activity: "all"},
		{Kind: "activity_rules", Model: "gpt-5"},
		{Kind: "account_balance", Model: "private-user"},
		{Kind: "today_usage", Activity: "all"},
		{Kind: "lottery_chances", Page: 2},
	} {
		_, err := s.querySiteInfo(context.Background(), DefaultConfig(), Job{}, q)
		require.Error(t, err, "%+v", q)
	}
}

func TestSiteQueryRetiredFeaturesRejectedWithoutDatabaseOrProvider(t *testing.T) {
	s := New(nil, nil, false, nil)
	for _, q := range []siteQuery{{Kind: "model_price", Model: "model-a"}, {Kind: "activity_rules", Activity: "invite"}} {
		_, err := s.querySiteInfo(context.Background(), DefaultConfig(), Job{}, q)
		require.Error(t, err)
	}
}

func TestSiteQueryFailureResultsNeverExposeProviderErrors(t *testing.T) {
	s := New(nil, nil, false, nil)
	for _, tc := range []struct {
		err  error
		want string
	}{
		{ErrChannelStatusDisabled, "渠道监控未开启"},
		{ErrChannelStatusUnsupported, "被动监控未开启"},
		{errors.New("secret-provider-token"), "渠道状态查询失败"},
	} {
		s.SetChannelStatusProvider(channelStatusProviderFunc(func(context.Context) (*ChannelStatusReport, error) { return nil, tc.err }))
		text, err := s.querySiteInfo(context.Background(), DefaultConfig(), Job{}, siteQuery{Kind: "channel_status"})
		require.NoError(t, err)
		require.Contains(t, text, tc.want)
		require.NotContains(t, text, "secret-provider-token")
	}
	s.SetChannelStatusProvider(channelStatusProviderFunc(func(context.Context) (*ChannelStatusReport, error) { return nil, nil }))
	text, err := s.querySiteInfo(context.Background(), DefaultConfig(), Job{}, siteQuery{Kind: "channel_status"})
	require.NoError(t, err)
	require.Contains(t, text, "查询失败")
}

func TestSiteQueryChannelStatusPreservesSampleAndVisibilitySemantics(t *testing.T) {
	s := New(nil, nil, false, nil)
	report := channelStatusFixture()
	report.Overall = ChannelStatusMetric{HasRequests: true, InsufficientSamples: true, RPM: 123.45}
	report.Groups[0].Metrics.CacheRate = new(float64)
	s.SetChannelStatusProvider(channelStatusProviderFunc(func(context.Context) (*ChannelStatusReport, error) { return report, nil }))
	text, err := s.querySiteInfo(context.Background(), DefaultConfig(), Job{}, siteQuery{Kind: "channel_status"})
	require.NoError(t, err)
	require.Contains(t, text, "近 90 分钟")
	require.Contains(t, text, "更新：2026-10-16 14:30（北京时间）")
	require.Contains(t, text, "成功率样本不足")
	require.Contains(t, text, "缓存率：-\n首 Token 暂无数据")
	require.Contains(t, text, "缓存率：0.00%")
	require.NotContains(t, text, "RPM")
	require.NotContains(t, text, "总吞吐")
}

func TestSiteQueryRulesKeepRewardPrecisionAndDoNotPromiseExecution(t *testing.T) {
	c := CheckinConfig{Enabled: true, MinAmount: .00000001, MaxAmount: .00000002, StreakDays: 7, StreakChances: 3}
	text := siteCheckinRules(c)
	require.Contains(t, text, "USD 0.00000001～USD 0.00000002")
	require.Contains(t, text, "连续签到每 7 天奖励：3 次抽奖机会")
	require.NotContains(t, text, "已到账")
	lottery := defaultLotteryConfig()
	lottery.Enabled, lottery.GuaranteeEnabled = true, true
	text = siteLotteryRules(lottery)
	require.Contains(t, text, "人民币 10.00 元")
	require.NotContains(t, text, "每满 USD")
	require.Contains(t, text, "保底奖池")
	require.Contains(t, text, "普通中奖不重置保底进度")
	text = siteGrowthRules(GrowthConfig{Tasks: []GrowthTask{
		{ID: "active", Name: "公开任务", Kind: "usage_tokens", Target: 1000, Chances: 1, Enabled: true},
		{ID: "disabled", Name: "停用任务", Kind: "binding", Target: 1, Chances: 1},
	}})
	require.Contains(t, text, "累计消耗 1000 Token")
	require.NotContains(t, text, "停用任务")
	require.NotContains(t, text, "已到账")
}

func TestSiteQueryPageBoundsAndWholeBlocks(t *testing.T) {
	require.Contains(t, siteQueryPage([]string{"one"}, 2, "渠道状态"), "共 1 页")
	require.Contains(t, siteQueryPage([]string{strings.Repeat("长", 10001)}, 1, "价格 model"), "价格 model")
	require.NotContains(t, siteQueryPage([]string{"1", "2", "3", "4", "5", "6"}, 1, ""), "指令：")
}
