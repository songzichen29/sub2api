//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

// resetQuotaUserSubRepoStub 支持 GetByID、ResetUsageWindows，
// 其余方法继承 userSubRepoNoop（panic）。
type resetQuotaUserSubRepoStub struct {
	userSubRepoNoop

	sub *UserSubscription

	activateDailyStart    *time.Time
	activateWeeklyStart   *time.Time
	activateMonthlyStart  *time.Time
	lastResetDailyStart   *time.Time
	lastResetWeeklyStart  *time.Time
	lastResetMonthlyStart *time.Time
	resetDailyCalled      bool
	resetWeeklyCalled     bool
	resetMonthlyCalled    bool
	resetDailyErr         error
	resetWeeklyErr        error
	resetMonthlyErr       error
}

func (r *resetQuotaUserSubRepoStub) GetByID(_ context.Context, id int64) (*UserSubscription, error) {
	if r.sub == nil || r.sub.ID != id {
		return nil, ErrSubscriptionNotFound
	}
	cp := *r.sub
	return &cp, nil
}

func (r *resetQuotaUserSubRepoStub) ResetUsageWindows(_ context.Context, _ int64, resetDaily, resetWeekly, resetMonthly bool, dailyStart, weeklyStart, monthlyStart time.Time) error {
	r.resetDailyCalled = resetDaily
	r.resetWeeklyCalled = resetWeekly
	r.resetMonthlyCalled = resetMonthly
	r.dailyStart = dailyStart
	r.periodicStart = periodicStart
	if resetDaily && r.resetDailyErr != nil {
		return r.resetDailyErr
	}
	if resetWeekly && r.resetWeeklyErr != nil {
		return r.resetWeeklyErr
	}
	if resetMonthly && r.resetMonthlyErr != nil {
		return r.resetMonthlyErr
	}
	if r.sub == nil {
		return nil
	}
	if resetDaily {
		r.sub.DailyUsageUSD = 0
		r.sub.DailyWindowStart = &dailyStart
		r.lastResetDailyStart = &dailyStart
	}
	if resetWeekly {
		r.sub.WeeklyUsageUSD = 0
		r.sub.WeeklyWindowStart = &weeklyStart
		r.lastResetWeeklyStart = &weeklyStart
	}
	if resetMonthly {
		r.sub.MonthlyUsageUSD = 0
		r.sub.MonthlyWindowStart = &monthlyStart
		r.lastResetMonthlyStart = &monthlyStart
	}
	return nil
}

func (r *resetQuotaUserSubRepoStub) ResetDailyUsage(_ context.Context, _ int64, _ *time.Time, windowStart time.Time) error {
	r.resetDailyCalled = true
	r.lastResetDailyStart = &windowStart
	if r.resetDailyErr == nil && r.sub != nil {
		r.sub.DailyUsageUSD = 0
		r.sub.DailyWindowStart = &windowStart
	}
	return r.resetDailyErr
}

func (r *resetQuotaUserSubRepoStub) ResetWeeklyUsage(_ context.Context, _ int64, _ *time.Time, windowStart time.Time) error {
	r.resetWeeklyCalled = true
	r.lastResetWeeklyStart = &windowStart
	if r.sub != nil {
		r.sub.WeeklyWindowStart = &windowStart
		r.sub.WeeklyUsageUSD = 0
	}
	return r.resetWeeklyErr
}

func (r *resetQuotaUserSubRepoStub) ResetMonthlyUsage(_ context.Context, _ int64, _ *time.Time, windowStart time.Time) error {
	r.resetMonthlyCalled = true
	r.lastResetMonthlyStart = &windowStart
	if r.sub != nil {
		r.sub.MonthlyWindowStart = &windowStart
		r.sub.MonthlyUsageUSD = 0
	}
	return r.resetMonthlyErr
}

func (r *resetQuotaUserSubRepoStub) ActivateWindows(_ context.Context, _ int64, dailyStart, weeklyStart, monthlyStart time.Time) error {
	r.activateDailyStart = &dailyStart
	r.activateWeeklyStart = &weeklyStart
	r.activateMonthlyStart = &monthlyStart
	if r.sub != nil {
		r.sub.DailyWindowStart = &dailyStart
		r.sub.WeeklyWindowStart = &weeklyStart
		r.sub.MonthlyWindowStart = &monthlyStart
	}
	return nil
}

