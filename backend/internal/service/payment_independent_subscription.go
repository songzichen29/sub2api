package service

import (
	"context"
	"fmt"
	"strconv"

	"entgo.io/ent/dialect"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
)

// 新订单独立发放；订单行锁、订阅订单唯一键与同事务绑定保证回调重试不会重复赠送。
func (s *PaymentService) assignIndependentPaymentSubscription(ctx context.Context, order *dbent.PaymentOrder, input *AssignSubscriptionInput) (*UserSubscription, error) {
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()
	orderQuery := client.PaymentOrder.Query().Where(paymentorder.IDEQ(order.ID))
	// SQLite 没有行级 FOR UPDATE；事务写锁与订单唯一键仍约束发放。
	if client.Driver().Dialect() != dialect.SQLite {
		orderQuery.ForUpdate()
	}
	locked, err := orderQuery.Only(txCtx)
	if err != nil {
		return nil, err
	}
	existing, err := client.UserSubscription.Query().Where(usersubscription.PurchaseOrderIDEQ(order.ID)).Only(mixins.SkipSoftDelete(txCtx))
	if err != nil && !dbent.IsNotFound(err) {
		return nil, err
	}
	var sub *UserSubscription
	if existing != nil {
		sub, err = s.subscriptionSvc.userSubRepo.GetByIDIncludeDeleted(txCtx, existing.ID)
		if err != nil {
			return nil, err
		}
	}
	if existing == nil {
		input.PurchaseOrderID = &order.ID
		sub, err = s.subscriptionSvc.createSubscription(txCtx, input)
		if err != nil {
			return nil, fmt.Errorf("create independent subscription: %w", err)
		}
		if _, err := client.PaymentAuditLog.Create().SetOrderID(strconv.FormatInt(order.ID, 10)).SetAction("SUBSCRIPTION_ASSIGNED").SetDetail(fmt.Sprintf("subscription_id=%d", sub.ID)).SetOperator("system").Save(txCtx); err != nil {
			return nil, err
		}
	}
	if _, err := client.PaymentOrder.UpdateOneID(order.ID).SetSubscriptionID(sub.ID).SetUpdatedAt(locked.UpdatedAt).Save(txCtx); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	order.SubscriptionID = &sub.ID
	if err := s.subscriptionSvc.invalidateSubscriptionCaches(sub.UserID, sub.GroupID, sub.ID); err != nil {
		return nil, err
	}
	return sub, nil
}
