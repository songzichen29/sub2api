package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

type paidUserRateRepoStub struct {
	syncedUserID int64
	syncedRates  map[int64]*float64
	syncCalls    map[int64]map[int64]*float64
}

func (s *paidUserRateRepoStub) GetByUserID(context.Context, int64) (map[int64]float64, error) {
	panic("unexpected GetByUserID")
}
func (s *paidUserRateRepoStub) GetByUserAndGroup(context.Context, int64, int64) (*float64, error) {
	panic("unexpected GetByUserAndGroup")
}
func (s *paidUserRateRepoStub) GetRPMOverrideByUserAndGroup(context.Context, int64, int64) (*int, error) {
	panic("unexpected GetRPMOverrideByUserAndGroup")
}
func (s *paidUserRateRepoStub) GetByGroupID(context.Context, int64) ([]UserGroupRateEntry, error) {
	panic("unexpected GetByGroupID")
}
func (s *paidUserRateRepoStub) SyncUserGroupRates(_ context.Context, userID int64, rates map[int64]*float64) error {
	s.syncedUserID = userID
	s.syncedRates = rates
	if s.syncCalls == nil {
		s.syncCalls = map[int64]map[int64]*float64{}
	}
	copied := make(map[int64]*float64, len(rates))
	for groupID, rate := range rates {
		if rate == nil {
			copied[groupID] = nil
			continue
		}
		v := *rate
		copied[groupID] = &v
	}
	s.syncCalls[userID] = copied
	return nil
}
func (s *paidUserRateRepoStub) UpsertUserGroupRates(_ context.Context, entries []BatchUserGroupRateInput) error {
	if s.syncCalls == nil {
		s.syncCalls = map[int64]map[int64]*float64{}
	}
	for _, entry := range entries {
		if s.syncCalls[entry.UserID] == nil {
			s.syncCalls[entry.UserID] = map[int64]*float64{}
		}
		rate := entry.RateMultiplier
		s.syncCalls[entry.UserID][entry.GroupID] = &rate
	}
	return nil
}
func (s *paidUserRateRepoStub) SyncGroupRateMultipliers(context.Context, int64, []GroupRateMultiplierInput) error {
	panic("unexpected SyncGroupRateMultipliers")
}
func (s *paidUserRateRepoStub) SyncGroupRPMOverrides(context.Context, int64, []GroupRPMOverrideInput) error {
	panic("unexpected SyncGroupRPMOverrides")
}
func (s *paidUserRateRepoStub) ClearGroupRPMOverrides(context.Context, int64) error {
	panic("unexpected ClearGroupRPMOverrides")
}
func (s *paidUserRateRepoStub) DeleteByGroupID(context.Context, int64) error {
	panic("unexpected DeleteByGroupID")
}
func (s *paidUserRateRepoStub) DeleteByUserID(context.Context, int64) error {
	panic("unexpected DeleteByUserID")
}

func TestApplyPaidUserRateForBalanceOrder_SyncsConfiguredGroupRate(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	settingRepo := &paymentConfigSettingRepoStub{values: map[string]string{
		SettingPaidUserRateRules: `[{"group_id":88,"rate_multiplier":0.5},{"group_id":99,"rate_multiplier":0.8}]`,
	}}
	rateRepo := &paidUserRateRepoStub{}
	svc := &PaymentService{
		entClient:         client,
		configService:     NewPaymentConfigService(client, settingRepo, nil, nil),
		userGroupRateRepo: rateRepo,
	}

	err := svc.applyPaidUserRateForBalanceOrder(ctx, &dbent.PaymentOrder{
		ID:        123,
		UserID:    42,
		OrderType: payment.OrderTypeBalance,
	})
	require.NoError(t, err)
	require.Equal(t, int64(42), rateRepo.syncedUserID)
	require.Contains(t, rateRepo.syncedRates, int64(88))
	require.NotNil(t, rateRepo.syncedRates[88])
	require.InDelta(t, 0.5, *rateRepo.syncedRates[88], 1e-12)
	require.Contains(t, rateRepo.syncedRates, int64(99))
	require.NotNil(t, rateRepo.syncedRates[99])
	require.InDelta(t, 0.8, *rateRepo.syncedRates[99], 1e-12)

	logs, err := client.PaymentAuditLog.Query().All(ctx)
	require.NoError(t, err)
	require.Len(t, logs, 1)
	require.Equal(t, "PAID_USER_RATE_APPLIED", logs[0].Action)
}

