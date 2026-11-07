package qqcommunity

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/shopspring/decimal"
)

type lotteryQuerier interface {
	queryRower
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// Lottery recharge is actual CNY principal, not credited USD. Keep the shared
// rechargeSum unchanged: check-in and growth thresholds still use credit units.
func lotteryRechargeSum(ctx context.Context, db lotteryQuerier, uid int64, lockSources bool) (decimal.Decimal, error) {
	query := `SELECT pay_amount,fee_rate,amount,refund_amount,status,COALESCE(provider_key,''),payment_type,provider_snapshot
        FROM payment_orders WHERE user_id=$1 AND order_type='balance'
        AND paid_at IS NOT NULL AND completed_at IS NOT NULL AND completed_at >= $2 ORDER BY id`
	if lockSources {
		query += ` FOR SHARE`
	}
	rows, err := db.QueryContext(ctx, query, uid, lotteryPeriodStart)
	if err != nil {
		return decimal.Zero, err
	}
	defer rows.Close()
	total := decimal.Zero
	for rows.Next() {
		var paid, feeRate, credited, refunded decimal.Decimal
		var status, provider, method string
		var snapshot []byte
		if err := rows.Scan(&paid, &feeRate, &credited, &refunded, &status, &provider, &method, &snapshot); err != nil {
			return decimal.Zero, err
		}
		if lotteryPaymentIsCNY(provider, method, snapshot) {
			total = total.Add(lotteryOrderPrincipal(paid, feeRate, credited, refunded, status))
		}
	}
	return total, rows.Err()
}

func lotteryPaymentIsCNY(provider, method string, raw []byte) bool {
	var snapshot struct {
		Currency json.RawMessage `json:"currency"`
		Provider string          `json:"provider_key"`
	}
	if len(raw) > 0 {
		if json.Unmarshal(raw, &snapshot) != nil {
			return false
		}
		if len(snapshot.Currency) > 0 {
			var currency string
			return json.Unmarshal(snapshot.Currency, &currency) == nil && strings.EqualFold(strings.TrimSpace(currency), "CNY")
		}
	}
	// These providers only charge CNY and older snapshots omit the currency.
	// An unknown or foreign provider must never inherit the method's currency.
	provider = strings.TrimSpace(provider)
	if provider == "" {
		provider = strings.TrimSpace(snapshot.Provider)
	}
	if provider == "" {
		provider = strings.TrimSpace(method)
	}
	switch strings.ToLower(provider) {
	case "alipay", "wxpay", "easypay":
		return true
	default:
		return false
	}
}

func lotteryOrderPrincipal(paid, feeRate, credited, refunded decimal.Decimal, status string) decimal.Decimal {
	if status != "COMPLETED" && status != "REFUND_FAILED" && status != "PARTIALLY_REFUNDED" {
		return decimal.Zero
	}
	if !paid.IsPositive() || feeRate.IsNegative() || !paid.Equal(paid.Truncate(2)) {
		return decimal.Zero
	}
	// Payment adds a fee rounded UP to cents. Truncating the inverse to cents
	// recovers the paid principal exactly, including discounts/coupons. Never
	// divide credited USD by today's multiplier or use today's fee setting.
	rate := feeRate.Div(decimal.NewFromInt(100))
	principal, _ := paid.QuoRem(decimal.NewFromInt(1).Add(rate), 2)
	if !principal.Add(principal.Mul(rate).RoundUp(2)).Equal(paid) {
		return decimal.Zero
	}
	if status == "PARTIALLY_REFUNDED" {
		if !credited.IsPositive() || refunded.IsNegative() {
			return decimal.Zero
		}
		remaining := decimal.Max(credited.Sub(refunded), decimal.Zero)
		return principal.Mul(remaining).DivRound(credited, 8)
	}
	return principal
}