func newResetQuotaSvc(stub *resetQuotaUserSubRepoStub) *SubscriptionService {
	return NewSubscriptionService(groupRepoNoop{}, stub, nil, nil, nil)
}

// resetQuotaGroupAll 三档限额都配置（30/210/800），上限档 = monthly。
// 适用于「daily/weekly 可重置；monthly 是上限不可重置」的多数测试场景。
func resetQuotaGroupAll() *Group {
	daily := 30.0
	weekly := 210.0
	monthly := 800.0
	return &Group{
		ID:              20,
		DailyLimitUSD:   &daily,
		WeeklyLimitUSD:  &weekly,
		MonthlyLimitUSD: &monthly,
	}
}

// newAdminSub 构造一个 AssignedBy 已设置的管理员分配订阅，默认 source=admin 可重置。
func newAdminSub(id int64) *UserSubscription {
	return &UserSubscription{
		ID:      id,
		UserID:  10,
		GroupID: 20,
		Source:  domain.SubscriptionSourceAdmin,
		Group:   resetQuotaGroupAll(),
	}
}

func TestAdminResetQuota_ResetBoth(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{sub: newAdminSub(1)}
	svc := newResetQuotaSvc(stub)
	resetAt := time.Date(2026, 7, 1, 10, 37, 42, 123, time.UTC)
	svc.now = func() time.Time { return resetAt }

	result, err := svc.AdminResetQuota(context.Background(), 1, true, true, false)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, stub.resetDailyCalled, "应调用 ResetDailyUsage")
	require.True(t, stub.resetWeeklyCalled, "应调用 ResetWeeklyUsage")
	require.False(t, stub.resetMonthlyCalled, "不应调用 ResetMonthlyUsage")
	// 手动重置后日窗口锚定当天 0 点（保持 0 点刷新节奏），周窗口锚定重置时刻。
	require.Equal(t, timezone.StartOfDay(resetAt), stub.dailyStart)
	require.Equal(t, resetAt, stub.periodicStart)
	require.Equal(t, timezone.StartOfDay(resetAt), *result.DailyWindowStart)
	require.Equal(t, resetAt, *result.WeeklyWindowStart)
}

func TestAdminResetQuota_ResetDailyOnly(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{sub: newAdminSub(2)}
	svc := newResetQuotaSvc(stub)

	result, err := svc.AdminResetQuota(context.Background(), 2, true, false, false)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, stub.resetDailyCalled, "应调用 ResetDailyUsage")
	require.False(t, stub.resetWeeklyCalled, "不应调用 ResetWeeklyUsage")
	require.False(t, stub.resetMonthlyCalled, "不应调用 ResetMonthlyUsage")
}

func TestAdminResetQuota_ResetWeeklyOnly(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{sub: newAdminSub(3)}
	svc := newResetQuotaSvc(stub)

	result, err := svc.AdminResetQuota(context.Background(), 3, false, true, false)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, stub.resetDailyCalled, "不应调用 ResetDailyUsage")
	require.True(t, stub.resetWeeklyCalled, "应调用 ResetWeeklyUsage")
	require.False(t, stub.resetMonthlyCalled, "不应调用 ResetMonthlyUsage")
}

func TestAdminResetQuota_BothFalseReturnsError(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{sub: newAdminSub(7)}
	svc := newResetQuotaSvc(stub)

	_, err := svc.AdminResetQuota(context.Background(), 7, false, false, false)

	require.ErrorIs(t, err, ErrInvalidInput)
	require.False(t, stub.resetDailyCalled)
	require.False(t, stub.resetWeeklyCalled)
	require.False(t, stub.resetMonthlyCalled)
}

