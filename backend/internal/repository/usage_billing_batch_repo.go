package repository

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"sort"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

var (
	errUsageBillingBatchMixedUsers       = errors.New("usage billing batch contains commands of different users")
	errUsageBillingBatchDuplicateRequest = errors.New("usage billing batch contains duplicate request keys")
	errUsageBillingBatchNilCommand       = errors.New("usage billing batch contains nil command")
	errUsageBillingBatchNonFiniteAmount  = errors.New("usage billing batch contains non-finite amount")
)

type usageBillingRequestKey struct {
	requestID string
	apiKeyID  int64
}

// ApplyBatch 在一个事务内结算同一用户的多条计费命令。
//
// 每条命令的去重认领、各项累加与返回结果都与按 cmds 顺序逐条调用 Apply 一致：
//   - 按唯一键顺序复用 MySQL 的去重认领，保留归档指纹检查和生图冻结结算身份。
//   - 合并用户余额和 Key 额度写入；订阅逐条检查上限，账号配额保留原有六位小数规则。
//   - 逐条命令的扣费后余额、是否透支、Key 额度是否在该条耗尽、账号额度状态，
//     由合并语句返回的累加后值按命令顺序推算。
//
// 行锁按 user_subscriptions → users → api_keys → accounts 的顺序获取，表内按主键升序，
// 与 Apply 的加锁顺序一致。
//
// 命令校验失败（混合用户、重复请求键、缺 request_id、非有限金额）时直接返回 error。
// 返回 error 时没有任何命令生效；提交请求发出后连接中断时提交结果未知，与 Apply 相同。
func (r *usageBillingRepository) ApplyBatch(ctx context.Context, cmds []*service.UsageBillingCommand) (_ []service.UsageBillingBatchOutcome, err error) {
	if len(cmds) == 0 {
		return nil, nil
	}
	if r == nil || r.db == nil {
		return nil, errors.New("usage billing repository db is nil")
	}

	userID, err := validateUsageBillingBatch(cmds)
	if err != nil {
		return nil, err
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

	outcomes := make([]service.UsageBillingBatchOutcome, len(cmds))
	applied, err := r.claimUsageBillingBatch(ctx, tx, cmds, outcomes)
	if err != nil {
		return nil, err
	}
	for i := range cmds {
		if applied[i] {
			outcomes[i].Result = &service.UsageBillingApplyResult{Applied: true}
		}
	}

	if err := applyUsageBillingBatchSubscriptions(ctx, tx, cmds, applied, outcomes); err != nil {
		return nil, err
	}
	if err := applyUsageBillingBatchBalance(ctx, tx, userID, cmds, applied, outcomes); err != nil {
		return nil, err
	}
	if err := applyUsageBillingBatchAPIKeys(ctx, tx, cmds, applied, outcomes); err != nil {
		return nil, err
	}
	if err := applyUsageBillingBatchAccountQuotas(ctx, tx, cmds, applied, outcomes); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	return outcomes, nil
}

// validateUsageBillingBatch 规范化命令并校验：非空、request_id 非空、同一用户、(request_id, api_key_id) 互不相同。
func validateUsageBillingBatch(cmds []*service.UsageBillingCommand) (int64, error) {
	var userID int64
	seen := make(map[usageBillingRequestKey]struct{}, len(cmds))
	for i, cmd := range cmds {
		if cmd == nil {
			return 0, errUsageBillingBatchNilCommand
		}
		cmd.Normalize()
		if cmd.RequestID == "" {
			return 0, service.ErrUsageBillingRequestIDRequired
		}
		for _, amount := range []float64{cmd.BalanceCost, cmd.SubscriptionCost, cmd.APIKeyQuotaCost, cmd.APIKeyRateLimitCost, cmd.AccountQuotaCost} {
			if math.IsNaN(amount) || math.IsInf(amount, 0) {
				return 0, errUsageBillingBatchNonFiniteAmount
			}
		}
		if i == 0 {
			userID = cmd.UserID
		} else if cmd.UserID != userID {
			return 0, errUsageBillingBatchMixedUsers
		}
		key := usageBillingRequestKey{requestID: cmd.RequestID, apiKeyID: cmd.APIKeyID}
		if _, dup := seen[key]; dup {
			return 0, errUsageBillingBatchDuplicateRequest
		}
		seen[key] = struct{}{}
	}
	return userID, nil
}

// claimUsageBillingBatch 按唯一键排序认领，沿用单条结算的归档与冻结余额幂等规则。
func (r *usageBillingRepository) claimUsageBillingBatch(ctx context.Context, tx *sql.Tx, cmds []*service.UsageBillingCommand, outcomes []service.UsageBillingBatchOutcome) ([]bool, error) {
	order := make([]int, len(cmds))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool {
		left, right := cmds[order[a]], cmds[order[b]]
		if left.APIKeyID != right.APIKeyID {
			return left.APIKeyID < right.APIKeyID
		}
		return left.RequestID < right.RequestID
	})
	applied := make([]bool, len(cmds))
	for _, i := range order {
		claimed, err := r.claimUsageBillingKey(ctx, tx, cmds[i])
		if errors.Is(err, service.ErrUsageBillingRequestConflict) {
			outcomes[i].Err = err
			continue
		}
		if err != nil {
			return nil, err
		}
		if !claimed {
			outcomes[i].Result = &service.UsageBillingApplyResult{Applied: false}
			continue
		}
		if cmds[i].BalanceHoldAmount > 0 {
			if strings.TrimSpace(cmds[i].BalanceHoldSettlementID) == "" || strings.TrimSpace(cmds[i].BalanceHoldSettlementFingerprint) == "" {
				return nil, errors.New("usage billing balance hold settlement identity is required")
			}
			settled, err := r.claimUsageBillingRequest(ctx, tx, cmds[i].BalanceHoldSettlementID, cmds[i].APIKeyID, cmds[i].BalanceHoldSettlementFingerprint)
			if err != nil {
				return nil, err
			}
			if !settled {
				return nil, service.ErrUsageBillingBalanceHoldAlreadySettled
			}
		}
		applied[i] = true
	}
	return applied, nil
}

