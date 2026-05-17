package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/paymentproviderinstance"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// --- Refund Flow ---

// getOrderProviderInstance looks up the provider instance that processed this order.
// For legacy orders without provider_instance_id, it resolves only when the
// historical instance is uniquely identifiable from the stored order fields.
func (s *PaymentService) getOrderProviderInstance(ctx context.Context, o *dbent.PaymentOrder) (*dbent.PaymentProviderInstance, error) {
	if s == nil || s.entClient == nil || o == nil {
		return nil, nil
	}

	if snapshot := psOrderProviderSnapshot(o); snapshot != nil {
		return s.resolveSnapshotOrderProviderInstance(ctx, o, snapshot)
	}

	instIDStr := strings.TrimSpace(psStringValue(o.ProviderInstanceID))
	if instIDStr == "" {
		return s.resolveUniqueLegacyOrderProviderInstance(ctx, o)
	}

	instID, err := strconv.ParseInt(instIDStr, 10, 64)
	if err != nil {
		return nil, nil
	}
	return s.entClient.PaymentProviderInstance.Get(ctx, instID)
}

// getRefundOrderProviderInstance resolves the provider instance for refund paths.
// Refunds must be pinned to an explicit historical binding, so legacy
// "best-effort" provider guessing is intentionally not allowed here.
func (s *PaymentService) getRefundOrderProviderInstance(ctx context.Context, o *dbent.PaymentOrder) (*dbent.PaymentProviderInstance, error) {
	if s == nil || s.entClient == nil || o == nil {
		return nil, nil
	}

	if snapshot := psOrderProviderSnapshot(o); snapshot != nil {
		return s.resolveSnapshotOrderProviderInstance(ctx, o, snapshot)
	}

	instIDStr := strings.TrimSpace(psStringValue(o.ProviderInstanceID))
	if instIDStr == "" {
		return nil, nil
	}

	instID, err := strconv.ParseInt(instIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("order %d refund provider instance id is invalid: %s", o.ID, instIDStr)
	}
	inst, err := s.entClient.PaymentProviderInstance.Get(ctx, instID)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, fmt.Errorf("order %d refund provider instance %s is missing", o.ID, instIDStr)
		}
		return nil, err
	}
	return inst, nil
}

func (s *PaymentService) resolveUniqueLegacyOrderProviderInstance(ctx context.Context, o *dbent.PaymentOrder) (*dbent.PaymentProviderInstance, error) {
	paymentType := payment.GetBasePaymentType(strings.TrimSpace(o.PaymentType))
	providerKey := strings.TrimSpace(psStringValue(o.ProviderKey))
	if providerKey != "" {
		instances, err := s.entClient.PaymentProviderInstance.Query().
			Where(paymentproviderinstance.ProviderKeyEQ(providerKey)).
			All(ctx)
		if err != nil {
			return nil, err
		}
		matched := psFilterLegacyOrderProviderInstances(paymentType, instances)
		if len(matched) == 1 {
			return matched[0], nil
		}
		return nil, nil
	}

	if paymentType == "" {
		return nil, nil
	}

	instances, err := s.entClient.PaymentProviderInstance.Query().
		All(ctx)
	if err != nil {
		return nil, err
	}

	matched := psFilterLegacyOrderProviderInstances(paymentType, instances)
	if len(matched) == 1 {
		return matched[0], nil
	}
	return nil, nil
}

func psFilterLegacyOrderProviderInstances(orderPaymentType string, instances []*dbent.PaymentProviderInstance) []*dbent.PaymentProviderInstance {
	if len(instances) == 0 {
		return nil
	}
	if strings.TrimSpace(orderPaymentType) == "" {
		return instances
	}
	var matched []*dbent.PaymentProviderInstance
	for _, inst := range instances {
		if psLegacyOrderMatchesInstance(orderPaymentType, inst) {
			matched = append(matched, inst)
		}
	}
	return matched
}

