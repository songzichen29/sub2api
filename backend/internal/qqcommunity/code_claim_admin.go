package qqcommunity

import (
	"context"
	"database/sql"
	"errors"
)

// Conditional status update serializes with redemption's status='unused' update.
// Keep the claim record even when disabled so a retry cannot allocate a new code.
func (s *Service) DisableCampaignCode(ctx context.Context, id string) error {
	c, err := s.Config(ctx)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var codeID int64
	err = tx.QueryRowContext(ctx, `SELECT redeem_code_id FROM qqc_code_claims WHERE id=$1 AND app_id=$2`, id, c.AppID).Scan(&codeID)
	if errors.Is(err, sql.ErrNoRows) {
		return CampaignRuleError("发放记录不存在")
	}
	if err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `UPDATE redeem_codes SET status='disabled' WHERE id=$1 AND status='unused'`, codeID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return CampaignRuleError("兑换码已使用、已停用或已删除")
	}
	return tx.Commit()
}