func TestAdminResetQuota_SubscriptionNotFound(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{sub: nil}
	svc := newResetQuotaSvc(stub)

	_, err := svc.AdminResetQuota(context.Background(), 999, true, true, true)

	require.ErrorIs(t, err, ErrSubscriptionNotFound)
	require.False(t, stub.resetDailyCalled)
	require.False(t, stub.resetWeeklyCalled)
	require.False(t, stub.resetMonthlyCalled)
}

func TestAdminResetQuota_ResetDailyUsageError(t *testing.T) {
	dbErr := errors.New("db error")
	stub := &resetQuotaUserSubRepoStub{
		sub:           newAdminSub(4),
		resetDailyErr: dbErr,
	}
	svc := newResetQuotaSvc(stub)

	_, err := svc.AdminResetQuota(context.Background(), 4, true, true, false)

	require.ErrorIs(t, err, dbErr)
	require.True(t, stub.resetDailyCalled)
	require.True(t, stub.resetWeeklyCalled, "原子重置应在一次调用中提交所选窗口")
}

func TestAdminResetQuota_ResetWeeklyUsageError(t *testing.T) {
	dbErr := errors.New("db error")
	stub := &resetQuotaUserSubRepoStub{
		sub:            newAdminSub(5),
		resetWeeklyErr: dbErr,
	}
	svc := newResetQuotaSvc(stub)

	_, err := svc.AdminResetQuota(context.Background(), 5, false, true, false)

	require.ErrorIs(t, err, dbErr)
	require.True(t, stub.resetWeeklyCalled)
}

// TestAdminResetQuota_RejectMonthlyOnUpperBound 用三档全配的订阅尝试 reset monthly，
// monthly 是上限，必须被业务规则拒绝。
func TestAdminResetQuota_RejectMonthlyOnUpperBound(t *testing.T) {
	stub := &resetQuotaUserSubRepoStub{sub: newAdminSub(8)}
	svc := newResetQuotaSvc(stub)

	_, err := svc.AdminResetQuota(context.Background(), 8, false, false, true)

	require.ErrorIs(t, err, ErrInvalidResetTarget)
	require.False(t, stub.resetMonthlyCalled, "上限档不应实际触发 ResetMonthlyUsage")
}

// TestAdminResetQuota_OnlyDailyConfigured_RejectAll 仅配置 daily 时 daily 即上限，
// reset daily 也应被拒。
func TestAdminResetQuota_OnlyDailyConfigured_RejectAll(t *testing.T) {
	daily := 30.0
	sub := &UserSubscription{
		ID:      11,
		UserID:  10,
		GroupID: 20,
		Source:  domain.SubscriptionSourceAdmin,
		Group: &Group{
			ID:            20,
			DailyLimitUSD: &daily,
		},
	}
	stub := &resetQuotaUserSubRepoStub{sub: sub}
	svc := newResetQuotaSvc(stub)

	_, err := svc.AdminResetQuota(context.Background(), 11, true, false, false)

	require.ErrorIs(t, err, ErrInvalidResetTarget)
	require.False(t, stub.resetDailyCalled)
}

// TestAdminResetQuota_PaymentSourceLocked 付费订阅永远不可重置，即便档位配置允许。
func TestAdminResetQuota_PaymentSourceLocked(t *testing.T) {
	sub := newAdminSub(12)
	sub.Source = domain.SubscriptionSourcePayment
	stub := &resetQuotaUserSubRepoStub{sub: sub}
	svc := newResetQuotaSvc(stub)

	_, err := svc.AdminResetQuota(context.Background(), 12, true, false, false)

	require.ErrorIs(t, err, ErrPaidSubscriptionImmutable)
	require.False(t, stub.resetDailyCalled)
}

// TestAdminResetQuota_NoLimitsConfigured 没配置任何限额时拒绝重置。
func TestAdminResetQuota_NoLimitsConfigured(t *testing.T) {
	sub := &UserSubscription{
		ID:      13,
		UserID:  10,
		GroupID: 20,
		Source:  domain.SubscriptionSourceAdmin,
		Group:   &Group{ID: 20},
	}
	stub := &resetQuotaUserSubRepoStub{sub: sub}
	svc := newResetQuotaSvc(stub)

	_, err := svc.AdminResetQuota(context.Background(), 13, true, true, true)

	require.ErrorIs(t, err, ErrNoLimitsConfigured)
}

