package qqcommunity

import (
	"context"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

type CampaignPrize struct {
	ID                  string  `json:"id"`
	Name                string  `json:"name"`
	Type                string  `json:"type"`
	Value               float64 `json:"value"`
	SubscriptionGroupID *int64  `json:"subscription_group_id"`
	ValidityDays        int     `json:"validity_days"`
	ProbabilityBPS      int     `json:"probability_bps"`
	StockLimit          int64   `json:"stock_limit"`
	Allocated           int64   `json:"allocated"`
}

type CampaignTokenTier struct {
	Tokens   int64 `json:"tokens"`
	BonusBPS int   `json:"bonus_bps"`
}

func validateCampaignReward(kind string, value float64, group *int64, days int) error {
	switch kind {
	case "balance":
		if !validRewardAmount(value) || value <= 0 {
			return CampaignRuleError("每码余额需大于0且不超过1000 USD，最多8位小数")
		}
	case "lottery_chances", "concurrency":
		if math.IsNaN(value) || value < 1 || value > 1000 || math.Trunc(value) != value {
			return CampaignRuleError("每码次数或并发数必须是1～1000的整数")
		}
	case "subscription":
		if group == nil || *group <= 0 || days < 1 || days > 3650 {
			return CampaignRuleError("请选择订阅分组，并设置1～3650天")
		}
	default:
		return CampaignRuleError("不支持的兑换类型")
	}
	return nil
}

func validateCampaignPrizes(c CodeCampaign) error {
	if len(c.Prizes) < 1 || len(c.Prizes) > 20 || len(c.TokenTiers) > 20 {
		return CampaignRuleError("请配置1～20个主奖，Token加成最多20档")
	}
	total := 0
	for _, p := range c.Prizes {
		if p.ProbabilityBPS < 1 || p.ProbabilityBPS > 10000 {
			return CampaignRuleError("主奖概率需为0.01%～100%")
		}
		total += p.ProbabilityBPS
	}
	if total > 10000 {
		return CampaignRuleError("主奖基础概率合计不能超过100%")
	}
	prizes := append([]CampaignPrize(nil), c.Prizes...)
	if c.Compensation != nil {
		prizes = append(prizes, *c.Compensation)
	}
	if c.CompensationRequired && c.Compensation == nil {
		return CampaignRuleError("必有补偿需要配置补偿兑换码")
	}
	for _, p := range prizes {
		if strings.TrimSpace(p.Name) == "" || utf8.RuneCountInString(p.Name) > 60 || p.StockLimit < 0 || p.StockLimit > 100000 {
			return CampaignRuleError("奖项名称需为1～60个字符，数量需为0～100000")
		}
		if err := validateCampaignReward(p.Type, p.Value, p.SubscriptionGroupID, p.ValidityDays); err != nil {
			return err
		}
	}
	threshold, bonus := int64(0), 0
	for _, tier := range c.TokenTiers {
		if tier.Tokens <= threshold || tier.Tokens > 9007199254740991 || tier.BonusBPS <= bonus || total+tier.BonusBPS > 10000 {
			return CampaignRuleError("Token门槛与加成须递增，主奖基础概率加最高档加成不能超过100%")
		}
		threshold, bonus = tier.Tokens, tier.BonusBPS
	}
	return nil
}

func loadCampaignStock(ctx context.Context, db queryRower, c *CodeCampaign) error {
	for i := range c.Prizes {
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM qqc_code_claims WHERE campaign_id=$1 AND prize_id=$2`, c.ID, c.Prizes[i].ID).Scan(&c.Prizes[i].Allocated); err != nil {
			return err
		}
	}
	if c.Compensation != nil {
		return db.QueryRowContext(ctx, `SELECT COUNT(*) FROM qqc_code_claims WHERE campaign_id=$1 AND prize_id=$2`, c.ID, c.Compensation.ID).Scan(&c.Compensation.Allocated)
	}
	return nil
}

func (c CodeCampaign) probabilityMode() bool { return c.RewardMode == "probability" }

func (c CodeCampaign) campaignPrizes() []*CampaignPrize {
	out := make([]*CampaignPrize, 0, len(c.Prizes)+1)
	for i := range c.Prizes {
		out = append(out, &c.Prizes[i])
	}
	if c.Compensation != nil {
		out = append(out, c.Compensation)
	}
	return out
}

func prepareCampaignPrizes(ctx context.Context, tx *transaction, old CodeCampaign, c *CodeCampaign, issued int64) error {
	if !c.probabilityMode() {
		return nil
	}
	known := map[string]CampaignPrize{}
	for _, p := range old.campaignPrizes() {
		known[p.ID] = *p
	}
	seen := map[string]bool{}
	for _, p := range c.campaignPrizes() {
		if p.Type != "subscription" {
			p.SubscriptionGroupID, p.ValidityDays = nil, 0
		}
		if p.Type == "subscription" {
			p.Value = 1
		}
		if p.ID == "" {
			p.ID = randomID()
		} else if _, ok := known[p.ID]; !ok || seen[p.ID] {
			return CampaignRuleError("奖项标识已变化或重复，请刷新")
		}
		seen[p.ID] = true
		if prior, ok := known[p.ID]; ok && issued > 0 {
			oldGroup, newGroup := int64(0), int64(0)
			if prior.SubscriptionGroupID != nil {
				oldGroup = *prior.SubscriptionGroupID
			}
			if p.SubscriptionGroupID != nil {
				newGroup = *p.SubscriptionGroupID
			}
			if prior.Type != p.Type || prior.Value != p.Value || oldGroup != newGroup || prior.ValidityDays != p.ValidityDays {
				return CampaignRuleError("已有参与记录，奖项兑换内容不可变更；请新建活动")
			}
		}
		p.Allocated = 0
	}
	if issued > 0 {
		for id := range known {
			if !seen[id] {
				return CampaignRuleError("已有参与记录，不可删除奖项；可将总数量调整至已发数量")
			}
		}
		if (old.Compensation == nil) != (c.Compensation == nil) || old.CompensationRequired != c.CompensationRequired ||
			(old.Compensation != nil && old.Compensation.ID != c.Compensation.ID) {
			return CampaignRuleError("已有参与记录，补偿规则不可变更；请新建活动")
		}
	}
	if err := loadCampaignStock(ctx, tx, c); err != nil {
		return err
	}
	for _, p := range c.campaignPrizes() {
		if p.StockLimit < p.Allocated {
			return CampaignRuleError("奖项数量不能小于已分配数量")
		}
		p.Allocated = 0
		if p.Type == "subscription" {
			var valid bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM groups WHERE id=$1 AND subscription_type='subscription' AND status='active' AND deleted_at IS NULL)`, *p.SubscriptionGroupID).Scan(&valid); err != nil {
				return err
			}
			if !valid {
				return CampaignRuleError("订阅分组不存在或未启用")
			}
		}
	}
	return nil
}