func psLegacyOrderMatchesInstance(orderPaymentType string, inst *dbent.PaymentProviderInstance) bool {
	if inst == nil {
		return false
	}

	baseType := payment.GetBasePaymentType(strings.TrimSpace(orderPaymentType))
	instanceProviderKey := strings.TrimSpace(inst.ProviderKey)
	if baseType == "" {
		return false
	}

	if baseType == payment.TypeStripe {
		return instanceProviderKey == payment.TypeStripe
	}
	if instanceProviderKey == payment.TypeStripe {
		return false
	}
	if instanceProviderKey == baseType {
		return true
	}
	return payment.InstanceSupportsType(inst.SupportedTypes, baseType)
}

func (s *PaymentService) RequestRefund(ctx context.Context, oid, uid int64, reason string) error {
	o, err := s.validateRefundRequest(ctx, oid, uid)
	if err != nil {
		return err
	}
	u, err := s.userRepo.GetByID(ctx, o.UserID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	if u.Balance < o.Amount {
		return infraerrors.BadRequest("BALANCE_NOT_ENOUGH", "refund amount exceeds balance")
	}
	nr := strings.TrimSpace(reason)
	now := time.Now()
	by := fmt.Sprintf("%d", uid)
	c, err := s.entClient.PaymentOrder.Update().Where(paymentorder.IDEQ(oid), paymentorder.UserIDEQ(uid), paymentorder.StatusEQ(OrderStatusCompleted), paymentorder.OrderTypeEQ(payment.OrderTypeBalance)).SetStatus(OrderStatusRefundRequested).SetRefundRequestedAt(now).SetRefundRequestReason(nr).SetRefundRequestedBy(by).SetRefundAmount(o.Amount).Save(ctx)
	if err != nil {
		return fmt.Errorf("update: %w", err)
	}
	if c == 0 {
		return infraerrors.Conflict("CONFLICT", "order status changed")
	}
	s.writeAuditLog(ctx, oid, "REFUND_REQUESTED", fmt.Sprintf("user:%d", uid), map[string]any{"amount": o.Amount, "reason": nr})
	return nil
}

func (s *PaymentService) validateRefundRequest(ctx context.Context, oid, uid int64) (*dbent.PaymentOrder, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.UserID != uid {
		return nil, infraerrors.Forbidden("FORBIDDEN", "no permission")
	}
	if o.OrderType != payment.OrderTypeBalance {
		return nil, infraerrors.BadRequest("INVALID_ORDER_TYPE", "only balance orders can request refund")
	}
	if o.Status != OrderStatusCompleted {
		return nil, infraerrors.BadRequest("INVALID_STATUS", "only completed orders can request refund")
	}
	// Check provider instance allows user refund
	inst, err := s.getRefundOrderProviderInstance(ctx, o)
	if err != nil || inst == nil {
		return nil, infraerrors.Forbidden("USER_REFUND_DISABLED", "refund is not available for this order")
	}
	if !inst.AllowUserRefund {
		return nil, infraerrors.Forbidden("USER_REFUND_DISABLED", "user refund is not enabled for this provider")
	}
	return o, nil
}

func (s *PaymentService) PrepareRefund(ctx context.Context, oid int64, amt float64, reason string, force, deduct bool) (*RefundPlan, *RefundResult, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return nil, nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	if o.OrderType == payment.OrderTypeDailyLimitReset {
		return nil, nil, infraerrors.BadRequest("INVALID_ORDER_TYPE", "daily limit reset orders cannot be refunded")
	}
	ok := []string{OrderStatusCompleted, OrderStatusRefundRequested, OrderStatusRefundFailed}
	if !psSliceContains(ok, o.Status) {
		return nil, nil, infraerrors.BadRequest("INVALID_STATUS", "order status does not allow refund")
	}
	// Check provider instance allows admin refund
	inst, instErr := s.getRefundOrderProviderInstance(ctx, o)
	if instErr != nil {
		slog.Warn("refund: provider instance lookup failed", "orderID", oid, "error", instErr)
		return nil, nil, infraerrors.InternalServer("PROVIDER_LOOKUP_FAILED", "failed to look up payment provider for this order")
	}
	if inst == nil {
		// Legacy order without provider_instance_id — block refund
		return nil, nil, infraerrors.Forbidden("REFUND_DISABLED", "refund is not available for this order")
	}
	if !inst.RefundEnabled {
		return nil, nil, infraerrors.Forbidden("REFUND_DISABLED", "refund is not enabled for this provider")
	}
	if math.IsNaN(amt) || math.IsInf(amt, 0) {
		return nil, nil, infraerrors.BadRequest("INVALID_AMOUNT", "invalid refund amount")
	}
	if amt <= 0 {
		if o.OrderType == payment.OrderTypeSubscription {
			autoAmount, autoErr := s.calculateSubscriptionRefundAmount(ctx, o)
			if autoErr != nil {
				return nil, nil, autoErr
			}
			if autoAmount <= 0 {
				return nil, nil, infraerrors.BadRequest("NO_REFUNDABLE_AMOUNT", "subscription has no refundable amount")
			}
			amt = autoAmount
		} else {
			amt = o.Amount
		}
	}
	if amt-o.Amount > amountToleranceCNY {
		return nil, nil, infraerrors.BadRequest("REFUND_AMOUNT_EXCEEDED", "refund amount exceeds recharge")
	}
	ga := calculateGatewayRefundAmount(o.Amount, o.PayAmount, amt)
	rr := strings.TrimSpace(reason)
	if rr == "" && o.RefundRequestReason != nil {
		rr = *o.RefundRequestReason
	}
	if rr == "" {
		rr = fmt.Sprintf("refund order:%d", o.ID)
	}
	p := &RefundPlan{OrderID: oid, Order: o, RefundAmount: amt, GatewayAmount: ga, Reason: rr, Force: force, DeductBalance: deduct, DeductionType: payment.DeductionTypeNone}
	if deduct {
		if er := s.prepDeduct(ctx, o, p, force); er != nil {
			return nil, er, nil
		}
	}
	return p, nil, nil
}

func (s *PaymentService) prepDeduct(ctx context.Context, o *dbent.PaymentOrder, p *RefundPlan, force bool) *RefundResult {
	if o.OrderType == payment.OrderTypeSubscription {
		p.DeductionType = payment.DeductionTypeSubscription
		if o.SubscriptionGroupID != nil && o.SubscriptionDays != nil {
			sub, err := s.resolveRefundSubscription(ctx, o)
			if err == nil && sub != nil {
				p.SubscriptionID = sub.ID
				p.SubDaysToDeduct = subscriptionRemainingDays(sub, time.Now())
			} else if !force {
				return &RefundResult{Success: false, Warning: "cannot find active subscription for deduction, use force", RequireForce: true}
			} else {
				p.SubDaysToDeduct = *o.SubscriptionDays
			}
		}
		return nil
	}
	u, err := s.userRepo.GetByID(ctx, o.UserID)
	if err != nil {
		if !force {
			return &RefundResult{Success: false, Warning: "cannot fetch user balance, use force", RequireForce: true}
		}
		return nil
	}
	p.DeductionType = payment.DeductionTypeBalance
	p.BalanceToDeduct = math.Min(p.RefundAmount, u.Balance)
	return nil
}

func (s *PaymentService) ExecuteRefund(ctx context.Context, p *RefundPlan) (*RefundResult, error) {
	c, err := s.entClient.PaymentOrder.Update().Where(paymentorder.IDEQ(p.OrderID), paymentorder.StatusIn(OrderStatusCompleted, OrderStatusRefundRequested, OrderStatusRefundFailed)).SetStatus(OrderStatusRefunding).Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("lock: %w", err)
	}
	if c == 0 {
		return nil, infraerrors.Conflict("CONFLICT", "order status changed")
	}
	if p.DeductionType == payment.DeductionTypeBalance && p.BalanceToDeduct > 0 {
		// Skip balance deduction on retry if previous attempt already deducted
		// but failed to roll back (REFUND_ROLLBACK_FAILED in audit log).
		if !s.hasAuditLog(ctx, p.OrderID, "REFUND_ROLLBACK_FAILED") {
			if err := s.userRepo.DeductBalance(ctx, p.Order.UserID, p.BalanceToDeduct); err != nil {
				s.restoreStatus(ctx, p)
				return nil, fmt.Errorf("deduction: %w", err)
			}
		} else {
			slog.Warn("skipping balance deduction on retry (previous rollback failed)", "orderID", p.OrderID)
			p.BalanceToDeduct = 0
		}
	}
	if p.DeductionType == payment.DeductionTypeSubscription && p.SubDaysToDeduct > 0 && p.SubscriptionID > 0 {
		if !s.hasAuditLog(ctx, p.OrderID, "REFUND_ROLLBACK_FAILED") {
			_, err := s.subscriptionSvc.ExtendSubscription(ctx, p.SubscriptionID, -p.SubDaysToDeduct)
			if err != nil {
				if errors.Is(err, ErrAdjustWouldExpire) {
					// Deduction would expire the subscription — revoke it entirely
					slog.Info("subscription deduction would expire, revoking", "orderID", p.OrderID, "subID", p.SubscriptionID, "days", p.SubDaysToDeduct)
					if revokeErr := s.subscriptionSvc.RevokeSubscription(ctx, p.SubscriptionID); revokeErr != nil {
						s.restoreStatus(ctx, p)
						return nil, fmt.Errorf("revoke subscription: %w", revokeErr)
					}
				} else {
					// Other errors (DB failure, not found) — abort refund
					s.restoreStatus(ctx, p)
					return nil, fmt.Errorf("deduct subscription days: %w", err)
				}
			}
		} else {
			slog.Warn("skipping subscription deduction on retry (previous rollback failed)", "orderID", p.OrderID)
			p.SubDaysToDeduct = 0
		}
	}
	if err := s.gwRefund(ctx, p); err != nil {
		return s.handleGwFail(ctx, p, err)
	}
	return s.markRefundOk(ctx, p)
}

