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

// 普通奖励按原比例增加概率，从未中奖份额扣除；耗尽奖项的份额仍落入谢谢参与。
func lotteryPoolForTokens(c LotteryConfig, tokens int64) ([]LotteryPrize, *LotteryTokenBonus) {
	pool := append([]LotteryPrize(nil), c.Prizes...)
	if len(c.TokenTiers) == 0 {
		return availableLotteryPool(pool), nil
	}
	info := &LotteryTokenBonus{UsageTokens: tokens}
	bonus, base := 0, 0
	for _, tier := range c.TokenTiers {
		if tokens >= tier.Tokens {
			bonus = tier.BonusBPS
		}
	}
	if bonus == 0 {
		return availableLotteryPool(pool), info
	}
	for _, p := range pool {
		if p.Amount > 0 {
			base += p.ProbabilityBPS
		}
	}
	winTotal, winPrevious, missTotal, missPrevious := 0, 0, 0, 0
	for i, p := range pool {
		if p.Amount > 0 {
			winTotal += p.ProbabilityBPS
			boundary := winTotal * (base + bonus) / base
			pool[i].ProbabilityBPS = boundary - winPrevious
			winPrevious = boundary
			if p.StockLimit == nil || p.Allocated < *p.StockLimit {
				info.BonusBPS += pool[i].ProbabilityBPS - p.ProbabilityBPS
			}
			continue
		}
		missTotal += p.ProbabilityBPS
		boundary := missTotal * (10000 - base - bonus) / (10000 - base)
		pool[i].ProbabilityBPS = boundary - missPrevious
		missPrevious = boundary
	}
	return availableLotteryPool(pool), info
}
