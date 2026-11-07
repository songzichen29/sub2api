package service

import (
	"context"
	"fmt"
	"math"

	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Keep this bound aligned with the existing lottery grant ledger's CHECK.
const maxLotteryRedeemChances = 1000

func validateLotteryRedeemValue(codeType string, value float64) error {
	if codeType != RedeemTypeLotteryChances {
		return nil
	}
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 1 || value > maxLotteryRedeemChances || value != math.Trunc(value) {
		return infraerrors.BadRequest("REDEEM_LOTTERY_CHANCES_INVALID", "lottery chances must be a whole number from 1 to 1000")
	}
	return nil
}

// Use the SAME transaction that consumes the code. A failed grant rolls back
// consumption, and the unique source key prevents issuing the same grant twice.
// This ledger survives code deletion and never changes balances or CNY recharge.
func grantRedeemLotteryChances(ctx context.Context, codeID, userID int64, chances int) error {
	tx := dbent.TxFromContext(ctx)
	if tx == nil {
		return fmt.Errorf("lottery redemption requires a transaction")
	}
	client := tx.Client()
	query, args := entsql.Dialect(client.Driver().Dialect()).Insert("qqc_lottery_chance_grants").
		Columns("source_key", "user_id", "chances").
		Values(fmt.Sprintf("redeem-code:%d", codeID), userID, chances).Query()
	if _, err := client.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("grant lottery chances: %w", err)
	}
	return nil
}