func (s *PaymentService) gwRefund(ctx context.Context, p *RefundPlan) error {
	if p.Order.PaymentTradeNo == "" {
		s.writeAuditLog(ctx, p.Order.ID, "REFUND_NO_TRADE_NO", "admin", map[string]any{"detail": "skipped"})
		return nil
	}

	// Use the exact provider instance that created this order, not a random one
	// from the registry. Each instance has its own merchant credentials.
	prov, err := s.getRefundProvider(ctx, p.Order)
	if err != nil {
		return fmt.Errorf("get refund provider: %w", err)
	}
	if err := validateProviderSnapshotMetadata(p.Order, prov.ProviderKey(), providerMerchantIdentityMetadata(prov)); err != nil {
		s.writeAuditLog(ctx, p.Order.ID, "REFUND_PROVIDER_METADATA_MISMATCH", "admin", map[string]any{
			"detail": err.Error(),
		})
		return err
	}
	_, err = prov.Refund(ctx, payment.RefundRequest{
		TradeNo: p.Order.PaymentTradeNo,
		OrderID: p.Order.OutTradeNo,
		Amount:  strconv.FormatFloat(p.GatewayAmount, 'f', 2, 64),
		Reason:  p.Reason,
	})
	return err
}

// getRefundProvider creates a provider using the order's original instance config.
// Delegates to getOrderProvider which handles instance lookup and fallback.
func (s *PaymentService) getRefundProvider(ctx context.Context, o *dbent.PaymentOrder) (payment.Provider, error) {
	inst, err := s.getRefundOrderProviderInstance(ctx, o)
	if err != nil {
		return nil, err
	}
	if inst == nil {
		return nil, fmt.Errorf("refund provider instance is unavailable for order %d", o.ID)
	}
	return s.createProviderFromInstance(ctx, inst)
}

