package service

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type subscriptionEntRepo struct {
	client *dbent.Client
}

func (r *subscriptionEntRepo) clientFromContext(ctx context.Context) *dbent.Client {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return tx.Client()
	}
	return r.client
}

func (r *subscriptionEntRepo) Create(ctx context.Context, sub *UserSubscription) error {
	created, err := r.clientFromContext(ctx).UserSubscription.Create().
		SetUserID(sub.UserID).
		SetGroupID(sub.GroupID).
		SetStartsAt(sub.StartsAt).
		SetExpiresAt(sub.ExpiresAt).
		SetStatus(sub.Status).
		SetNillableDailyWindowStart(sub.DailyWindowStart).
		SetNillableWeeklyWindowStart(sub.WeeklyWindowStart).
		SetNillableMonthlyWindowStart(sub.MonthlyWindowStart).
		SetDailyUsageUsd(sub.DailyUsageUSD).
		SetWeeklyUsageUsd(sub.WeeklyUsageUSD).
		SetMonthlyUsageUsd(sub.MonthlyUsageUSD).
		SetAllowDailyOverdraft(sub.AllowDailyOverdraft).
		SetNotes(sub.Notes).
		SetSource(sub.Source).
		Save(ctx)
	if err != nil {
		return err
	}
	sub.ID = created.ID
	return nil
}

func (r *subscriptionEntRepo) GetByID(ctx context.Context, id int64) (*UserSubscription, error) {
	m, err := r.clientFromContext(ctx).UserSubscription.Query().Where(usersubscription.IDEQ(id)).Only(ctx)
	if err != nil {
		return nil, ErrSubscriptionNotFound
	}
	return entUserSubscriptionToService(m), nil
}

func (r *subscriptionEntRepo) GetByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*UserSubscription, error) {
	m, err := r.clientFromContext(ctx).UserSubscription.Query().
		Where(usersubscription.UserIDEQ(userID), usersubscription.GroupIDEQ(groupID)).
		Only(ctx)
	if err != nil {
		return nil, ErrSubscriptionNotFound
	}
	return entUserSubscriptionToService(m), nil
}

func (r *subscriptionEntRepo) GetActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*UserSubscription, error) {
	m, err := r.clientFromContext(ctx).UserSubscription.Query().
		Where(
			usersubscription.UserIDEQ(userID),
			usersubscription.GroupIDEQ(groupID),
			usersubscription.StatusEQ(SubscriptionStatusActive),
			usersubscription.ExpiresAtGT(time.Now()),
		).
		Only(ctx)
	if err != nil {
		return nil, ErrSubscriptionNotFound
	}
	return entUserSubscriptionToService(m), nil
}

func (r *subscriptionEntRepo) Update(ctx context.Context, sub *UserSubscription) error {
	builder := r.clientFromContext(ctx).UserSubscription.UpdateOneID(sub.ID).
		SetStartsAt(sub.StartsAt).
		SetExpiresAt(sub.ExpiresAt).
		SetStatus(sub.Status).
		SetDailyUsageUsd(sub.DailyUsageUSD).
		SetWeeklyUsageUsd(sub.WeeklyUsageUSD).
		SetMonthlyUsageUsd(sub.MonthlyUsageUSD).
		SetAllowDailyOverdraft(sub.AllowDailyOverdraft).
		SetNotes(sub.Notes).
		SetSource(sub.Source)
	if sub.DailyWindowStart != nil {
		builder.SetDailyWindowStart(*sub.DailyWindowStart)
	} else {
		builder.ClearDailyWindowStart()
	}
	if sub.WeeklyWindowStart != nil {
		builder.SetWeeklyWindowStart(*sub.WeeklyWindowStart)
	} else {
		builder.ClearWeeklyWindowStart()
	}
	if sub.MonthlyWindowStart != nil {
		builder.SetMonthlyWindowStart(*sub.MonthlyWindowStart)
	} else {
		builder.ClearMonthlyWindowStart()
	}
	_, err := builder.Save(ctx)
	return err
}

