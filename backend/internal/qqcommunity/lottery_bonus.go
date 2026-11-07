package qqcommunity

import "context"

type lotteryBonusBalance struct {
	remaining int64
	used      int64
}

func (b *lotteryBonusBalance) apply(delta int64) {
	if delta > 0 {
		b.remaining += delta
	} else if b.remaining > 0 {
		b.remaining--
		b.used++
	}
}

// Grants can pay only for draws at or after their issuance. Replaying this
// immutable ledger protects new grants from historical recharge deficits and
// consumes available grants first, identically for website and QQ draws.
// uid=0 loads all grant holders in one query for the administrator's summary.
func lotteryBonusUsage(ctx context.Context, db lotteryQuerier, uid int64) (map[int64]int64, error) {
	rows, err := db.QueryContext(ctx, `SELECT user_id,chance_delta FROM (
 SELECT user_id,chances AS chance_delta,created_at,0 AS event_order,source_key AS event_key
 FROM qqc_lottery_chance_grants WHERE created_at >= $1 AND ($2=0 OR user_id=$2)
 UNION ALL
 SELECT d.user_id,-1 AS chance_delta,d.created_at,1 AS event_order,d.request_key AS event_key
 FROM qqc_lottery_draws d WHERE d.created_at >= $1 AND ($2=0 OR d.user_id=$2)
 AND EXISTS(SELECT 1 FROM qqc_lottery_chance_grants g WHERE g.user_id=d.user_id AND g.created_at >= $1 AND g.created_at <= d.created_at)
) events ORDER BY created_at,event_order,event_key`, lotteryPeriodStart, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	balances := map[int64]lotteryBonusBalance{}
	for rows.Next() {
		var userID, delta int64
		if err := rows.Scan(&userID, &delta); err != nil {
			return nil, err
		}
		balance := balances[userID]
		balance.apply(delta)
		balances[userID] = balance
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	used := make(map[int64]int64, len(balances))
	for userID, balance := range balances {
		used[userID] = balance.used
	}
	return used, nil
}