func (s *PaymentService) handleGwFail(ctx context.Context, p *RefundPlan, gErr error) (*RefundResult, error) {
	if s.RollbackRefund(ctx, p, gErr) {
		s.restoreStatus(ctx, p)
		s.writeAuditLog(ctx, p.OrderID, "REFUND_GATEWAY_FAILED", "admin", map[string]any{"detail": psErrMsg(gErr)})
		return &RefundResult{Success: false, Warning: "gateway failed: " + psErrMsg(gErr) + ", rolled back"}, nil
	}
	now := time.Now()
	_, _ = s.entClient.PaymentOrder.UpdateOneID(p.OrderID).SetStatus(OrderStatusRefundFailed).SetFailedAt(now).SetFailedReason(psErrMsg(gErr)).Save(ctx)
	s.writeAuditLog(ctx, p.OrderID, "REFUND_FAILED", "admin", map[string]any{"detail": psErrMsg(gErr)})
	return nil, infraerrors.InternalServer("REFUND_FAILED", psErrMsg(gErr))
}

func (s *PaymentService) markRefundOk(ctx context.Context, p *RefundPlan) (*RefundResult, error) {
	fs := OrderStatusRefunded
	if p.RefundAmount < p.Order.Amount {
		fs = OrderStatusPartiallyRefunded
	}
	now := time.Now()
	_, err := s.entClient.PaymentOrder.UpdateOneID(p.OrderID).SetStatus(fs).SetRefundAmount(p.RefundAmount).SetRefundReason(p.Reason).SetRefundAt(now).SetForceRefund(p.Force).Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("mark refund: %w", err)
	}
	s.writeAuditLog(ctx, p.OrderID, "REFUND_SUCCESS", "admin", map[string]any{"refundAmount": p.RefundAmount, "reason": p.Reason, "balanceDeducted": p.BalanceToDeduct, "force": p.Force})
	return &RefundResult{Success: true, BalanceDeducted: p.BalanceToDeduct, SubDaysDeducted: p.SubDaysToDeduct}, nil
}

