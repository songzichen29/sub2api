package qqcommunity

import (
	"context"
	"errors"
)

// 库存以已结算的普通中奖记录为准；配置保存、改名和排序不会重置。
func loadLotteryStock(ctx context.Context, db queryRower, prizes []LotteryPrize) error {
	for i := range prizes {
		prizes[i].Allocated = 0
		if prizes[i].ID == "" {
			continue
		}
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM qqc_lottery_draws WHERE prize_id=$1`, prizes[i].ID).Scan(&prizes[i].Allocated); err != nil {
			return err
		}
	}
	return nil
}

func prepareLotteryStock(ctx context.Context, tx *transaction, current LotteryConfig, next *LotteryConfig) error {
	known := map[string]bool{}
	for _, p := range current.Prizes {
		if p.ID != "" {
			known[p.ID] = true
		}
	}
	seen := map[string]bool{}
	for i := range next.Prizes {
		p := &next.Prizes[i]
		if p.ID == "" {
			p.ID = randomID()
		} else if !known[p.ID] || seen[p.ID] {
			return errors.New("奖项标识已失效或重复，请刷新配置")
		}
		seen[p.ID] = true
	}
	if err := loadLotteryStock(ctx, tx, next.Prizes); err != nil {
		return err
	}
	for i := range next.Prizes {
		p := &next.Prizes[i]
		if p.StockLimit != nil && *p.StockLimit < p.Allocated {
			return errors.New("奖项数量不能小于已发数量")
		}
		p.Allocated = 0
	}
	return nil
}

func availableLotteryPool(prizes []LotteryPrize) []LotteryPrize {
	pool := append([]LotteryPrize(nil), prizes...)
	for i := range pool {
		if pool[i].StockLimit != nil && pool[i].Allocated >= *pool[i].StockLimit {
			pool[i].Name = "谢谢参与"
			pool[i].Amount = 0
		}
	}
	return pool
}

func (c *LotteryConfig) reservePrize(prize LotteryPrize) {
	if prize.Amount <= 0 || prize.ID == "" {
		return
	}
	for i := range c.Prizes {
		if c.Prizes[i].ID == prize.ID {
			c.Prizes[i].Allocated++
			return
		}
	}
}
