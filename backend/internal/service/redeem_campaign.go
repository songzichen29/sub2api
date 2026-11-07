package service

import (
	"context"
	"fmt"

	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Campaign ownership is immutable and independent of editable code notes.
// Check in the consumption transaction; an unavailable ledger fails closed.
func validateCampaignRecipient(ctx context.Context, codeID, userID int64) error {
	tx := dbent.TxFromContext(ctx)
	if tx == nil {
		return fmt.Errorf("campaign redemption requires a transaction")
	}
	client := tx.Client()
	b := entsql.Dialect(client.Driver().Dialect())
	query, args := b.Select("user_id").From(b.Table("qqc_code_claims")).Where(entsql.EQ("redeem_code_id", codeID)).Query()
	rows, err := client.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("check campaign recipient: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		var owner int64
		if err := rows.Scan(&owner); err != nil {
			return err
		}
		if owner != userID {
			return infraerrors.Forbidden("REDEEM_CODE_RECIPIENT_MISMATCH", "此活动兑换码仅限领取时绑定的账号使用")
		}
	}
	return rows.Err()
}