// 用累计边界按比例放大，避免逐奖四舍五入改变总概率。
func campaignDistribution(c CodeCampaign, tokens int64) ([]int, int, int) {
	base, bonus := 0, 0
	for _, p := range c.Prizes {
		base += p.ProbabilityBPS
	}
	for _, tier := range c.TokenTiers {
		if tokens >= tier.Tokens {
			bonus = tier.BonusBPS
		}
	}
	weights := make([]int, len(c.Prizes))
	cumulative, previous, available := 0, 0, 0
	for i, p := range c.Prizes {
		cumulative += p.ProbabilityBPS
		boundary := cumulative * (base + bonus) / base
		weights[i] = boundary - previous
		previous = boundary
		if p.Allocated < p.StockLimit {
			available += weights[i]
		}
	}
	return weights, available, bonus
}

func campaignPrizeAt(c CodeCampaign, weights []int, roll int) (*CampaignPrize, string) {
	for i, p := range c.Prizes {
		roll -= weights[i]
		if roll < 0 {
			if p.Allocated < p.StockLimit {
				return &c.Prizes[i], "prize"
			}
			break
		}
	}
	if c.Compensation != nil && c.Compensation.Allocated < c.Compensation.StockLimit {
		return c.Compensation, "compensation"
	}
	return nil, "none"
}

func campaignUsageTokens(ctx context.Context, tx *transaction, uid int64, now time.Time) (int64, error) {
	local := now.In(checkinZone)
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, checkinZone)
	var total int64
	err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(input_tokens),0)+COALESCE(SUM(output_tokens),0)+COALESCE(SUM(cache_creation_tokens),0)+COALESCE(SUM(cache_read_tokens),0) FROM usage_logs WHERE user_id=$1 AND created_at >= $2 AND created_at <= $3`, uid, start, now).Scan(&total)
	return total, err
}