// usageBillingAmount 把已量化到 8 位小数的金额转为精确十进制。
func usageBillingAmount(v float64) decimal.Decimal {
	return decimal.NewFromFloat(v)
}

// usageBillingAmountGroup 汇总同一行上的多条命令：indexes 按命令顺序排列。
type usageBillingAmountGroup struct {
	sum     decimal.Decimal
	indexes []int
}

func groupUsageBillingAmounts(cmds []*service.UsageBillingCommand, applied []bool, pick func(*service.UsageBillingCommand) (int64, float64, bool)) (map[int64]*usageBillingAmountGroup, []int64) {
	groups := make(map[int64]*usageBillingAmountGroup)
	for i, cmd := range cmds {
		if !applied[i] {
			continue
		}
		id, amount, ok := pick(cmd)
		if !ok || !(amount > 0) {
			continue
		}
		g := groups[id]
		if g == nil {
			g = &usageBillingAmountGroup{}
			groups[id] = g
		}
		g.sum = g.sum.Add(usageBillingAmount(amount))
		g.indexes = append(g.indexes, i)
	}
	ids := make([]int64, 0, len(groups))
	for id := range groups {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(a, b int) bool { return ids[a] < ids[b] })
	return groups, ids
}

func applyUsageBillingBatchSubscriptions(ctx context.Context, tx *sql.Tx, cmds []*service.UsageBillingCommand, applied []bool, outcomes []service.UsageBillingBatchOutcome) error {
	groups, ids := groupUsageBillingAmounts(cmds, applied, func(cmd *service.UsageBillingCommand) (int64, float64, bool) {
		if cmd.SubscriptionID == nil {
			return 0, 0, false
		}
		return *cmd.SubscriptionID, cmd.SubscriptionCost, true
	})
	for _, id := range ids {
		for _, i := range groups[id].indexes {
			exhausted, err := incrementUsageBillingSubscription(ctx, tx, id, cmds[i].SubscriptionCost, cmds[i].AllowSubscriptionOverLimit)
			if err != nil {
				return err
			}
			outcomes[i].Result.SubscriptionQuotaExhausted = exhausted
		}
	}
	return nil
}

