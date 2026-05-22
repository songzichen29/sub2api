package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
)

const (
	paymentFulfillmentEmailSourceType = "payment_order"
	paymentFulfillmentEmailTimeout    = 30 * time.Second
)

// PaymentNotificationEmailBridge observes immutable payment audit logs and sends
// user-facing payment success emails outside the payment fulfillment path.
// This keeps payment state transitions unchanged: notification failures are
// logged and deduplicated by NotificationEmailService delivery keys, but never
// affect order completion.
type PaymentNotificationEmailBridge struct {
	entClient                *dbent.Client
	notificationEmailService *NotificationEmailService
	userBalanceResolver      paymentNotificationUserBalanceResolver
	groupRepo                GroupRepository
}

type paymentNotificationUserBalanceResolver interface {
	GetByID(context.Context, int64) (*User, error)
}

func NewPaymentNotificationEmailBridge(
	entClient *dbent.Client,
	notificationEmailService *NotificationEmailService,
	userRepo UserRepository,
	groupRepo GroupRepository,
) *PaymentNotificationEmailBridge {
	bridge := &PaymentNotificationEmailBridge{
		entClient:                entClient,
		notificationEmailService: notificationEmailService,
		userBalanceResolver:      userRepo,
		groupRepo:                groupRepo,
	}
	bridge.installAuditLogHook()
	return bridge
}

func newPaymentNotificationEmailBridgeForTest(
	entClient *dbent.Client,
	notificationEmailService *NotificationEmailService,
	userBalanceResolver paymentNotificationUserBalanceResolver,
	groupRepo GroupRepository,
) *PaymentNotificationEmailBridge {
	bridge := &PaymentNotificationEmailBridge{
		entClient:                entClient,
		notificationEmailService: notificationEmailService,
		userBalanceResolver:      userBalanceResolver,
		groupRepo:                groupRepo,
	}
	bridge.installAuditLogHook()
	return bridge
}

func (b *PaymentNotificationEmailBridge) installAuditLogHook() {
	if b == nil || b.entClient == nil || b.notificationEmailService == nil {
		return
	}
	b.entClient.PaymentAuditLog.Use(func(next dbent.Mutator) dbent.Mutator {
		return dbent.MutateFunc(func(ctx context.Context, mutation dbent.Mutation) (dbent.Value, error) {
			value, err := next.Mutate(ctx, mutation)
			if err != nil || !mutation.Op().Is(dbent.OpCreate) {
				return value, err
			}
			auditMutation, ok := mutation.(*dbent.PaymentAuditLogMutation)
			if !ok {
				return value, err
			}
			orderID, _ := auditMutation.OrderID()
			action, _ := auditMutation.Action()
			b.dispatch(orderID, action)
			return value, nil
		})
	})
}

func (b *PaymentNotificationEmailBridge) dispatch(orderIDText, action string) {
	action = strings.TrimSpace(action)
	if action != "RECHARGE_SUCCESS" && action != "SUBSCRIPTION_SUCCESS" {
		return
	}
	orderID, err := strconv.ParseInt(strings.TrimSpace(orderIDText), 10, 64)
	if err != nil || orderID <= 0 {
		return
	}
	go b.send(context.Background(), orderID, action)
}

func (b *PaymentNotificationEmailBridge) send(parent context.Context, orderID int64, action string) {
	if b == nil || b.entClient == nil || b.notificationEmailService == nil {
		return
	}
	ctx, cancel := context.WithTimeout(parent, paymentFulfillmentEmailTimeout)
	defer cancel()

	order, err := b.entClient.PaymentOrder.Query().
		Where(paymentorder.IDEQ(orderID)).
		Only(ctx)
	if err != nil {
		slog.Warn("payment success notification email skipped: order lookup failed", "order_id", orderID, "action", action, "err", err)
		return
	}
	if err := b.sendForOrder(ctx, order, action); err != nil {
		slog.Warn("payment success notification email failed", "order_id", orderID, "action", action, "err", err)
	}
}