// TestAdminResetQuota_RejectUnconfiguredWindow 重置 group 中未配置的档位时拒绝。
// 例如 group 只配 daily+monthly，请求 reset weekly 应被拒。
func TestAdminResetQuota_RejectUnconfiguredWindow(t *testing.T) {
	daily := 30.0
	monthly := 800.0
	sub := &UserSubscription{
		ID:      14,
		UserID:  10,
		GroupID: 20,
		Source:  domain.SubscriptionSourceAdmin,
		Group: &Group{
			ID:              20,
			DailyLimitUSD:   &daily,
			MonthlyLimitUSD: &monthly,
		},
	}
	stub := &resetQuotaUserSubRepoStub{sub: sub}
	svc := newResetQuotaSvc(stub)

	_, err := svc.AdminResetQuota(context.Background(), 14, false, true, false)

	require.ErrorIs(t, err, ErrInvalidResetTarget)
	require.False(t, stub.resetWeeklyCalled)
}

func TestAdminResetQuota_ReturnsRefreshedSub(t *testing.T) {
	sub := newAdminSub(6)
	sub.DailyUsageUSD = 99.9
	stub := &resetQuotaUserSubRepoStub{sub: sub}

	svc := newResetQuotaSvc(stub)
	result, err := svc.AdminResetQuota(context.Background(), 6, true, false, false)

	require.NoError(t, err)
	// ResetUsageWindows stub 会将 sub.DailyUsageUSD 归零，
	// 服务应返回第二次 GetByID 的刷新值而非初始的 99.9
	require.Equal(t, float64(0), result.DailyUsageUSD, "返回的订阅应反映已归零的用量")
	require.True(t, stub.resetDailyCalled)
}

func TestCheckAndActivateWindow_UsesSubscriptionAnchoredRollingWindows(t *testing.T) {
	startsAt := time.Now().Add(-50 * time.Hour)
	sub := newAdminSub(100)
	sub.StartsAt = startsAt
	sub.DailyWindowStart = nil
	sub.WeeklyWindowStart = nil
	sub.MonthlyWindowStart = nil
	stub := &resetQuotaUserSubRepoStub{sub: sub}
	svc := newResetQuotaSvc(stub)

	err := svc.CheckAndActivateWindow(context.Background(), sub)

	require.NoError(t, err)
	require.NotNil(t, stub.activateDailyStart)
	require.NotNil(t, stub.activateWeeklyStart)
	require.NotNil(t, stub.activateMonthlyStart)

	expectedDaily := startsAt.Add(48 * time.Hour)
	expectedWeekly := startsAt
	expectedMonthly := startsAt
	require.WithinDuration(t, expectedDaily, *stub.activateDailyStart, 2*time.Second)
	require.WithinDuration(t, expectedWeekly, *stub.activateWeeklyStart, 2*time.Second)
	require.WithinDuration(t, expectedMonthly, *stub.activateMonthlyStart, 2*time.Second)
}

func TestPaidResetDailyQuota_KeepsRollingWindowBoundary(t *testing.T) {
	price := 9.9
	limit := 20.0
	startsAt := time.Now().Add(-50 * time.Hour)
	currentDailyWindowStart := startsAt.Add(48 * time.Hour)
	sub := &UserSubscription{
		ID:               101,
		UserID:           10,
		GroupID:          20,
		StartsAt:         startsAt,
		ExpiresAt:        time.Now().Add(24 * time.Hour),
		Status:           SubscriptionStatusActive,
		Source:           domain.SubscriptionSourcePayment,
		DailyUsageUSD:    18.5,
		DailyWindowStart: &currentDailyWindowStart,
		Group: &Group{
			ID:                   20,
			SubscriptionType:     "subscription",
			DailyLimitUSD:        &limit,
			DailyLimitResetPrice: &price,
		},
	}
	stub := &resetQuotaUserSubRepoStub{sub: sub}
	svc := newResetQuotaSvc(stub)

	result, err := svc.PaidResetDailyQuota(context.Background(), sub.UserID, sub.ID)

	require.NoError(t, err)
	require.True(t, stub.resetDailyCalled)
	require.NotNil(t, sub.DailyWindowStart)
	require.WithinDuration(t, currentDailyWindowStart, *sub.DailyWindowStart, 2*time.Second)
	require.Equal(t, 0.0, result.DailyUsageUSD)
}