func TestApplyPaidUserRateForBalanceOrder_DisabledNoop(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	rateRepo := &paidUserRateRepoStub{}
	svc := &PaymentService{
		entClient:         client,
		configService:     NewPaymentConfigService(client, &paymentConfigSettingRepoStub{values: map[string]string{}}, nil, nil),
		userGroupRateRepo: rateRepo,
	}

	err := svc.applyPaidUserRateForBalanceOrder(ctx, &dbent.PaymentOrder{
		ID:        124,
		UserID:    42,
		OrderType: payment.OrderTypeBalance,
	})
	require.NoError(t, err)
	require.Nil(t, rateRepo.syncedRates)
}

func TestBackfillPaidUserRates_AppliesRulesToHistoricalCompletedBalanceUsers(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	groupA := createPaidUserRateTestGroup(t, ctx, client, "paid-rate-group-a", StatusActive)
	groupB := createPaidUserRateTestGroup(t, ctx, client, "paid-rate-group-b", StatusActive)
	completedBalanceUser := createPaidUserRateTestUser(t, ctx, client, "completed-balance")
	anotherCompletedBalanceUser := createPaidUserRateTestUser(t, ctx, client, "another-completed-balance")
	pendingBalanceUser := createPaidUserRateTestUser(t, ctx, client, "pending-balance")
	completedSubscriptionUser := createPaidUserRateTestUser(t, ctx, client, "completed-subscription")

	createPaidUserRateTestOrder(t, ctx, client, completedBalanceUser, payment.OrderTypeBalance, OrderStatusCompleted, "completed-balance-1")
	createPaidUserRateTestOrder(t, ctx, client, completedBalanceUser, payment.OrderTypeBalance, OrderStatusCompleted, "completed-balance-2")
	createPaidUserRateTestOrder(t, ctx, client, anotherCompletedBalanceUser, payment.OrderTypeBalance, OrderStatusCompleted, "completed-balance-3")
	createPaidUserRateTestOrder(t, ctx, client, pendingBalanceUser, payment.OrderTypeBalance, OrderStatusPending, "pending-balance")
	createPaidUserRateTestOrder(t, ctx, client, completedSubscriptionUser, payment.OrderTypeSubscription, OrderStatusCompleted, "completed-subscription")

	rateRepo := &paidUserRateRepoStub{}
	svc := NewPaymentConfigService(client, &paymentConfigSettingRepoStub{values: map[string]string{}}, rateRepo, nil)

	_, err := svc.backfillPaidUserRates(ctx, []PaymentPaidUserRateRule{
		{GroupID: groupA.ID, RateMultiplier: 0.5},
		{GroupID: groupB.ID, RateMultiplier: 0.8},
	})
	require.NoError(t, err)
	require.Len(t, rateRepo.syncCalls, 2)
	require.Contains(t, rateRepo.syncCalls, completedBalanceUser.ID)
	require.Contains(t, rateRepo.syncCalls, anotherCompletedBalanceUser.ID)
	require.NotContains(t, rateRepo.syncCalls, pendingBalanceUser.ID)
	require.NotContains(t, rateRepo.syncCalls, completedSubscriptionUser.ID)
	require.InDelta(t, 0.5, *rateRepo.syncCalls[completedBalanceUser.ID][groupA.ID], 1e-12)
	require.InDelta(t, 0.8, *rateRepo.syncCalls[anotherCompletedBalanceUser.ID][groupB.ID], 1e-12)
}