func applyUsageBillingBatchBalance(ctx context.Context, tx *sql.Tx, userID int64, cmds []*service.UsageBillingCommand, applied []bool, outcomes []service.UsageBillingBatchOutcome) error {
	for i, cmd := range cmds {
		if applied[i] && cmd.BalanceHoldAmount > 0 {
			// 冻结余额与普通扣费共存时，按原命令顺序保留每条结算后的可用余额。
			for j, entry := range cmds {
				if !applied[j] {
					continue
				}
				if entry.BalanceHoldAmount > 0 {
					balance, err := captureUsageBillingHeldBalance(ctx, tx, entry)
					if err != nil {
						return err
					}
					outcomes[j].Result.NewBalance = &balance
					continue
				}
				if entry.BalanceCost > 0 {
					balance, sufficient, err := deductUsageBillingBalance(ctx, tx, userID, entry.BalanceCost)
					if err != nil {
						return err
					}
					outcomes[j].Result.NewBalance = &balance
					outcomes[j].Result.BalanceOverdrafted = !sufficient
				}
			}
			return nil
		}
	}
	groups, _ := groupUsageBillingAmounts(cmds, applied, func(cmd *service.UsageBillingCommand) (int64, float64, bool) {
		return userID, cmd.BalanceCost, true
	})
	group := groups[userID]
	if group == nil {
		return nil
	}

	var running decimal.Decimal
	err := tx.QueryRowContext(ctx, `SELECT balance FROM users WHERE id = ? AND deleted_at IS NULL FOR UPDATE`, userID).Scan(&running)
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrUserNotFound
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET balance = balance - ?, updated_at = NOW() WHERE id = ? AND deleted_at IS NULL`, group.sum, userID); err != nil {
		return err
	}

	for _, i := range group.indexes {
		cost := usageBillingAmount(cmds[i].BalanceCost)
		sufficient := running.GreaterThanOrEqual(cost)
		running = running.Sub(cost)
		balance := running.InexactFloat64()
		outcomes[i].Result.NewBalance = &balance
		outcomes[i].Result.BalanceOverdrafted = !sufficient
	}
	return nil
}

func applyUsageBillingBatchAPIKeys(ctx context.Context, tx *sql.Tx, cmds []*service.UsageBillingCommand, applied []bool, outcomes []service.UsageBillingBatchOutcome) error {
	quotaGroups, quotaIDs := groupUsageBillingAmounts(cmds, applied, func(cmd *service.UsageBillingCommand) (int64, float64, bool) {
		return cmd.APIKeyID, cmd.APIKeyQuotaCost, true
	})
	rateGroups, rateIDs := groupUsageBillingAmounts(cmds, applied, func(cmd *service.UsageBillingCommand) (int64, float64, bool) {
		return cmd.APIKeyID, cmd.APIKeyRateLimitCost, true
	})

	ids := mergeSortedIDs(quotaIDs, rateIDs)
	for _, id := range ids {
		if group := quotaGroups[id]; group != nil {
			if err := applyUsageBillingBatchAPIKeyQuota(ctx, tx, id, group, cmds, outcomes); err != nil {
				return err
			}
		}
		if group := rateGroups[id]; group != nil {
			err := incrementUsageBillingAPIKeyRateLimit(ctx, tx, id, group.sum.InexactFloat64())
			if err != nil && !errors.Is(err, service.ErrAPIKeyNotFound) {
				return err
			}
		}
	}
	return nil
}

func applyUsageBillingBatchAPIKeyQuota(ctx context.Context, tx *sql.Tx, apiKeyID int64, group *usageBillingAmountGroup, cmds []*service.UsageBillingCommand, outcomes []service.UsageBillingBatchOutcome) error {
	var quota, running decimal.Decimal
	err := tx.QueryRowContext(ctx, `SELECT quota, quota_used FROM api_keys WHERE id = ? AND deleted_at IS NULL FOR UPDATE`, apiKeyID).Scan(&quota, &running)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE api_keys
		SET status = CASE
				WHEN quota > 0 AND status = ? AND quota_used < quota AND quota_used + ? >= quota THEN ?
				ELSE status
			END,
			quota_used = quota_used + ?, updated_at = NOW()
		WHERE id = ? AND deleted_at IS NULL
	`, service.StatusAPIKeyActive, group.sum, service.StatusAPIKeyQuotaExhausted, group.sum, apiKeyID); err != nil {
		return err
	}
	if !quota.IsPositive() {
		return nil
	}

	for _, i := range group.indexes {
		before := running
		running = running.Add(usageBillingAmount(cmds[i].APIKeyQuotaCost))
		if running.GreaterThanOrEqual(quota) && before.LessThan(quota) {
			outcomes[i].Result.APIKeyQuotaExhausted = true
		}
	}
	return nil
}

func applyUsageBillingBatchAccountQuotas(ctx context.Context, tx *sql.Tx, cmds []*service.UsageBillingCommand, applied []bool, outcomes []service.UsageBillingBatchOutcome) error {
	groups, ids := groupUsageBillingAmounts(cmds, applied, func(cmd *service.UsageBillingCommand) (int64, float64, bool) {
		return cmd.AccountID, cmd.AccountQuotaCost, service.AccountTypeSupportsQuota(cmd.AccountType)
	})
	for _, id := range ids {
		for _, i := range groups[id].indexes {
			state, err := incrementUsageBillingAccountQuota(ctx, tx, id, cmds[i].AccountQuotaCost, cmds[i].AccountQuotaHasDailyLimit, cmds[i].AccountQuotaHasWeeklyLimit)
			if err != nil && !errors.Is(err, service.ErrAccountNotFound) {
				return err
			}
			outcomes[i].Result.QuotaState = state
		}
	}
	return nil
}

// mergeSortedIDs 合并两个升序 id 列表并去重。
func mergeSortedIDs(a, b []int64) []int64 {
	out := make([]int64, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		switch {
		case j >= len(b) || (i < len(a) && a[i] < b[j]):
			out = append(out, a[i])
			i++
		case i >= len(a) || b[j] < a[i]:
			out = append(out, b[j])
			j++
		default:
			out = append(out, a[i])
			i++
			j++
		}
	}
	return out
}