func TestFulfillPaidDailyQuotaReset_IgnoresLatestGroupResetSwitch(t *testing.T) {
	startsAt := time.Now().Add(-50 * time.Hour)
	currentDailyWindowStart := startsAt.Add(48 * time.Hour)
	sub := &UserSubscription{
		ID:               103,
		UserID:           10,
		GroupID:          20,
		StartsAt:         startsAt,
		ExpiresAt:        time.Now().Add(24 * time.Hour),
		Status:           SubscriptionStatusActive,
		DailyUsageUSD:    18.5,
		DailyWindowStart: &currentDailyWindowStart,
		Group: &Group{
			ID:               20,
			SubscriptionType: "subscription",
			// 注意：这里不提供 DailyLimitResetPrice，模拟“管理员下单后把按钮关闭”
		},
	}
	stub := &resetQuotaUserSubRepoStub{sub: sub}
	svc := newResetQuotaSvc(stub)

	result, err := svc.FulfillPaidDailyQuotaReset(context.Background(), sub.UserID, sub.ID)

	require.NoError(t, err)
	require.True(t, stub.resetDailyCalled)
	require.NotNil(t, sub.DailyWindowStart)
	require.WithinDuration(t, currentDailyWindowStart, *sub.DailyWindowStart, 2*time.Second)
	require.Equal(t, 0.0, result.DailyUsageUSD)
}

func TestAdminResetQuota_UsesAnchoredDailyWindow(t *testing.T) {
	startsAt := time.Now().Add(-(8*24 + 6) * time.Hour)
	sub := newAdminSub(102)
	weekly := 210.0
	sub.Group.WeeklyLimitUSD = &weekly
	sub.Group.MonthlyLimitUSD = nil
	sub.StartsAt = startsAt
	sub.DailyUsageUSD = 1
	sub.WeeklyUsageUSD = 2
	sub.MonthlyUsageUSD = 3
	stub := &resetQuotaUserSubRepoStub{sub: sub}
	svc := newResetQuotaSvc(stub)

	_, err := svc.AdminResetQuota(context.Background(), sub.ID, true, false, false)

	require.NoError(t, err)
	require.NotNil(t, stub.lastResetDailyStart)
	require.WithinDuration(t, sub.CurrentDailyWindowStart(time.Now()), *stub.lastResetDailyStart, 2*time.Second)
}

func TestCheckAndResetWindows_DailyOverdraftSkipsPeriodUsageResets(t *testing.T) {
	startsAt := time.Now().Add(-(35 * 24 * time.Hour))
	dailyStart := startsAt
	weeklyStart := startsAt
	monthlyStart := startsAt
	sub := newAdminSub(103)
	sub.StartsAt = startsAt
	sub.ExpiresAt = time.Now().Add(24 * time.Hour)
	sub.DailyWindowStart = &dailyStart
	sub.WeeklyWindowStart = &weeklyStart
	sub.MonthlyWindowStart = &monthlyStart
	sub.DailyUsageUSD = 1
	sub.WeeklyUsageUSD = 2
	sub.MonthlyUsageUSD = 3
	sub.AllowDailyOverdraft = true
	sub.Group.AllowDailyOverdraft = true
	sub.Group.SubscriptionType = SubscriptionTypeSubscription

	stub := &resetQuotaUserSubRepoStub{sub: sub}
	svc := newResetQuotaSvc(stub)

	err := svc.CheckAndResetWindows(context.Background(), sub)

	require.NoError(t, err)
	require.True(t, stub.resetDailyCalled)
	require.False(t, stub.resetWeeklyCalled)
	require.False(t, stub.resetMonthlyCalled)
	require.Equal(t, 2.0, sub.WeeklyUsageUSD)
	require.Equal(t, 3.0, sub.MonthlyUsageUSD)
}
