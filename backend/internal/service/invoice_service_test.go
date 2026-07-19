package service

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestInvoiceApplicationRejectReleasesOrdersAndInvoicedLocksThem(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := NewInvoiceService(client)

	user, err := client.User.Create().
		SetEmail("invoice@example.com").
		SetPasswordHash("hash").
		SetUsername("invoice-user").
		Save(ctx)
	require.NoError(t, err)

	header, err := svc.CreateHeader(ctx, user.ID, InvoiceHeaderInput{
		TitleType: InvoiceHeaderTypeCompany,
		Title:     "Example Company",
		TaxNumber: "91350000TEST000001",
		Email:     "finance@example.com",
		IsDefault: true,
	})
	require.NoError(t, err)

	order := createInvoiceTestOrder(t, ctx, client, user.ID, user.Email, user.Username, 300)

	application, err := svc.CreateApplication(ctx, user.ID, CreateInvoiceApplicationInput{
		OrderIDs: []int64{order.ID},
		HeaderID: header.ID,
	})
	require.NoError(t, err)
	require.Equal(t, InvoiceApplicationStatusPending, application.Status)
	require.Equal(t, 300.0, application.TotalAmount)

	lockedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, InvoiceOrderStatusProcessing, lockedOrder.InvoiceStatus)
	require.NotNil(t, lockedOrder.InvoiceApplicationID)
	require.Equal(t, application.ID, *lockedOrder.InvoiceApplicationID)

	_, err = svc.UpdateApplication(ctx, application.ID, 99, UpdateInvoiceApplicationInput{
		Status:          InvoiceApplicationStatusRejected,
		RejectionReason: "header needs correction",
	})
	require.NoError(t, err)

	releasedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, InvoiceOrderStatusUnapplied, releasedOrder.InvoiceStatus)
	require.Nil(t, releasedOrder.InvoiceApplicationID)

	secondApplication, err := svc.CreateApplication(ctx, user.ID, CreateInvoiceApplicationInput{
		OrderIDs: []int64{order.ID},
		HeaderID: header.ID,
	})
	require.NoError(t, err)
	_, err = svc.UpdateApplication(ctx, secondApplication.ID, 99, UpdateInvoiceApplicationInput{
		Status:        InvoiceApplicationStatusInvoiced,
		InvoiceNumber: "01234567",
	})
	require.NoError(t, err)

	invoicedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, InvoiceOrderStatusInvoiced, invoicedOrder.InvoiceStatus)

	_, err = svc.CreateApplication(ctx, user.ID, CreateInvoiceApplicationInput{
		OrderIDs: []int64{order.ID},
		HeaderID: header.ID,
	})
	require.Error(t, err)
}

func TestInvoiceApplicationRequiresMinimumAmount(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := NewInvoiceService(client)

	user, err := client.User.Create().
		SetEmail("invoice-min@example.com").
		SetPasswordHash("hash").
		SetUsername("invoice-min-user").
		Save(ctx)
	require.NoError(t, err)
	header, err := svc.CreateHeader(ctx, user.ID, InvoiceHeaderInput{TitleType: InvoiceHeaderTypePersonal, Title: "Invoice User"})
	require.NoError(t, err)
	order := createInvoiceTestOrder(t, ctx, client, user.ID, user.Email, user.Username, 299.99)

	_, err = svc.CreateApplication(ctx, user.ID, CreateInvoiceApplicationInput{OrderIDs: []int64{order.ID}, HeaderID: header.ID})
	require.Error(t, err)
}

func TestInvoiceApplicationRequiresInvoiceNumberWhenInvoiced(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	svc := NewInvoiceService(client)

	user, err := client.User.Create().
		SetEmail("invoice-number@example.com").
		SetPasswordHash("hash").
		SetUsername("invoice-number-user").
		Save(ctx)
	require.NoError(t, err)
	header, err := svc.CreateHeader(ctx, user.ID, InvoiceHeaderInput{TitleType: InvoiceHeaderTypePersonal, Title: "Invoice Number User"})
	require.NoError(t, err)
	order := createInvoiceTestOrder(t, ctx, client, user.ID, user.Email, user.Username, 300)
	application, err := svc.CreateApplication(ctx, user.ID, CreateInvoiceApplicationInput{OrderIDs: []int64{order.ID}, HeaderID: header.ID})
	require.NoError(t, err)

	_, err = svc.UpdateApplication(ctx, application.ID, 99, UpdateInvoiceApplicationInput{Status: InvoiceApplicationStatusInvoiced})
	require.Error(t, err)

	lockedOrder, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, InvoiceOrderStatusProcessing, lockedOrder.InvoiceStatus)

	_, err = svc.UpdateApplication(ctx, application.ID, 99, UpdateInvoiceApplicationInput{
		Status:        InvoiceApplicationStatusInvoiced,
		InvoiceNumber: "INV-20260719-001",
	})
	require.NoError(t, err)
}

func createInvoiceTestOrder(t *testing.T, ctx context.Context, client *dbent.Client, userID int64, email, username string, amount float64) *dbent.PaymentOrder {
	t.Helper()
	order, err := client.PaymentOrder.Create().
		SetUserID(userID).
		SetUserEmail(email).
		SetUserName(username).
		SetAmount(amount).
		SetPayAmount(amount).
		SetFeeRate(0).
		SetRechargeCode("INVOICE-ORDER").
		SetOutTradeNo("invoice-order-" + time.Now().Format("150405.000000000")).
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("invoice-trade").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusCompleted).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetPaidAt(time.Now()).
		SetCompletedAt(time.Now()).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)
	return order
}
