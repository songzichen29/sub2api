package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/qqbot"
)

type qqBotHost struct {
	keys interface {
		GetAvailableGroups(context.Context, int64) ([]Group, error)
		InvalidateAuthCacheByUserID(context.Context, int64)
	}
	billing interface {
		InvalidateUserBalance(context.Context, int64) error
	}
	monitor interface {
		ParseFilter(string, []string, []string, []int64) (ChannelMonitorV2Filter, error)
		Matrix(context.Context, ChannelMonitorV2Filter, ChannelMonitorV2GroupBy, bool) (*ChannelMonitorV2Matrix, error)
	}
	settings interface {
		GetChannelMonitorRuntime(context.Context) ChannelMonitorRuntime
	}
}

func (h *qqBotHost) InvalidateBalance(ctx context.Context, id int64) {
	_ = h.billing.InvalidateUserBalance(ctx, id)
	h.keys.InvalidateAuthCacheByUserID(ctx, id)
}
func (h *qqBotHost) ChannelStatus(ctx context.Context, id int64) (string, error) {
	runtime := h.settings.GetChannelMonitorRuntime(ctx)
	if !runtime.PassiveAggregationAllowed() {
		return "渠道监控尚未开启。", nil
	}
	groups, err := h.keys.GetAvailableGroups(ctx, id)
	if err != nil {
		return "", err
	}
	if len(groups) == 0 {
		return "当前账号没有可用分组。", nil
	}
	filter, err := h.monitor.ParseFilter("1h", nil, nil, nil)
	if err != nil {
		return "", err
	}
	filter.RestrictGroups = true
	filter.AllowedGroupIDs = make([]int64, 0, len(groups))
	for _, g := range groups {
		filter.AllowedGroupIDs = append(filter.AllowedGroupIDs, g.ID)
	}
	matrix, err := h.monitor.Matrix(ctx, filter, ChannelMonitorV2GroupByPlatformGroup, false)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	out.WriteString("你可用分组近 1 小时的渠道状态：")
	n := 0
	for _, row := range matrix.Items {
		if n >= 20 {
			out.WriteString("\n更多状态请登录网站查看。")
			break
		}
		if row.Health.Overall == "unknown" {
			fmt.Fprintf(&out, "\n%s / %s：样本不足，请稍后再查", row.GroupName, row.Platform)
			n++
			continue
		}
		delay := "暂无"
		if row.Metrics.TTFT.P50Ms != nil {
			delay = fmt.Sprintf("%dms", *row.Metrics.TTFT.P50Ms)
		}
		fmt.Fprintf(&out, "\n%s / %s：成功率 %.1f%%，首字延迟 %s", row.GroupName, row.Platform, row.Metrics.SuccessRate*100, delay)
		n++
	}
	if n == 0 {
		return "你可用的分组近 1 小时暂无监控数据。", nil
	}
	return out.String(), nil
}
func ProvideQQBotService(repo qqbot.Repository, encryptor SecretEncryptor, keys *APIKeyService, billing *BillingCacheService, monitor *ChannelMonitorV2Service, settings *SettingService, cfg *config.Config, db *sql.DB, email *EmailService, users UserRepository, groups GroupRepository, channels ChannelRepository, channelService *ChannelService, priceResolver *ModelPricingResolver, billingService *BillingService, rates UserGroupRateRepository) (*qqbot.Service, error) {
	host := &qqBotHost{keys: keys, billing: billing, monitor: monitor, settings: settings}
	s := qqbot.NewService(repo, encryptor, host, fmt.Sprintf("http://127.0.0.1:%d", cfg.Server.Port))
	community, err := provideQQCommunityService(db, encryptor, cfg, keys, billing, email, settings, users, monitor, groups, channels, channelService, priceResolver, billingService, rates)
	if err != nil {
		return nil, err
	}
	s.Community = community
	return s, nil
}