func (b *PaymentNotificationEmailBridge) sendForOrder(ctx context.Context, order *dbent.PaymentOrder, action string) error {
	if order == nil || strings.TrimSpace(order.UserEmail) == "" {
		return nil
	}
	switch action {
	case "RECHARGE_SUCCESS":
		return b.sendBalanceRechargeSuccess(ctx, order)
	case "SUBSCRIPTION_SUCCESS":
		return b.sendSubscriptionPurchaseSuccess(ctx, order)
	default:
		return nil
	}
}

func (b *PaymentNotificationEmailBridge) sendBalanceRechargeSuccess(ctx context.Context, order *dbent.PaymentOrder) error {
	return b.notificationEmailService.Send(ctx, NotificationEmailSendInput{
		Event:          NotificationEmailEventBalanceRechargeSuccess,
		RecipientEmail: order.UserEmail,
		RecipientName:  firstNonEmpty(order.UserName, order.UserEmail),
		UserID:         order.UserID,
		SourceType:     paymentFulfillmentEmailSourceType,
		SourceID:       strconv.FormatInt(order.ID, 10),
		Variables: map[string]string{
			"recharge_amount": fmt.Sprintf("%.2f", order.Amount),
			"current_balance": b.currentUserBalance(ctx, order.UserID),
			"order_id":        strconv.FormatInt(order.ID, 10),
		},
	})
}

func (b *PaymentNotificationEmailBridge) sendSubscriptionPurchaseSuccess(ctx context.Context, order *dbent.PaymentOrder) error {
	variables := map[string]string{
		"subscription_group": b.subscriptionGroupName(ctx, order),
		"subscription_days":  subscriptionDaysText(order),
		"expiry_time":        b.subscriptionExpiryTime(ctx, order),
		"order_id":           strconv.FormatInt(order.ID, 10),
	}
	return b.notificationEmailService.Send(ctx, NotificationEmailSendInput{
		Event:          NotificationEmailEventSubscriptionPurchaseSuccess,
		RecipientEmail: order.UserEmail,
		RecipientName:  firstNonEmpty(order.UserName, order.UserEmail),
		UserID:         order.UserID,
		SourceType:     paymentFulfillmentEmailSourceType,
		SourceID:       strconv.FormatInt(order.ID, 10),
		Variables:      variables,
	})
}

func (b *PaymentNotificationEmailBridge) currentUserBalance(ctx context.Context, userID int64) string {
	if b == nil || b.userBalanceResolver == nil || userID <= 0 {
		return ""
	}
	user, err := b.userBalanceResolver.GetByID(ctx, userID)
	if err != nil || user == nil {
		return ""
	}
	return fmt.Sprintf("%.2f", user.Balance)
}

func (b *PaymentNotificationEmailBridge) subscriptionGroupName(ctx context.Context, order *dbent.PaymentOrder) string {
	if order == nil || order.SubscriptionGroupID == nil || *order.SubscriptionGroupID <= 0 || b == nil || b.groupRepo == nil {
		return "Subscription"
	}
	group, err := b.groupRepo.GetByID(ctx, *order.SubscriptionGroupID)
	if err != nil || group == nil || strings.TrimSpace(group.Name) == "" {
		return "Subscription"
	}
	return strings.TrimSpace(group.Name)
}

func (b *PaymentNotificationEmailBridge) subscriptionExpiryTime(ctx context.Context, order *dbent.PaymentOrder) string {
	if order == nil || order.SubscriptionID == nil || *order.SubscriptionID <= 0 || b == nil || b.entClient == nil {
		return ""
	}
	sub, err := b.entClient.UserSubscription.Get(ctx, *order.SubscriptionID)
	if err != nil || sub == nil || sub.ExpiresAt.IsZero() {
		return ""
	}
	return sub.ExpiresAt.Format("2006-01-02 15:04")
}

func subscriptionDaysText(order *dbent.PaymentOrder) string {
	if order == nil || order.SubscriptionDays == nil || *order.SubscriptionDays <= 0 {
		return ""
	}
	return strconv.Itoa(*order.SubscriptionDays)
}

// Ensure the bridge provider is retained by wire even though callers do not use it directly.
func (b *PaymentNotificationEmailBridge) Start() {}

func (b *PaymentNotificationEmailBridge) Stop() {}