func (s *PaymentService) RollbackRefund(ctx context.Context, p *RefundPlan, gErr error) bool {
	if p.DeductionType == payment.DeductionTypeBalance && p.BalanceToDeduct > 0 {
		if err := s.userRepo.UpdateBalance(ctx, p.Order.UserID, p.BalanceToDeduct); err != nil {
			slog.Error("[CRITICAL] rollback failed", "orderID", p.OrderID, "amount", p.BalanceToDeduct, "error", err)
			s.writeAuditLog(ctx, p.OrderID, "REFUND_ROLLBACK_FAILED", "admin", map[string]any{"gatewayError": psErrMsg(gErr), "rollbackError": psErrMsg(err), "balanceDeducted": p.BalanceToDeduct})
			return false
		}
	}
	if p.DeductionType == payment.DeductionTypeSubscription && p.SubDaysToDeduct > 0 && p.SubscriptionID > 0 {
		if _, err := s.subscriptionSvc.ExtendSubscription(ctx, p.SubscriptionID, p.SubDaysToDeduct); err != nil {
			slog.Error("[CRITICAL] subscription rollback failed", "orderID", p.OrderID, "subID", p.SubscriptionID, "days", p.SubDaysToDeduct, "error", err)
			s.writeAuditLog(ctx, p.OrderID, "REFUND_ROLLBACK_FAILED", "admin", map[string]any{"gatewayError": psErrMsg(gErr), "rollbackError": psErrMsg(err), "subDaysDeducted": p.SubDaysToDeduct})
			return false
		}
	}
	return true
}

func (s *PaymentService) restoreStatus(ctx context.Context, p *RefundPlan) {
	rs := OrderStatusCompleted
	if p.Order.Status == OrderStatusRefundRequested {
		rs = OrderStatusRefundRequested
	}
	_, _ = s.entClient.PaymentOrder.UpdateOneID(p.OrderID).SetStatus(rs).Save(ctx)
}