func (r *subscriptionEntRepo) Delete(context.Context, int64) error { panic("unexpected Delete") }
func (r *subscriptionEntRepo) ListByUserID(context.Context, int64) ([]UserSubscription, error) {
	panic("unexpected ListByUserID")
}
func (r *subscriptionEntRepo) ListActiveByUserID(context.Context, int64) ([]UserSubscription, error) {
	panic("unexpected ListActiveByUserID")
}
func (r *subscriptionEntRepo) ListByGroupID(context.Context, int64, pagination.PaginationParams) ([]UserSubscription, *pagination.PaginationResult, error) {
	panic("unexpected ListByGroupID")
}
func (r *subscriptionEntRepo) List(context.Context, pagination.PaginationParams, *int64, *int64, string, string, string, string) ([]UserSubscription, *pagination.PaginationResult, error) {
	panic("unexpected List")
}
func (r *subscriptionEntRepo) ExistsByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (bool, error) {
	_, err := r.GetByUserIDAndGroupID(ctx, userID, groupID)
	return err == nil, nil
}
func (r *subscriptionEntRepo) ExtendExpiry(ctx context.Context, subscriptionID int64, newExpiresAt time.Time) error {
	_, err := r.clientFromContext(ctx).UserSubscription.UpdateOneID(subscriptionID).SetExpiresAt(newExpiresAt).Save(ctx)
	return err
}
func (r *subscriptionEntRepo) UpdateStatus(ctx context.Context, subscriptionID int64, status string) error {
	_, err := r.clientFromContext(ctx).UserSubscription.UpdateOneID(subscriptionID).SetStatus(status).Save(ctx)
	return err
}
func (r *subscriptionEntRepo) UpdateNotes(ctx context.Context, subscriptionID int64, notes string) error {
	_, err := r.clientFromContext(ctx).UserSubscription.UpdateOneID(subscriptionID).SetNotes(notes).Save(ctx)
	return err
}
func (r *subscriptionEntRepo) UpdateDailyOverdraft(ctx context.Context, subscriptionID int64, enabled bool) error {
	_, err := r.clientFromContext(ctx).UserSubscription.UpdateOneID(subscriptionID).SetAllowDailyOverdraft(enabled).Save(ctx)
	return err
}
func (r *subscriptionEntRepo) ActivateWindows(context.Context, int64, time.Time, time.Time, time.Time) error {
	panic("unexpected ActivateWindows")
}
func (r *subscriptionEntRepo) ResetDailyUsage(ctx context.Context, id int64, newWindowStart time.Time) error {
	_, err := r.clientFromContext(ctx).UserSubscription.UpdateOneID(id).
		SetDailyUsageUsd(0).
		SetDailyWindowStart(newWindowStart).
		Save(ctx)
	return err
}
func (r *subscriptionEntRepo) ResetWeeklyUsage(context.Context, int64, time.Time) error {
	panic("unexpected ResetWeeklyUsage")
}
func (r *subscriptionEntRepo) ResetMonthlyUsage(context.Context, int64, time.Time) error {
	panic("unexpected ResetMonthlyUsage")
}
func (r *subscriptionEntRepo) IncrementUsage(context.Context, int64, float64) error {
	panic("unexpected IncrementUsage")
}
func (r *subscriptionEntRepo) GetLatestUsedAtBySubscriptionIDs(context.Context, []int64) (map[int64]*time.Time, error) {
	return map[int64]*time.Time{}, nil
}
func (r *subscriptionEntRepo) BatchUpdateExpiredStatus(context.Context) (int64, error) {
	panic("unexpected BatchUpdateExpiredStatus")
}

