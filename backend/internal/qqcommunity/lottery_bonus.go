package qqcommunity

import (
	"context"
	"encoding/json"
	"errors"
)

type lotterySourceUsage struct {
	bonusUsed  int64
	legacyUsed int64
}

type lotteryBonusBalance struct {
	remaining int64
	used      int64
	legacy    int64
}

// New settlements persist their source, so a later grant can never relabel a
// recharge draw. Only unclassified historical draws use chronological replay.
func (b *lotteryBonusBalance) draw(raw []byte, legacyCreditOnly bool) error {
	var snapshot struct {
		ChanceSource     string  `json:"chance_source"`
		RechargeCurrency string  `json:"recharge_currency"`
		RechargeStep     float64 `json:"recharge_step"`
		ChancesPerStep   int64   `json:"chances_per_step"`
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return err
	}
	switch snapshot.ChanceSource {
	case "activity":
		// PostgreSQL NOW() is the transaction start, which may precede a
		// grant committed while the draw waited for the user lock. Trust the
		// settled source and validate its balance after replaying all events.
		b.remaining--
		b.used++
	case "recharge":
		// This draw never consumes an activity grant.
	case "":
		if b.remaining > 0 {
			b.apply(-1)
		} else if legacyCreditOnly && snapshot.RechargeCurrency == "" && snapshot.RechargeStep > 0 && snapshot.ChancesPerStep > 0 {
			// Before CNY accounting, balance codes counted as USD recharge.
			// With no completed online balance order before this draw, its
			// positive historical recharge total came from non-cash credit.
			// Preserve the draw without charging it to future CNY payments.
			b.legacy++
		}
	default:
		return errors.New("unknown lottery chance source")
	}
	return nil
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
// uid=0 loads all users in one query for the administrator's summary.
func lotteryUsageBySource(ctx context.Context, db lotteryQuerier, uid int64) (map[int64]lotterySourceUsage, error) {
	rows, err := db.QueryContext(ctx, `SELECT user_id,chance_delta,config_snapshot,legacy_credit_only FROM (
 SELECT user_id,chances AS chance_delta,NULL AS config_snapshot,false AS legacy_credit_only,created_at,0 AS event_order,source_key AS event_key
 FROM qqc_lottery_chance_grants WHERE created_at >= $1 AND ($2=0 OR user_id=$2)
 UNION ALL
 SELECT d.user_id,-1 AS chance_delta,d.config_snapshot,
 d.recharge_total>0 AND NOT EXISTS(SELECT 1 FROM payment_orders p WHERE p.user_id=d.user_id AND p.order_type='balance'
 AND p.paid_at IS NOT NULL AND p.completed_at >= $1 AND p.completed_at <= d.created_at) AS legacy_credit_only,
 d.created_at,1 AS event_order,d.request_key AS event_key
 FROM qqc_lottery_draws d WHERE d.created_at >= $1 AND ($2=0 OR d.user_id=$2)
) events ORDER BY created_at,event_order,event_key`, lotteryPeriodStart, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	balances := map[int64]lotteryBonusBalance{}
	for rows.Next() {
		var userID, delta int64
		var snapshot []byte
		var legacyCreditOnly bool
		if err := rows.Scan(&userID, &delta, &snapshot, &legacyCreditOnly); err != nil {
			return nil, err
		}
		balance := balances[userID]
		if delta > 0 {
			balance.apply(delta)
		} else if err := balance.draw(snapshot, legacyCreditOnly); err != nil {
			return nil, err
		}
		balances[userID] = balance
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	used := make(map[int64]lotterySourceUsage, len(balances))
	for userID, balance := range balances {
		if balance.remaining < 0 {
			return nil, errors.New("activity lottery chance ledger is inconsistent")
		}
		used[userID] = lotterySourceUsage{bonusUsed: balance.used, legacyUsed: balance.legacy}
	}
	return used, nil
}