func (s *PaymentService) PreviewRefund(ctx context.Context, oid int64, amt float64) (*RefundPreview, error) {
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		return nil, infraerrors.NotFound("NOT_FOUND", "order not found")
	}
	actuallyRefunded := 0.0
	if o.Status == OrderStatusPartiallyRefunded || o.Status == OrderStatusRefunded {
		actuallyRefunded = o.RefundAmount
	}
	maxRefundable := o.Amount - actuallyRefunded
	if maxRefundable < 0 {
		maxRefundable = 0
	}

	preview := &RefundPreview{
		RefundAmount:            amt,
		MaxRefundableAmount:     math.Round(maxRefundable*100) / 100,
		CalculatedAutomatically: false,
	}
	if o.OrderType == payment.OrderTypeSubscription && amt <= 0 {
		calculated, calcErr := s.calculateSubscriptionRefundAmount(ctx, o)
		if calcErr != nil {
			return nil, calcErr
		}
		preview.RefundAmount = calculated
		preview.CalculatedAutomatically = true
	}
	if preview.RefundAmount < 0 {
		preview.RefundAmount = 0
	}
	if preview.RefundAmount > preview.MaxRefundableAmount {
		preview.RefundAmount = preview.MaxRefundableAmount
	}
	preview.RefundAmount = math.Round(preview.RefundAmount*100) / 100
	return preview, nil
}

func (s *PaymentService) calculateSubscriptionRefundAmount(ctx context.Context, o *dbent.PaymentOrder) (float64, error) {
	if o == nil {
		return 0, infraerrors.BadRequest("INVALID_ORDER", "order is required")
	}
	if o.OrderType != payment.OrderTypeSubscription {
		return 0, infraerrors.BadRequest("INVALID_ORDER_TYPE", "only subscription orders support automatic refund calculation")
	}
	if o.SubscriptionGroupID == nil || o.SubscriptionDays == nil || *o.SubscriptionDays <= 0 {
		return 0, infraerrors.BadRequest("INVALID_SUBSCRIPTION_ORDER", "subscription order missing required refund metadata")
	}

	sub, err := s.resolveRefundSubscription(ctx, o)
	if err != nil {
		return 0, err
	}
	if sub == nil {
		return 0, infraerrors.BadRequest("SUBSCRIPTION_NOT_FOUND", "cannot find subscription for this order")
	}
	if !sub.ExpiresAt.After(time.Now()) {
		return 0, ErrSubscriptionExpired
	}
	if sub.Group == nil {
		sub.Group, err = s.groupRepo.GetByID(ctx, sub.GroupID)
		if err != nil {
			return 0, infraerrors.BadRequest("GROUP_NOT_FOUND", "subscription group not found")
		}
	}
	if sub.Group == nil || !sub.Group.HasDailyLimit() {
		return 0, infraerrors.BadRequest("DAILY_LIMIT_NOT_CONFIGURED", "subscription group has no daily limit, cannot calculate automatic refund")
	}

	orderDays := *o.SubscriptionDays
	if orderDays <= 0 {
		return 0, infraerrors.BadRequest("INVALID_SUBSCRIPTION_DAYS", "subscription days must be greater than zero")
	}
	totalDays := subscriptionEffectiveDays(sub)
	if totalDays <= 0 {
		totalDays = orderDays
	}

	now := time.Now()
	fullUsedDays := 0
	if now.After(sub.StartsAt) {
		fullUsedDays = int(now.Sub(sub.StartsAt) / dailyWindowDuration)
	}
	if fullUsedDays < 0 {
		fullUsedDays = 0
	}
	if fullUsedDays > totalDays {
		fullUsedDays = totalDays
	}

	remainingFullDays := totalDays - fullUsedDays - 1
	if remainingFullDays < 0 {
		remainingFullDays = 0
	}

	dailyLimit := *sub.Group.DailyLimitUSD
	renewalDailyWindowStarted := subscriptionOrderDailyWindowStarted(o, sub, now)
	todayRemainingRatio := 0.0
	if dailyLimit > 0 && fullUsedDays < totalDays && !renewalDailyWindowStarted {
		remaining := dailyLimit - sub.DailyUsageUSD
		if remaining < 0 {
			remaining = 0
		}
		todayRemainingRatio = remaining / dailyLimit
		if todayRemainingRatio > 1 {
			todayRemainingRatio = 1
		}
	}

	refundableDaysEquivalent := float64(remainingFullDays) + todayRemainingRatio
	maxEquivalent := float64(totalDays)
	if renewalDailyWindowStarted {
		// The current day granted by this renewal has already started. Avoid
		// refunding it as unused just because fulfillment reset daily_usage_usd.
		maxEquivalent = float64(orderDays - 1)
		if maxEquivalent < 0 {
			maxEquivalent = 0
		}
	}
	if refundableDaysEquivalent < 0 {
		refundableDaysEquivalent = 0
	}
	if refundableDaysEquivalent > maxEquivalent {
		refundableDaysEquivalent = maxEquivalent
	}

	refundAmount := o.Amount * refundableDaysEquivalent / float64(totalDays)
	if refundAmount < 0 {
		refundAmount = 0
	}
	if refundAmount > o.Amount {
		refundAmount = o.Amount
	}
	return math.Round(refundAmount*100) / 100, nil
}