func entUserSubscriptionToService(m *dbent.UserSubscription) *UserSubscription {
	if m == nil {
		return nil
	}
	notes := ""
	if m.Notes != nil {
		notes = *m.Notes
	}
	return &UserSubscription{
		ID:                  m.ID,
		UserID:              m.UserID,
		GroupID:             m.GroupID,
		StartsAt:            m.StartsAt,
		ExpiresAt:           m.ExpiresAt,
		Status:              m.Status,
		DailyWindowStart:    m.DailyWindowStart,
		WeeklyWindowStart:   m.WeeklyWindowStart,
		MonthlyWindowStart:  m.MonthlyWindowStart,
		DailyUsageUSD:       m.DailyUsageUsd,
		WeeklyUsageUSD:      m.WeeklyUsageUsd,
		MonthlyUsageUSD:     m.MonthlyUsageUsd,
		AllowDailyOverdraft: m.AllowDailyOverdraft,
		Notes:               notes,
		Source:              m.Source,
		CreatedAt:           m.CreatedAt,
		UpdatedAt:           m.UpdatedAt,
	}
}

func TestAssignOrExtendSubscription_ExpiredSubscriptionResetsStartAnchor(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("sub-reactivate@example.com").
		SetPasswordHash("hash").
		SetUsername("sub-reactivate-user").
		Save(ctx)
	require.NoError(t, err)

	group, err := client.Group.Create().
		SetName("sub-reactivate-group").
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SetRateMultiplier(1.0).
		Save(ctx)
	require.NoError(t, err)

	repo := &subscriptionEntRepo{client: client}
	oldStart := time.Now().Add(-20 * 24 * time.Hour)
	expiredAt := time.Now().Add(-48 * time.Hour)
	dailyStart := oldStart
	weeklyStart := oldStart
	monthlyStart := oldStart

	err = repo.Create(ctx, &UserSubscription{
		UserID:             user.ID,
		GroupID:            group.ID,
		StartsAt:           oldStart,
		ExpiresAt:          expiredAt,
		Status:             SubscriptionStatusExpired,
		DailyWindowStart:   &dailyStart,
		WeeklyWindowStart:  &weeklyStart,
		MonthlyWindowStart: &monthlyStart,
		DailyUsageUSD:      9,
		WeeklyUsageUSD:     99,
		MonthlyUsageUSD:    199,
		Notes:              "old-note",
		Source:             "payment",
	})
	require.NoError(t, err)

	svc := NewSubscriptionService(&subscriptionGroupRepoStub{
		group: &Group{ID: group.ID, SubscriptionType: SubscriptionTypeSubscription},
	}, repo, nil, client, nil)

	before := time.Now()
	sub, reused, err := svc.AssignOrExtendSubscription(ctx, &AssignSubscriptionInput{
		UserID:       user.ID,
		GroupID:      group.ID,
		ValidityDays: 7,
		Notes:        "renew-note",
		Source:       "payment",
	})
	after := time.Now()

	require.NoError(t, err)
	require.True(t, reused)
	require.NotNil(t, sub)
	require.True(t, sub.StartsAt.After(expiredAt))
	require.WithinDuration(t, before, sub.StartsAt, 3*time.Second)
	require.True(t, sub.ExpiresAt.After(after.AddDate(0, 0, 6)))
	require.Equal(t, SubscriptionStatusActive, sub.Status)
	require.Nil(t, sub.DailyWindowStart)
	require.Nil(t, sub.WeeklyWindowStart)
	require.Nil(t, sub.MonthlyWindowStart)
	require.Equal(t, float64(0), sub.DailyUsageUSD)
	require.Equal(t, float64(0), sub.WeeklyUsageUSD)
	require.Equal(t, float64(0), sub.MonthlyUsageUSD)
	require.Contains(t, sub.Notes, "old-note")
	require.Contains(t, sub.Notes, "renew-note")
}

