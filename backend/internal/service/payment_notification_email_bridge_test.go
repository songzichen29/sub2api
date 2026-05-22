//go:build unit

package service

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

type paymentNotificationMemorySettings struct {
	mu   sync.Mutex
	data map[string]string
}

func newPaymentNotificationMemorySettings() *paymentNotificationMemorySettings {
	return &paymentNotificationMemorySettings{data: map[string]string{}}
}

func (s *paymentNotificationMemorySettings) Get(ctx context.Context, key string) (*Setting, error) {
	value, err := s.GetValue(ctx, key)
	if err != nil {
		return nil, err
	}
	return &Setting{Key: key, Value: value}, nil
}

func (s *paymentNotificationMemorySettings) GetValue(ctx context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.data[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (s *paymentNotificationMemorySettings) Set(ctx context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
	return nil
}

func (s *paymentNotificationMemorySettings) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, err := s.GetValue(ctx, key); err == nil {
			out[key] = value
		}
	}
	return out, nil
}

func (s *paymentNotificationMemorySettings) SetMultiple(ctx context.Context, settings map[string]string) error {
	for key, value := range settings {
		if err := s.Set(ctx, key, value); err != nil {
			return err
		}
	}
	return nil
}

func (s *paymentNotificationMemorySettings) GetAll(ctx context.Context) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]string, len(s.data))
	for key, value := range s.data {
		out[key] = value
	}
	return out, nil
}

func (s *paymentNotificationMemorySettings) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}

type paymentNotificationCaptureEmailService struct {
	mu   sync.Mutex
	sent []paymentNotificationSentEmail
}

type paymentNotificationSentEmail struct {
	to      string
	subject string
	body    string
}

func (s *paymentNotificationCaptureEmailService) SendEmail(ctx context.Context, to, subject, body string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, paymentNotificationSentEmail{to: to, subject: subject, body: body})
	return nil
}

func (s *paymentNotificationCaptureEmailService) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sent)
}

func (s *paymentNotificationCaptureEmailService) last() paymentNotificationSentEmail {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sent) == 0 {
		return paymentNotificationSentEmail{}
	}
	return s.sent[len(s.sent)-1]
}

type paymentNotificationUserRepo struct {
	user *User
}

func (r paymentNotificationUserRepo) GetByID(context.Context, int64) (*User, error) {
	return r.user, nil
}

func newPaymentNotificationBridgeTestClient(t *testing.T) *dbent.Client {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func newPaymentNotificationEmailService(settings SettingRepository, capture *paymentNotificationCaptureEmailService) *NotificationEmailService {
	emailSvc := &EmailService{settingRepo: settings}
	emailSvc.sendFunc = capture.SendEmail
	return NewNotificationEmailService(settings, emailSvc)
}

func TestPaymentNotificationEmailBridgeSendsBalanceRechargeSuccess(t *testing.T) {
	ctx := context.Background()
	client := newPaymentNotificationBridgeTestClient(t)
	settings := newPaymentNotificationMemorySettings()
	capture := &paymentNotificationCaptureEmailService{}
	notificationSvc := newPaymentNotificationEmailService(settings, capture)
	newPaymentNotificationEmailBridgeForTest(client, notificationSvc, paymentNotificationUserRepo{user: &User{ID: 7, Balance: 42.5}}, nil)

	order := createPaymentNotificationBridgeOrder(t, ctx, client, payment.OrderTypeBalance)
	client.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(order.ID, 10)).
		SetAction("RECHARGE_SUCCESS").
		SetDetail("{}").
		SetOperator("system").
		SaveX(ctx)

	require.Eventually(t, func() bool { return capture.count() == 1 }, time.Second, 10*time.Millisecond)
	sent := capture.last()
	require.Equal(t, order.UserEmail, sent.to)
	require.Contains(t, sent.subject, "Balance recharge successful")
	require.Contains(t, sent.body, "12.34")
	require.Contains(t, sent.body, "42.50")
}