func TestUpdatePaymentConfig_BackfillsPaidUserRatesWhenRulesSaved(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	group := createPaidUserRateTestGroup(t, ctx, client, "update-config-paid-rate-group", StatusActive)
	user := createPaidUserRateTestUser(t, ctx, client, "update-config-backfill")
	createPaidUserRateTestOrder(t, ctx, client, user, payment.OrderTypeBalance, OrderStatusCompleted, "update-config-backfill")

	settingRepo := &paymentConfigSettingRepoStub{values: map[string]string{}}
	rateRepo := &paidUserRateRepoStub{}
	svc := NewPaymentConfigService(client, settingRepo, rateRepo, nil)

	err := svc.UpdatePaymentConfig(ctx, UpdatePaymentConfigRequest{
		PaidUserRateRules: []PaymentPaidUserRateRule{
			{GroupID: group.ID, RateMultiplier: 0.5},
		},
	})
	require.NoError(t, err)
	require.JSONEq(t, fmt.Sprintf(`[{"group_id":%d,"rate_multiplier":0.5}]`, group.ID), settingRepo.updates[SettingPaidUserRateRules])
	require.JSONEq(t, `{"total_paid_users":1,"assigned_users":1,"rule_count":1,"status":"success","updated_at":"ignored"}`, maskPaidUserRateBackfillUpdatedAt(t, settingRepo.values[SettingPaidUserRateBackfill]))
	require.Contains(t, rateRepo.syncCalls, user.ID)
	require.InDelta(t, 0.5, *rateRepo.syncCalls[user.ID][group.ID], 1e-12)
}

func TestUpdatePaymentConfig_RejectsInactivePaidUserRateGroup(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	group := createPaidUserRateTestGroup(t, ctx, client, "inactive-paid-rate-group", StatusDisabled)

	svc := NewPaymentConfigService(client, &paymentConfigSettingRepoStub{values: map[string]string{}}, &paidUserRateRepoStub{}, nil)
	err := svc.UpdatePaymentConfig(ctx, UpdatePaymentConfigRequest{
		PaidUserRateRules: []PaymentPaidUserRateRule{
			{GroupID: group.ID, RateMultiplier: 0.5},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "active group")
}

func maskPaidUserRateBackfillUpdatedAt(t *testing.T, raw string) string {
	t.Helper()
	var status PaymentPaidUserRateBackfillStatus
	require.NoError(t, json.Unmarshal([]byte(raw), &status))
	require.NotEmpty(t, status.UpdatedAt)
	status.UpdatedAt = "ignored"
	data, err := json.Marshal(status)
	require.NoError(t, err)
	return string(data)
}

func createPaidUserRateTestGroup(t *testing.T, ctx context.Context, client *dbent.Client, name string, status string) *dbent.Group {
	t.Helper()

	group, err := client.Group.Create().
		SetName(name).
		SetStatus(status).
		SetPlatform(PlatformAnthropic).
		SetSubscriptionType(SubscriptionTypeStandard).
		Save(ctx)
	require.NoError(t, err)
	return group
}

func createPaidUserRateTestUser(t *testing.T, ctx context.Context, client *dbent.Client, suffix string) *dbent.User {
	t.Helper()

	user, err := client.User.Create().
		SetEmail(suffix + "@example.com").
		SetPasswordHash("hash").
		SetUsername(suffix + "-user").
		Save(ctx)
	require.NoError(t, err)
	return user
}

func createPaidUserRateTestOrder(t *testing.T, ctx context.Context, client *dbent.Client, user *dbent.User, orderType string, status string, suffix string) {
	t.Helper()

	now := time.Now()
	create := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(88).
		SetPayAmount(88).
		SetFeeRate(0).
		SetRechargeCode("PAID-RATE-" + suffix).
		SetOutTradeNo("sub2_paid_rate_" + suffix).
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("trade-" + suffix).
		SetOrderType(orderType).
		SetStatus(status).
		SetExpiresAt(now.Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com")
	if status == OrderStatusCompleted {
		create.SetPaidAt(now).SetCompletedAt(now)
	}
	_, err := create.Save(ctx)
	require.NoError(t, err)
}