func TestAssignOrExtendSubscription_PaidRenewalResetsDailyUsage(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("sub-paid-renew-reset@example.com").
		SetPasswordHash("hash").
		SetUsername("sub-paid-renew-reset-user").
		Save(ctx)
	require.NoError(t, err)

	dailyLimit := 80.0
	group, err := client.Group.Create().
		SetName("sub-paid-renew-reset-group").
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SetDailyLimitUsd(dailyLimit).
		SetRateMultiplier(1.0).
		Save(ctx)
	require.NoError(t, err)

	repo := &subscriptionEntRepo{client: client}
	startsAt := time.Now().Add(-2 * time.Hour)
	expiresAt := time.Now().Add(22 * time.Hour)
	dailyStart := startsAt
	err = repo.Create(ctx, &UserSubscription{
		UserID:           user.ID,
		GroupID:          group.ID,
		StartsAt:         startsAt,
		ExpiresAt:        expiresAt,
		Status:           SubscriptionStatusActive,
		DailyWindowStart: &dailyStart,
		DailyUsageUSD:    dailyLimit,
		Source:           "payment",
	})
	require.NoError(t, err)

	svc := NewSubscriptionService(&subscriptionGroupRepoStub{
		group: &Group{
			ID:               group.ID,
			SubscriptionType: SubscriptionTypeSubscription,
			DailyLimitUSD:    &dailyLimit,
		},
	}, repo, nil, client, nil)

	before := time.Now()
	sub, reused, err := svc.AssignOrExtendSubscription(ctx, &AssignSubscriptionInput{
		UserID:       user.ID,
		GroupID:      group.ID,
		ValidityDays: 1,
		Notes:        "renew-note",
		Source:       "payment",
	})
	after := time.Now()

	require.NoError(t, err)
	require.True(t, reused)
	require.NotNil(t, sub)
	require.Equal(t, 0.0, sub.DailyUsageUSD)
	require.NotNil(t, sub.DailyWindowStart)
	require.WithinDuration(t, before, *sub.DailyWindowStart, 3*time.Second)
	require.True(t, sub.ExpiresAt.After(expiresAt))
	sub.Group = &Group{ID: group.ID, SubscriptionType: SubscriptionTypeSubscription, DailyLimitUSD: &dailyLimit}
	_, err = svc.ValidateAndCheckLimits(context.Background(), sub, sub.Group)
	require.NoError(t, err)
	require.WithinDuration(t, after.Add(24*time.Hour), *sub.DailyResetTime(), 3*time.Second)
}

func TestSubscriptionService_ValidateAndCheckLimits_DailyOverdraftUsesPeriodPool(t *testing.T) {
	daily := 80.0
	weekly := 560.0
	now := time.Now()
	sub := &UserSubscription{
		StartsAt:            now.Add(-time.Hour),
		ExpiresAt:           now.Add(24 * time.Hour),
		Status:              SubscriptionStatusActive,
		DailyUsageUSD:       80,
		WeeklyUsageUSD:      120,
		AllowDailyOverdraft: true,
	}
	strictGroup := &Group{SubscriptionType: SubscriptionTypeSubscription, DailyLimitUSD: &daily, WeeklyLimitUSD: &weekly}
	overdraftGroup := &Group{SubscriptionType: SubscriptionTypeSubscription, DailyLimitUSD: &daily, WeeklyLimitUSD: &weekly, AllowDailyOverdraft: true}
	svc := &SubscriptionService{}

	_, err := svc.ValidateAndCheckLimits(context.Background(), sub, strictGroup)
	require.ErrorIs(t, err, ErrDailyLimitExceeded)

	_, err = svc.ValidateAndCheckLimits(context.Background(), sub, overdraftGroup)
	require.NoError(t, err)

	sub.WeeklyUsageUSD = weekly
	_, err = svc.ValidateAndCheckLimits(context.Background(), sub, overdraftGroup)
	require.ErrorIs(t, err, ErrWeeklyLimitExceeded)
}