type PaymentOrderRefundDisplay struct {
	CanRefund                 bool
	SubscriptionExpiresAt     *time.Time
	SubscriptionRemainingDays *int
}

func (s *PaymentService) GetOrderRefundDisplay(ctx context.Context, o *dbent.PaymentOrder) PaymentOrderRefundDisplay {
	result := PaymentOrderRefundDisplay{}
	if o == nil {
		return result
	}
	okStatuses := []string{OrderStatusCompleted, OrderStatusRefundRequested, OrderStatusRefundFailed, OrderStatusPartiallyRefunded}
	if !psSliceContains(okStatuses, o.Status) || o.OrderType == payment.OrderTypeDailyLimitReset {
		return result
	}
	if o.OrderType != payment.OrderTypeSubscription {
		result.CanRefund = true
		return result
	}
	sub, err := s.resolveRefundSubscription(ctx, o)
	if err != nil || sub == nil {
		result.CanRefund = true
		return result
	}
	expiresAt := sub.ExpiresAt
	result.SubscriptionExpiresAt = &expiresAt
	remainingDays := subscriptionRemainingDays(sub, time.Now())
	result.SubscriptionRemainingDays = &remainingDays
	result.CanRefund = true
	return result
}

func (s *PaymentService) resolveRefundSubscription(ctx context.Context, o *dbent.PaymentOrder) (*UserSubscription, error) {
	if s == nil || o == nil || s.entClient == nil {
		return nil, ErrSubscriptionNotFound
	}
	if o.SubscriptionID != nil && *o.SubscriptionID > 0 && s.subscriptionSvc != nil {
		sub, err := s.subscriptionSvc.GetByID(ctx, *o.SubscriptionID)
		if err == nil && sub != nil {
			return sub, nil
		}
	}
	if o.SubscriptionGroupID == nil {
		return nil, ErrSubscriptionNotFound
	}

	orderNote := fmt.Sprintf("payment order %d", o.ID)
	candidates, err := s.entClient.UserSubscription.Query().
		Where(
			usersubscription.UserIDEQ(o.UserID),
			usersubscription.GroupIDEQ(*o.SubscriptionGroupID),
			usersubscription.SourceEQ(domain.SubscriptionSourcePayment),
		).
		WithGroup().
		All(ctx)
	if err != nil {
		return nil, err
	}

	var matched []*dbent.UserSubscription
	var noteMatched []*dbent.UserSubscription
	for _, item := range candidates {
		matched = append(matched, item)
		if item.Notes != nil && strings.Contains(*item.Notes, orderNote) {
			noteMatched = append(noteMatched, item)
		}
	}
	if len(noteMatched) == 1 {
		return paymentRefundEntSubscriptionToService(noteMatched[0]), nil
	}
	if len(noteMatched) > 1 {
		return nil, infraerrors.Conflict("SUBSCRIPTION_MATCH_CONFLICT", "multiple subscriptions matched this order")
	}
	if len(matched) == 1 {
		return paymentRefundEntSubscriptionToService(matched[0]), nil
	}
	if len(matched) == 0 {
		return nil, ErrSubscriptionNotFound
	}
	return nil, infraerrors.Conflict("SUBSCRIPTION_MATCH_CONFLICT", "cannot uniquely resolve subscription for this order")
}

