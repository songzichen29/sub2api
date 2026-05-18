//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGroup_GetImagePrice_1K 测试 1K 尺寸返回正确价格
func TestGroup_GetImagePrice_1K(t *testing.T) {
	price := 0.10
	group := &Group{
		ImagePrice1K: &price,
	}

	result := group.GetImagePrice("1K")
	require.NotNil(t, result)
	require.InDelta(t, 0.10, *result, 0.0001)
}

// TestGroup_GetImagePrice_2K 测试 2K 尺寸返回正确价格
func TestGroup_GetImagePrice_2K(t *testing.T) {
	price := 0.15
	group := &Group{
		ImagePrice2K: &price,
	}

	result := group.GetImagePrice("2K")
	require.NotNil(t, result)
	require.InDelta(t, 0.15, *result, 0.0001)
}

// TestGroup_GetImagePrice_4K 测试 4K 尺寸返回正确价格
func TestGroup_GetImagePrice_4K(t *testing.T) {
	price := 0.30
	group := &Group{
		ImagePrice4K: &price,
	}

	result := group.GetImagePrice("4K")
	require.NotNil(t, result)
	require.InDelta(t, 0.30, *result, 0.0001)
}

// TestGroup_GetImagePrice_UnknownSize 测试未知尺寸回退 2K
func TestGroup_GetImagePrice_UnknownSize(t *testing.T) {
	price2K := 0.15
	group := &Group{
		ImagePrice2K: &price2K,
	}

	// 未知尺寸 "3K" 应该回退到 2K
	result := group.GetImagePrice("3K")
	require.NotNil(t, result)
	require.InDelta(t, 0.15, *result, 0.0001)

	// 空字符串也回退到 2K
	result = group.GetImagePrice("")
	require.NotNil(t, result)
	require.InDelta(t, 0.15, *result, 0.0001)
}

// TestGroup_GetImagePrice_NilValues 测试未配置时返回 nil
func TestGroup_GetImagePrice_NilValues(t *testing.T) {
	group := &Group{
		// 所有 ImagePrice 字段都是 nil
	}

	require.Nil(t, group.GetImagePrice("1K"))
	require.Nil(t, group.GetImagePrice("2K"))
	require.Nil(t, group.GetImagePrice("4K"))
	require.Nil(t, group.GetImagePrice("unknown"))
}

// TestGroup_GetImagePrice_PartialConfig 测试部分配置
func TestGroup_GetImagePrice_PartialConfig(t *testing.T) {
	price1K := 0.10
	group := &Group{
		ImagePrice1K: &price1K,
		// ImagePrice2K 和 ImagePrice4K 未配置
	}

	result := group.GetImagePrice("1K")
	require.NotNil(t, result)
	require.InDelta(t, 0.10, *result, 0.0001)

	// 2K 和 4K 返回 nil
	require.Nil(t, group.GetImagePrice("2K"))
	require.Nil(t, group.GetImagePrice("4K"))
}

func TestGroup_AllowsDailyOverdraft(t *testing.T) {
	daily := 80.0
	weekly := 560.0

	tests := []struct {
		name  string
		group *Group
		want  bool
	}{
		{
			name: "disabled flag",
			group: &Group{
				SubscriptionType: SubscriptionTypeSubscription,
				DailyLimitUSD:    &daily,
				WeeklyLimitUSD:   &weekly,
			},
			want: false,
		},
		{
			name: "standard group never allows",
			group: &Group{
				SubscriptionType:    SubscriptionTypeStandard,
				DailyLimitUSD:       &daily,
				WeeklyLimitUSD:      &weekly,
				AllowDailyOverdraft: true,
			},
			want: false,
		},
		{
			name: "daily pool allows",
			group: &Group{
				SubscriptionType:    SubscriptionTypeSubscription,
				DailyLimitUSD:       &daily,
				AllowDailyOverdraft: true,
			},
			want: true,
		},
		{
			name: "weekly field does not control overdraft",
			group: &Group{
				SubscriptionType:    SubscriptionTypeSubscription,
				DailyLimitUSD:       &daily,
				WeeklyLimitUSD:      &weekly,
				AllowDailyOverdraft: true,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.group.AllowsDailyOverdraft())
			require.Equal(t, tt.group.HasDailyLimit() && !tt.want, tt.group.ShouldEnforceDailyLimit())
		})
	}
}

func TestUserSubscription_CheckLimitsWithDailyOverdraft(t *testing.T) {
	daily := 80.0
	weekly := 560.0
	strictGroup := &Group{SubscriptionType: SubscriptionTypeSubscription, DailyLimitUSD: &daily, WeeklyLimitUSD: &weekly}
	overdraftGroup := &Group{SubscriptionType: SubscriptionTypeSubscription, DailyLimitUSD: &daily, WeeklyLimitUSD: &weekly, AllowDailyOverdraft: true}
	now := time.Now()
	startsAt := now.Add(-time.Hour)

	sub := &UserSubscription{StartsAt: startsAt, ExpiresAt: startsAt.Add(5 * 24 * time.Hour), DailyUsageUSD: 80, WeeklyUsageUSD: 120}
	require.False(t, sub.CheckDailyLimit(strictGroup, 0))
	require.False(t, sub.CheckDailyLimit(overdraftGroup, 0))
	sub.AllowDailyOverdraft = true
	require.True(t, sub.CheckDailyLimit(overdraftGroup, 0))
	require.True(t, sub.CheckWeeklyLimit(overdraftGroup, 0))

	sub.WeeklyUsageUSD = 400
	require.False(t, sub.CheckDailyLimit(overdraftGroup, 0))
	require.True(t, sub.CheckWeeklyLimit(overdraftGroup, 0))
}