func TestPaymentNotificationEmailBridgeDeduplicatesByOrderAndEvent(t *testing.T) {
	ctx := context.Background()
	client := newPaymentNotificationBridgeTestClient(t)
	settings := newPaymentNotificationMemorySettings()
	capture := &paymentNotificationCaptureEmailService{}
	notificationSvc := newPaymentNotificationEmailService(settings, capture)
	newPaymentNotificationEmailBridgeForTest(client, notificationSvc, paymentNotificationUserRepo{user: &User{ID: 7, Balance: 42.5}}, nil)

	order := createPaymentNotificationBridgeOrder(t, ctx, client, payment.OrderTypeBalance)
	for i := 0; i < 2; i++ {
		client.PaymentAuditLog.Create().
			SetOrderID(strconv.FormatInt(order.ID, 10)).
			SetAction("RECHARGE_SUCCESS").
			SetDetail("{}").
			SetOperator("system").
			SaveX(ctx)
	}

	require.Eventually(t, func() bool { return capture.count() == 1 }, time.Second, 10*time.Millisecond)
	require.Never(t, func() bool { return capture.count() > 1 }, 100*time.Millisecond, 10*time.Millisecond)
}

func TestPaymentNotificationEmailBridgeSendsSubscriptionPurchaseSuccess(t *testing.T) {
	ctx := context.Background()
	client := newPaymentNotificationBridgeTestClient(t)
	settings := newPaymentNotificationMemorySettings()
	capture := &paymentNotificationCaptureEmailService{}
	notificationSvc := newPaymentNotificationEmailService(settings, capture)
	newPaymentNotificationEmailBridgeForTest(client, notificationSvc, paymentNotificationUserRepo{user: &User{ID: 7}}, nil)

	order := createPaymentNotificationBridgeOrder(t, ctx, client, payment.OrderTypeSubscription)
	client.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(order.ID, 10)).
		SetAction("SUBSCRIPTION_SUCCESS").
		SetDetail("{}").
		SetOperator("system").
		SaveX(ctx)

	require.Eventually(t, func() bool { return capture.count() == 1 }, time.Second, 10*time.Millisecond)
	sent := capture.last()
	require.Equal(t, order.UserEmail, sent.to)
	require.Contains(t, sent.subject, "Subscription purchase successful")
	require.Contains(t, sent.body, "30")
	require.Contains(t, sent.body, "Subscription")
}

func createPaymentNotificationBridgeOrder(t *testing.T, ctx context.Context, client *dbent.Client, orderType string) *dbent.PaymentOrder {
	t.Helper()
	user := client.User.Create().
		SetEmail("buyer@example.com").
		SetPasswordHash("hash").
		SetUsername("Buyer").
		SetRole(RoleUser).
		SetStatus(StatusActive).
		SaveX(ctx)
	builder := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(12.34).
		SetPayAmount(12.34).
		SetFeeRate(0).
		SetRechargeCode("PAY-1").
		SetOutTradeNo("sub2_test").
		SetPaymentType("stripe").
		SetPaymentTradeNo("").
		SetOrderType(orderType).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("example.com")
	if orderType == payment.OrderTypeSubscription {
		group := client.Group.Create().
			SetName("Pro").
			SetRateMultiplier(1).
			SetStatus(StatusActive).
			SetPlatform("openai").
			SaveX(ctx)
		sub := client.UserSubscription.Create().
			SetUserID(user.ID).
			SetGroupID(group.ID).
			SetStartsAt(time.Now()).
			SetExpiresAt(time.Now().Add(30 * 24 * time.Hour)).
			SetStatus(SubscriptionStatusActive).
			SetAssignedAt(time.Now()).
			SaveX(ctx)
		builder.SetSubscriptionGroupID(group.ID).
			SetSubscriptionID(sub.ID).
			SetSubscriptionDays(30)
	}
	return builder.SaveX(ctx)
}