func paymentRefundEntSubscriptionToService(sub *dbent.UserSubscription) *UserSubscription {
	if sub == nil {
		return nil
	}
	result := &UserSubscription{
		ID:                 sub.ID,
		UserID:             sub.UserID,
		GroupID:            sub.GroupID,
		StartsAt:           sub.StartsAt,
		ExpiresAt:          sub.ExpiresAt,
		Status:             sub.Status,
		DailyWindowStart:   sub.DailyWindowStart,
		WeeklyWindowStart:  sub.WeeklyWindowStart,
		MonthlyWindowStart: sub.MonthlyWindowStart,
		DailyUsageUSD:      sub.DailyUsageUsd,
		WeeklyUsageUSD:     sub.WeeklyUsageUsd,
		MonthlyUsageUSD:    sub.MonthlyUsageUsd,
		AssignedBy:         sub.AssignedBy,
		AssignedAt:         sub.AssignedAt,
		Source:             sub.Source,
		CreatedAt:          sub.CreatedAt,
		UpdatedAt:          sub.UpdatedAt,
	}
	if sub.Notes != nil {
		result.Notes = *sub.Notes
	}
	if sub.Edges.Group != nil {
		result.Group = &Group{
			ID:               sub.Edges.Group.ID,
			Name:             sub.Edges.Group.Name,
			Status:           sub.Edges.Group.Status,
			Platform:         sub.Edges.Group.Platform,
			SubscriptionType: sub.Edges.Group.SubscriptionType,
			CreatedAt:        sub.Edges.Group.CreatedAt,
			UpdatedAt:        sub.Edges.Group.UpdatedAt,
		}
		if sub.Edges.Group.DailyLimitUsd != nil {
			v := *sub.Edges.Group.DailyLimitUsd
			result.Group.DailyLimitUSD = &v
		}
		if sub.Edges.Group.WeeklyLimitUsd != nil {
			v := *sub.Edges.Group.WeeklyLimitUsd
			result.Group.WeeklyLimitUSD = &v
		}
		if sub.Edges.Group.MonthlyLimitUsd != nil {
			v := *sub.Edges.Group.MonthlyLimitUsd
			result.Group.MonthlyLimitUSD = &v
		}
	}
	return result
}

func subscriptionOrderDailyWindowStarted(o *dbent.PaymentOrder, sub *UserSubscription, now time.Time) bool {
	if o == nil || sub == nil || sub.DailyWindowStart == nil {
		return false
	}
	orderTime := o.CompletedAt
	if orderTime == nil {
		orderTime = o.PaidAt
	}
	if orderTime == nil {
		return false
	}
	if now.Before(*sub.DailyWindowStart) || !now.Before(sub.DailyWindowStart.Add(dailyWindowDuration)) {
		return false
	}
	// Paid subscription renewal resets daily_window_start at fulfillment time.
	// When refunding inside that freshly granted 24h window, do not treat
	// daily_usage_usd=0 as a fully refundable unused day; the day card has
	// already started and should only leave future full days refundable.
	return !sub.DailyWindowStart.Before(*orderTime)
}

func subscriptionEffectiveDays(sub *UserSubscription) int {
	if sub == nil {
		return 0
	}
	duration := sub.ExpiresAt.Sub(sub.StartsAt)
	if duration <= 0 {
		return 0
	}
	days := int(math.Ceil(duration.Hours() / 24))
	if days < 1 {
		return 1
	}
	return days
}

func subscriptionRemainingDays(sub *UserSubscription, now time.Time) int {
	if sub == nil || !sub.ExpiresAt.After(now) {
		return 0
	}
	days := int(math.Ceil(sub.ExpiresAt.Sub(now).Hours() / 24))
	if days < 0 {
		return 0
	}
	return days
}
