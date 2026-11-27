package qqcommunity

import (
	"context"
	"time"
)

type LotteryTokenBonus struct {
	UsageTokens int64 `json:"usage_tokens"`
	BonusBPS    int   `json:"bonus_bps"`
}

func lotteryUsageTokens(ctx context.Context, tx *transaction, c LotteryConfig, uid int64) (int64, error) {
	if len(c.TokenTiers) == 0 {
		return 0, nil
	}
	return campaignUsageTokens(ctx, tx, uid, time.Now())
}

// 每个普通奖励独立加上百分点；先按完整奖池折算，再将耗尽奖项转为谢谢参与。
func lotteryPoolForTokens(c LotteryConfig, tokens int64) ([]LotteryPrize, *LotteryTokenBonus) {
	pool := append([]LotteryPrize(nil), c.Prizes...)
	if len(c.TokenTiers) == 0 {
		return availableLotteryPool(pool), nil
	}
	info := &LotteryTokenBonus{UsageTokens: tokens}
	bonus := 0
	for _, tier := range c.TokenTiers {
		if tokens >= tier.Tokens {
			bonus = tier.BonusBPS
		}
	}
	if bonus == 0 {
		return availableLotteryPool(pool), info
	}
	base := make([]int, 0, len(pool))
	missBase := 0
	for _, p := range pool {
		if p.Amount > 0 {
			base = append(base, p.ProbabilityBPS)
			continue
		}
		missBase += p.ProbabilityBPS
	}
	weights := tokenPrizeWeights(base, bonus)
	rewardTotal := 0
	for _, weight := range weights {
		rewardTotal += weight
	}
	rewardIndex, missTotal, missPrevious := 0, 0, 0
	for i, p := range pool {
		if p.Amount > 0 {
			pool[i].ProbabilityBPS = weights[rewardIndex]
			rewardIndex++
			if p.StockLimit == nil || p.Allocated < *p.StockLimit {
				info.BonusBPS = bonus
			}
			continue
		}
		missTotal += p.ProbabilityBPS
		boundary := missTotal * (10000 - rewardTotal) / missBase
		pool[i].ProbabilityBPS = boundary - missPrevious
		missPrevious = boundary
	}
	return availableLotteryPool(pool), info
}
