package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type usageBillingRepository struct {
	db *sql.DB
}

func NewUsageBillingRepository(_ *dbent.Client, sqlDB *sql.DB) service.UsageBillingRepository {
	return &usageBillingRepository{db: sqlDB}
}

func (r *usageBillingRepository) Apply(ctx context.Context, cmd *service.UsageBillingCommand) (_ *service.UsageBillingApplyResult, err error) {
	if cmd == nil {
		return &service.UsageBillingApplyResult{}, nil
	}
	if r == nil || r.db == nil {
		return nil, errors.New("usage billing repository db is nil")
	}

	cmd.Normalize()
	if cmd.RequestID == "" {
		return nil, service.ErrUsageBillingRequestIDRequired
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	applied, err := r.claimUsageBillingKey(ctx, tx, cmd)
	if err != nil {
		return nil, err
	}
	if !applied {
		return &service.UsageBillingApplyResult{Applied: false}, nil
	}

	result := &service.UsageBillingApplyResult{Applied: true}
	if err := r.applyUsageBillingEffects(ctx, tx, cmd, result); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	return result, nil
}

func (r *usageBillingRepository) claimUsageBillingKey(ctx context.Context, tx *sql.Tx, cmd *service.UsageBillingCommand) (bool, error) {
	var archivedFingerprint string
	err := tx.QueryRowContext(ctx, `
		SELECT request_fingerprint
		FROM usage_billing_dedup_archive
		WHERE request_id = ? AND api_key_id = ?
	`, cmd.RequestID, cmd.APIKeyID).Scan(&archivedFingerprint)
	if err == nil {
		if strings.TrimSpace(archivedFingerprint) != strings.TrimSpace(cmd.RequestFingerprint) {
			return false, service.ErrUsageBillingRequestConflict
		}
		return false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}

	res, err := tx.ExecContext(ctx, `
		INSERT IGNORE INTO usage_billing_dedup (request_id, api_key_id, request_fingerprint)
		VALUES (?, ?, ?)
	`, cmd.RequestID, cmd.APIKeyID, cmd.RequestFingerprint)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected > 0 {
		return true, nil
	}

	var existingFingerprint string
	if err := tx.QueryRowContext(ctx, `
		SELECT request_fingerprint
		FROM usage_billing_dedup
		WHERE request_id = ? AND api_key_id = ?
	`, cmd.RequestID, cmd.APIKeyID).Scan(&existingFingerprint); err != nil {
		return false, err
	}
	if strings.TrimSpace(existingFingerprint) != strings.TrimSpace(cmd.RequestFingerprint) {
		return false, service.ErrUsageBillingRequestConflict
	}
	return false, nil
}

func (r *usageBillingRepository) claimUsageBillingRequest(ctx context.Context, tx *sql.Tx, requestID string, apiKeyID int64, requestFingerprint string) (bool, error) {
	return r.claimUsageBillingKey(ctx, tx, &service.UsageBillingCommand{
		RequestID:          requestID,
		APIKeyID:           apiKeyID,
		RequestFingerprint: requestFingerprint,
	})
}

func (r *usageBillingRepository) ReserveBatchImageBalance(ctx context.Context, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	return r.applyBatchImageBalanceHold(ctx, cmd, reserveUsageBillingBatchImageBalance)
}

func (r *usageBillingRepository) CaptureBatchImageBalance(ctx context.Context, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	return r.applyBatchImageBalanceHold(ctx, cmd, captureUsageBillingBatchImageBalance)
}

func (r *usageBillingRepository) ReleaseBatchImageBalance(ctx context.Context, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	return r.applyBatchImageBalanceHold(ctx, cmd, releaseUsageBillingBatchImageBalance)
}

func (r *usageBillingRepository) applyBatchImageBalanceHold(
	ctx context.Context,
	cmd *service.BatchImageBalanceHoldCommand,
	apply func(context.Context, *sql.Tx, *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error),
) (_ *service.BatchImageBalanceHoldResult, err error) {
	if cmd == nil {
		return &service.BatchImageBalanceHoldResult{}, nil
	}
	if r == nil || r.db == nil {
		return nil, errors.New("usage billing repository db is nil")
	}
	cmd.Normalize()
	if cmd.RequestID == "" {
		return nil, service.ErrUsageBillingRequestIDRequired
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	applied, err := r.claimUsageBillingRequest(ctx, tx, cmd.RequestID, cmd.APIKeyID, cmd.RequestFingerprint)
	if err != nil {
		return nil, err
	}
	if !applied {
		return &service.BatchImageBalanceHoldResult{Applied: false}, nil
	}

	result, err := apply(ctx, tx, cmd)
	if err != nil {
		return nil, err
	}
	if result == nil {
		result = &service.BatchImageBalanceHoldResult{}
	}
	result.Applied = true

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	return result, nil
}

func (r *usageBillingRepository) applyUsageBillingEffects(ctx context.Context, tx *sql.Tx, cmd *service.UsageBillingCommand, result *service.UsageBillingApplyResult) error {
	if cmd.SubscriptionCost > 0 && cmd.SubscriptionID != nil {
		if err := incrementUsageBillingSubscription(ctx, tx, *cmd.SubscriptionID, cmd.SubscriptionCost, cmd.AllowSubscriptionOverLimit); err != nil {
			return err
		}
	}

	if cmd.BalanceCost > 0 {
		newBalance, sufficient, err := deductUsageBillingBalance(ctx, tx, cmd.UserID, cmd.BalanceCost)
		if err != nil {
			return err
		}
		result.NewBalance = &newBalance
		result.BalanceOverdrafted = !sufficient
	}

	if cmd.APIKeyQuotaCost > 0 {
		exhausted, err := incrementUsageBillingAPIKeyQuota(ctx, tx, cmd.APIKeyID, cmd.APIKeyQuotaCost)
		if err != nil {
			return err
		}
		result.APIKeyQuotaExhausted = exhausted
	}

	if cmd.APIKeyRateLimitCost > 0 {
		if err := incrementUsageBillingAPIKeyRateLimit(ctx, tx, cmd.APIKeyID, cmd.APIKeyRateLimitCost); err != nil {
			return err
		}
	}

	if cmd.AccountQuotaCost > 0 && (strings.EqualFold(cmd.AccountType, service.AccountTypeAPIKey) || strings.EqualFold(cmd.AccountType, service.AccountTypeBedrock)) {
		quotaState, err := incrementUsageBillingAccountQuota(ctx, tx, cmd.AccountID, cmd.AccountQuotaCost)
		if err != nil {
			return err
		}
		result.QuotaState = quotaState
	}

	return nil
}

func incrementUsageBillingSubscription(ctx context.Context, tx *sql.Tx, subscriptionID int64, costUSD float64, allowOverLimit bool) error {
	now := time.Now()
	const baseUpdateSQL = `
		UPDATE user_subscriptions us
		JOIN ` + quotedGroupsTable + ` g ON us.group_id = g.id AND g.deleted_at IS NULL
		CROSS JOIN (SELECT ? AS now_ts) clock
		SET
			us.daily_usage_usd = us.daily_usage_usd + ?,
			us.weekly_usage_usd = us.weekly_usage_usd + ?,
			us.monthly_usage_usd = us.monthly_usage_usd + ?,
			us.quota_used_usd = us.quota_used_usd + ?,
			us.status = CASE
				WHEN COALESCE(us.quota_limit_usd, 0) > 0 AND us.quota_used_usd + ? >= us.quota_limit_usd THEN ?
				WHEN g.allow_daily_overdraft = TRUE
					AND us.allow_daily_overdraft = TRUE
					AND COALESCE(g.daily_limit_usd, 0) > 0
					AND (
						CASE
							WHEN COALESCE(us.validity_unit, 'day') IN ('day', 'days') THEN GREATEST(
								us.weekly_usage_usd + ?,
								g.daily_limit_usd * LEAST(
									GREATEST(0, FLOOR(TIMESTAMPDIFF(SECOND, us.starts_at, NOW()) / 86400)),
									GREATEST(1, CEIL(TIMESTAMPDIFF(SECOND, us.starts_at, us.expires_at) / 86400))
								) + CASE
									WHEN us.daily_window_start IS NOT NULL
										AND us.daily_window_start = TIMESTAMPADD(
											SECOND,
											FLOOR(TIMESTAMPDIFF(SECOND, us.starts_at, NOW()) / 86400) * 86400,
											us.starts_at
										)
										THEN us.daily_usage_usd + ?
									ELSE ?
								END
							)
							ELSE us.weekly_usage_usd + ?
						END
					) >= g.daily_limit_usd * GREATEST(1, CEIL(TIMESTAMPDIFF(SECOND, us.starts_at, us.expires_at) / 86400))
					THEN ?
				WHEN COALESCE(g.weekly_limit_usd, 0) > 0 AND us.weekly_usage_usd + ? >= g.weekly_limit_usd THEN ?
				WHEN COALESCE(g.monthly_limit_usd, 0) > 0 AND us.monthly_usage_usd + ? >= g.monthly_limit_usd THEN ?
				ELSE us.status
			END,
			us.updated_at = NOW()
		WHERE us.id = ?
			AND us.deleted_at IS NULL
			AND (
				COALESCE(us.quota_limit_usd, 0) <= 0
				OR us.quota_used_usd + ? <= us.quota_limit_usd
				OR (? AND us.quota_used_usd < us.quota_limit_usd)
			)
			AND (
				COALESCE(g.daily_limit_usd, 0) <= 0
				OR (
					g.allow_daily_overdraft = TRUE
					AND us.allow_daily_overdraft = TRUE
					AND (
						CASE
							WHEN COALESCE(us.validity_unit, 'day') IN ('day', 'days') THEN GREATEST(
								us.weekly_usage_usd + ?,
								g.daily_limit_usd * LEAST(
									GREATEST(0, FLOOR(TIMESTAMPDIFF(SECOND, us.starts_at, NOW()) / 86400)),
									GREATEST(1, CEIL(TIMESTAMPDIFF(SECOND, us.starts_at, us.expires_at) / 86400))
								) + CASE
									WHEN us.daily_window_start IS NOT NULL
										AND us.daily_window_start = TIMESTAMPADD(
											SECOND,
											FLOOR(TIMESTAMPDIFF(SECOND, us.starts_at, NOW()) / 86400) * 86400,
											us.starts_at
										)
										THEN us.daily_usage_usd + ?
									ELSE ?
								END
							)
							ELSE us.weekly_usage_usd + ?
						END
					) <= g.daily_limit_usd * GREATEST(1, CEIL(TIMESTAMPDIFF(SECOND, us.starts_at, us.expires_at) / 86400))
				)
				OR (
					? AND g.allow_daily_overdraft = TRUE
					AND us.allow_daily_overdraft = TRUE
					AND (
						CASE
							WHEN COALESCE(us.validity_unit, 'day') IN ('day', 'days') THEN GREATEST(
								us.weekly_usage_usd,
								g.daily_limit_usd * LEAST(
									GREATEST(0, FLOOR(TIMESTAMPDIFF(SECOND, us.starts_at, NOW()) / 86400)),
									GREATEST(1, CEIL(TIMESTAMPDIFF(SECOND, us.starts_at, us.expires_at) / 86400))
								) + CASE
									WHEN us.daily_window_start IS NOT NULL
										AND us.daily_window_start = TIMESTAMPADD(
											SECOND,
											FLOOR(TIMESTAMPDIFF(SECOND, us.starts_at, NOW()) / 86400) * 86400,
											us.starts_at
										)
										THEN us.daily_usage_usd
									ELSE 0
								END
							)
							ELSE us.weekly_usage_usd
						END
					) < g.daily_limit_usd * GREATEST(1, CEIL(TIMESTAMPDIFF(SECOND, us.starts_at, us.expires_at) / 86400))
				)
				OR (
					NOT (g.allow_daily_overdraft = TRUE AND COALESCE(us.validity_unit, 'day') IN ('day', 'days'))
					AND us.daily_usage_usd + ? <= g.daily_limit_usd
				)
				OR (
					? AND NOT (g.allow_daily_overdraft = TRUE AND COALESCE(us.validity_unit, 'day') IN ('day', 'days'))
					AND us.daily_usage_usd < g.daily_limit_usd
				)
				OR (
					g.allow_daily_overdraft = TRUE
					AND us.allow_daily_overdraft = FALSE
					AND COALESCE(us.validity_unit, 'day') IN ('day', 'days')
					AND us.daily_usage_usd + GREATEST(
						0,
						us.weekly_usage_usd - CASE
							WHEN us.daily_window_start IS NOT NULL
								AND us.daily_window_start = TIMESTAMPADD(
									SECOND,
									FLOOR(TIMESTAMPDIFF(SECOND, us.starts_at, NOW()) / 86400) * 86400,
									us.starts_at
								)
								THEN us.daily_usage_usd
							ELSE 0
						END - g.daily_limit_usd * LEAST(
							GREATEST(0, FLOOR(TIMESTAMPDIFF(SECOND, us.starts_at, NOW()) / 86400)),
							GREATEST(1, CEIL(TIMESTAMPDIFF(SECOND, us.starts_at, us.expires_at) / 86400))
						)
					) + ? <= g.daily_limit_usd
				)
				OR (
					? AND g.allow_daily_overdraft = TRUE
					AND us.allow_daily_overdraft = FALSE
					AND COALESCE(us.validity_unit, 'day') IN ('day', 'days')
					AND us.daily_usage_usd + GREATEST(
						0,
						us.weekly_usage_usd - CASE
							WHEN us.daily_window_start IS NOT NULL
								AND us.daily_window_start = TIMESTAMPADD(
									SECOND,
									FLOOR(TIMESTAMPDIFF(SECOND, us.starts_at, NOW()) / 86400) * 86400,
									us.starts_at
								)
								THEN us.daily_usage_usd
							ELSE 0
						END - g.daily_limit_usd * LEAST(
							GREATEST(0, FLOOR(TIMESTAMPDIFF(SECOND, us.starts_at, NOW()) / 86400)),
							GREATEST(1, CEIL(TIMESTAMPDIFF(SECOND, us.starts_at, us.expires_at) / 86400))
						)
					) < g.daily_limit_usd
				)
			)
			AND (
				COALESCE(g.weekly_limit_usd, 0) <= 0
				OR (g.allow_daily_overdraft = TRUE AND us.allow_daily_overdraft = TRUE)
				OR us.weekly_usage_usd + ? <= g.weekly_limit_usd
				OR (? AND us.weekly_usage_usd < g.weekly_limit_usd)
			)
			AND (
				COALESCE(g.monthly_limit_usd, 0) <= 0
				OR (g.allow_daily_overdraft = TRUE AND us.allow_daily_overdraft = TRUE)
				OR us.monthly_usage_usd + ? <= g.monthly_limit_usd
				OR (? AND us.monthly_usage_usd < g.monthly_limit_usd)
			)
	`
	updateSQL := replaceSQLNowWithClock(baseUpdateSQL)
	res, err := tx.ExecContext(ctx, updateSQL,
		now, costUSD, costUSD, costUSD, costUSD,
		costUSD, service.SubscriptionStatusQuotaExhausted,
		costUSD, costUSD, costUSD, costUSD, service.SubscriptionStatusQuotaExhausted,
		costUSD, service.SubscriptionStatusQuotaExhausted,
		costUSD, service.SubscriptionStatusQuotaExhausted,
		subscriptionID,
		costUSD, allowOverLimit,
		costUSD, costUSD, costUSD, costUSD,
		allowOverLimit,
		costUSD, allowOverLimit, costUSD, allowOverLimit,
		costUSD, allowOverLimit,
		costUSD, allowOverLimit,
	)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected > 0 {
		return nil
	}

	exists, limitErr := subscriptionBillingLimitExceeded(ctx, tx, subscriptionID)
	if limitErr != nil {
		return limitErr
	}
	if exists {
		return service.ErrUsageBillingSubscriptionLimitExceeded
	}
	return service.ErrSubscriptionNotFound
}

func subscriptionBillingLimitExceeded(ctx context.Context, tx *sql.Tx, subscriptionID int64) (bool, error) {
	var matched int
	err := tx.QueryRowContext(ctx, `
		SELECT 1
		FROM user_subscriptions us
		JOIN `+quotedGroupsTable+` g ON us.group_id = g.id AND g.deleted_at IS NULL
		WHERE us.id = ? AND us.deleted_at IS NULL
		LIMIT 1
	`, subscriptionID).Scan(&matched)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func deductUsageBillingBalance(ctx context.Context, tx *sql.Tx, userID int64, amount float64) (float64, bool, error) {
	res, err := tx.ExecContext(ctx, `
		UPDATE users
		SET balance = balance - ?, updated_at = NOW()
		WHERE id = ? AND deleted_at IS NULL AND balance >= ?
	`, amount, userID, amount)
	if err != nil {
		return 0, false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, false, err
	}
	if affected > 0 {
		var newBalance float64
		if err := tx.QueryRowContext(ctx, `SELECT balance FROM users WHERE id = ?`, userID).Scan(&newBalance); err != nil {
			return 0, false, err
		}
		return newBalance, true, nil
	}

	res, err = tx.ExecContext(ctx, `
		UPDATE users
		SET balance = balance - ?, updated_at = NOW()
		WHERE id = ? AND deleted_at IS NULL
	`, amount, userID)
	if err != nil {
		return 0, false, err
	}
	affected, err = res.RowsAffected()
	if err != nil {
		return 0, false, err
	}
	if affected == 0 {
		return 0, false, service.ErrUserNotFound
	}
	var newBalance float64
	if err := tx.QueryRowContext(ctx, `SELECT balance FROM users WHERE id = ?`, userID).Scan(&newBalance); err != nil {
		return 0, false, err
	}
	return newBalance, false, nil
}

func reserveUsageBillingBatchImageBalance(ctx context.Context, tx *sql.Tx, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	if cmd.HoldAmount <= 0 {
		return &service.BatchImageBalanceHoldResult{}, nil
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE users
		SET balance = balance - ?,
			frozen_balance = COALESCE(frozen_balance, 0) + ?,
			updated_at = NOW()
		WHERE id = ? AND deleted_at IS NULL AND balance >= ?
	`, cmd.HoldAmount, cmd.HoldAmount, cmd.UserID, cmd.HoldAmount)
	if err != nil {
		return nil, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected > 0 {
		return queryUsageBillingBalanceHoldResult(ctx, tx, cmd.UserID)
	}
	if exists, existsErr := userExistsForBilling(ctx, tx, cmd.UserID); existsErr != nil {
		return nil, existsErr
	} else if !exists {
		return nil, service.ErrUserNotFound
	}
	return nil, service.ErrBatchImageInsufficientBalance
}

func captureUsageBillingBatchImageBalance(ctx context.Context, tx *sql.Tx, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	if cmd.HoldAmount <= 0 && cmd.ActualAmount <= 0 {
		return &service.BatchImageBalanceHoldResult{}, nil
	}
	if cmd.ActualAmount-cmd.HoldAmount > 0.00000001 {
		return nil, service.ErrBatchImageSettlementCostExceedsHold
	}
	refundAmount := cmd.HoldAmount - cmd.ActualAmount
	if refundAmount < 0 {
		refundAmount = 0
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE users
		SET balance = balance + ?,
			frozen_balance = COALESCE(frozen_balance, 0) - ?,
			updated_at = NOW()
		WHERE id = ? AND deleted_at IS NULL AND COALESCE(frozen_balance, 0) >= ?
	`, refundAmount, cmd.HoldAmount, cmd.UserID, cmd.HoldAmount)
	if err != nil {
		return nil, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected > 0 {
		return queryUsageBillingBalanceHoldResult(ctx, tx, cmd.UserID)
	}
	if exists, existsErr := userExistsForBilling(ctx, tx, cmd.UserID); existsErr != nil {
		return nil, existsErr
	} else if !exists {
		return nil, service.ErrUserNotFound
	}
	return nil, errors.New("batch image frozen balance is insufficient")
}

func releaseUsageBillingBatchImageBalance(ctx context.Context, tx *sql.Tx, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	if cmd.HoldAmount <= 0 {
		return &service.BatchImageBalanceHoldResult{}, nil
	}
	// 释放前校验该 job 确实预留过 hold（hold request id 已被 claim），
	// 防止从未成功冻结的 job 触发"幻影释放"，从其他用户的冻结资金池中凭空生成余额。
	held, heldErr := batchImageHoldClaimExists(ctx, tx, service.BatchImageHoldRequestID(cmd.BatchID), cmd.APIKeyID)
	if heldErr != nil {
		return nil, heldErr
	}
	if !held {
		logger.LegacyPrintf("repository.usage_billing", "[BatchImage] release skipped, hold was never reserved: batch=%s", cmd.BatchID)
		return &service.BatchImageBalanceHoldResult{}, nil
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE users
		SET balance = balance + ?,
			frozen_balance = COALESCE(frozen_balance, 0) - ?,
			updated_at = NOW()
		WHERE id = ? AND deleted_at IS NULL AND COALESCE(frozen_balance, 0) >= ?
	`, cmd.HoldAmount, cmd.HoldAmount, cmd.UserID, cmd.HoldAmount)
	if err != nil {
		return nil, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected > 0 {
		return queryUsageBillingBalanceHoldResult(ctx, tx, cmd.UserID)
	}
	if exists, existsErr := userExistsForBilling(ctx, tx, cmd.UserID); existsErr != nil {
		return nil, existsErr
	} else if !exists {
		return nil, service.ErrUserNotFound
	}
	return nil, errors.New("batch image frozen balance is insufficient")
}

func queryUsageBillingBalanceHoldResult(ctx context.Context, tx *sql.Tx, userID int64) (*service.BatchImageBalanceHoldResult, error) {
	var balance, frozen float64
	if err := tx.QueryRowContext(ctx, `
		SELECT balance, COALESCE(frozen_balance, 0)
		FROM users
		WHERE id = ?
	`, userID).Scan(&balance, &frozen); err != nil {
		return nil, err
	}
	return &service.BatchImageBalanceHoldResult{NewBalance: &balance, FrozenBalance: &frozen}, nil
}

// batchImageHoldClaimExists 检查 hold request id 是否已在 dedup（或归档）表中被 claim，
// 即该 batch 的冻结操作确实成功提交过。
func batchImageHoldClaimExists(ctx context.Context, tx *sql.Tx, holdRequestID string, apiKeyID int64) (bool, error) {
	var exists int
	err := tx.QueryRowContext(ctx, `
		SELECT 1
		FROM usage_billing_dedup
		WHERE request_id = ? AND api_key_id = ?
	`, holdRequestID, apiKeyID).Scan(&exists)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	err = tx.QueryRowContext(ctx, `
		SELECT 1
		FROM usage_billing_dedup_archive
		WHERE request_id = ? AND api_key_id = ?
	`, holdRequestID, apiKeyID).Scan(&exists)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, err
}

func userExistsForBilling(ctx context.Context, tx *sql.Tx, userID int64) (bool, error) {
	var exists int
	err := tx.QueryRowContext(ctx, `
		SELECT 1
		FROM users
		WHERE id = ? AND deleted_at IS NULL
	`, userID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func incrementUsageBillingAPIKeyQuota(ctx context.Context, tx *sql.Tx, apiKeyID int64, amount float64) (bool, error) {
	var quota float64
	var quotaUsed float64
	if err := tx.QueryRowContext(ctx, `
		SELECT quota, quota_used
		FROM api_keys
		WHERE id = ? AND deleted_at IS NULL
		FOR UPDATE
	`, apiKeyID).Scan(&quota, &quotaUsed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, service.ErrAPIKeyNotFound
		}
		return false, err
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE api_keys
		SET quota_used = quota_used + ?,
			status = CASE
				WHEN quota > 0 AND status = ? AND quota_used < quota AND quota_used + ? >= quota THEN ?
				ELSE status
			END,
			updated_at = NOW()
		WHERE id = ? AND deleted_at IS NULL
	`, amount, service.StatusAPIKeyActive, amount, service.StatusAPIKeyQuotaExhausted, apiKeyID)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected == 0 {
		return false, service.ErrAPIKeyNotFound
	}
	return quota > 0 && quotaUsed < quota && quotaUsed+amount >= quota, nil
}

func incrementUsageBillingAPIKeyRateLimit(ctx context.Context, tx *sql.Tx, apiKeyID int64, cost float64) error {
	res, err := tx.ExecContext(ctx, `
		UPDATE api_keys SET
			usage_5h = CASE WHEN window_5h_start IS NOT NULL AND DATE_ADD(window_5h_start, INTERVAL 5 HOUR) <= NOW() THEN ? ELSE usage_5h + ? END,
			usage_1d = CASE WHEN window_1d_start IS NOT NULL AND DATE_ADD(window_1d_start, INTERVAL 24 HOUR) <= NOW() THEN ? ELSE usage_1d + ? END,
			usage_7d = CASE WHEN window_7d_start IS NOT NULL AND DATE_ADD(window_7d_start, INTERVAL 7 DAY) <= NOW() THEN ? ELSE usage_7d + ? END,
			window_5h_start = CASE WHEN window_5h_start IS NULL OR DATE_ADD(window_5h_start, INTERVAL 5 HOUR) <= NOW() THEN NOW() ELSE window_5h_start END,
			window_1d_start = CASE WHEN window_1d_start IS NULL OR DATE_ADD(window_1d_start, INTERVAL 24 HOUR) <= NOW() THEN DATE(NOW()) ELSE window_1d_start END,
			window_7d_start = CASE WHEN window_7d_start IS NULL OR DATE_ADD(window_7d_start, INTERVAL 7 DAY) <= NOW() THEN DATE(NOW()) ELSE window_7d_start END,
			updated_at = NOW()
		WHERE id = ? AND deleted_at IS NULL
	`, cost, cost, cost, cost, cost, cost, apiKeyID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrAPIKeyNotFound
	}
	return nil
}

func incrementUsageBillingAccountQuota(ctx context.Context, tx *sql.Tx, accountID int64, amount float64) (*service.AccountQuotaState, error) {
	var extraRaw sql.NullString
	if err := tx.QueryRowContext(ctx, `
		SELECT extra FROM accounts
		WHERE id = ? AND deleted_at IS NULL
		FOR UPDATE
	`, accountID).Scan(&extraRaw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, service.ErrAccountNotFound
		}
		return nil, err
	}

	extra := map[string]any{}
	if extraRaw.Valid && strings.TrimSpace(extraRaw.String) != "" {
		if err := json.Unmarshal([]byte(extraRaw.String), &extra); err != nil {
			return nil, fmt.Errorf("parse account extra: %w", err)
		}
	}

	now := time.Now().UTC()
	state := &service.AccountQuotaState{
		TotalUsed:   toFloat(extra["quota_used"]),
		TotalLimit:  toFloat(extra["quota_limit"]),
		DailyUsed:   toFloat(extra["quota_daily_used"]),
		DailyLimit:  toFloat(extra["quota_daily_limit"]),
		WeeklyUsed:  toFloat(extra["quota_weekly_used"]),
		WeeklyLimit: toFloat(extra["quota_weekly_limit"]),
	}

	state.TotalUsed += amount
	if state.DailyLimit > 0 {
		dailyStart := parseExtraTime(extra["quota_daily_start"])
		dailyExpired := dailyStart.IsZero() || !dailyStart.Add(24*time.Hour).After(now)
		if dailyExpired {
			state.DailyUsed = amount
			extra["quota_daily_start"] = now.Format(time.RFC3339Nano)
			extra["quota_daily_reset_at"] = now.Truncate(24 * time.Hour).Add(24 * time.Hour).Format(time.RFC3339Nano)
		} else {
			state.DailyUsed += amount
		}
	}
	if state.WeeklyLimit > 0 {
		weeklyStart := parseExtraTime(extra["quota_weekly_start"])
		weeklyExpired := weeklyStart.IsZero() || !weeklyStart.Add(7*24*time.Hour).After(now)
		if weeklyExpired {
			state.WeeklyUsed = amount
			extra["quota_weekly_start"] = now.Format(time.RFC3339Nano)
			extra["quota_weekly_reset_at"] = now.Truncate(24 * time.Hour).Add(7 * 24 * time.Hour).Format(time.RFC3339Nano)
		} else {
			state.WeeklyUsed += amount
		}
	}

	extra["quota_used"] = round6(state.TotalUsed)
	extra["quota_limit"] = round6(state.TotalLimit)
	extra["quota_daily_used"] = round6(state.DailyUsed)
	extra["quota_daily_limit"] = round6(state.DailyLimit)
	extra["quota_weekly_used"] = round6(state.WeeklyUsed)
	extra["quota_weekly_limit"] = round6(state.WeeklyLimit)

	extraJSON, err := json.Marshal(extra)
	if err != nil {
		return nil, err
	}
	res, err := tx.ExecContext(ctx, `UPDATE accounts SET extra = ?, updated_at = NOW() WHERE id = ? AND deleted_at IS NULL`, string(extraJSON), accountID)
	if err != nil {
		return nil, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, service.ErrAccountNotFound
	}

	crossedTotal := state.TotalLimit > 0 && state.TotalUsed >= state.TotalLimit && (state.TotalUsed-amount) < state.TotalLimit
	crossedDaily := state.DailyLimit > 0 && state.DailyUsed >= state.DailyLimit && (state.DailyUsed-amount) < state.DailyLimit
	crossedWeekly := state.WeeklyLimit > 0 && state.WeeklyUsed >= state.WeeklyLimit && (state.WeeklyUsed-amount) < state.WeeklyLimit
	if crossedTotal || crossedDaily || crossedWeekly {
		if err := enqueueSchedulerOutbox(ctx, tx, service.SchedulerOutboxEventAccountChanged, &accountID, nil, nil); err != nil {
			logger.LegacyPrintf("repository.usage_billing", "[SchedulerOutbox] enqueue quota exceeded failed: account=%d err=%v", accountID, err)
			return nil, err
		}
	}
	return state, nil
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f
	default:
		return 0
	}
}

func parseExtraTime(v any) time.Time {
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func round6(v float64) float64 {
	return math.Round(v*1e6) / 1e6
}
