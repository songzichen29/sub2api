package service

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/domain"
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
		SetValidityUnit(normalizeSubscriptionValidityUnit(sub.ValidityUnit)).
		SetNillableDailyWindowStart(sub.DailyWindowStart).
		SetNillableWeeklyWindowStart(sub.WeeklyWindowStart).
		SetNillableMonthlyWindowStart(sub.MonthlyWindowStart).
		SetDailyUsageUsd(sub.DailyUsageUSD).
		SetWeeklyUsageUsd(sub.WeeklyUsageUSD).
		SetMonthlyUsageUsd(sub.MonthlyUsageUSD).
		SetAllowDailyOverdraft(sub.AllowDailyOverdraft).
		SetSkipWeekends(sub.SkipWeekends).
		SetNotes(sub.Notes).
		SetSource(sub.Source).
		SetNillableWeekendSkipOriginalExpiresAt(sub.WeekendSkipOriginalExpiresAt).
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

func (r *subscriptionEntRepo) GetByIDIncludeDeleted(ctx context.Context, id int64) (*UserSubscription, error) {
	m, err := r.clientFromContext(ctx).UserSubscription.Query().Where(usersubscription.IDEQ(id)).Only(mixins.SkipSoftDelete(ctx))
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
		SetValidityUnit(normalizeSubscriptionValidityUnit(sub.ValidityUnit)).
		SetDailyUsageUsd(sub.DailyUsageUSD).
		SetWeeklyUsageUsd(sub.WeeklyUsageUSD).
		SetMonthlyUsageUsd(sub.MonthlyUsageUSD).
		SetAllowDailyOverdraft(sub.AllowDailyOverdraft).
		SetSkipWeekends(sub.SkipWeekends).
		SetNotes(sub.Notes).
		SetSource(sub.Source)
	if sub.WeekendSkipOriginalExpiresAt != nil {
		builder.SetWeekendSkipOriginalExpiresAt(*sub.WeekendSkipOriginalExpiresAt)
	} else {
		builder.ClearWeekendSkipOriginalExpiresAt()
	}
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
func (r *subscriptionEntRepo) Restore(ctx context.Context, subscriptionID int64, restoredStatus string) (*UserSubscription, error) {
	_, err := r.clientFromContext(ctx).UserSubscription.UpdateOneID(subscriptionID).
		SetStatus(restoredStatus).
		ClearDeletedAt().
		SetUpdatedAt(time.Now()).
		Save(mixins.SkipSoftDelete(ctx))
	if err != nil {
		return nil, ErrSubscriptionNotFound
	}
	return r.GetByID(ctx, subscriptionID)
}
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
func (r *subscriptionEntRepo) ExistsActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (bool, error) {
	_, err := r.GetActiveByUserIDAndGroupID(ctx, userID, groupID)
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
func (r *subscriptionEntRepo) ResetUsageWindows(context.Context, int64, bool, bool, bool, time.Time, time.Time, time.Time) error {
	panic("unexpected ResetUsageWindows")
}
func (r *subscriptionEntRepo) ResetDailyUsage(ctx context.Context, id int64, _ *time.Time, newWindowStart time.Time) error {
	_, err := r.clientFromContext(ctx).UserSubscription.UpdateOneID(id).
		SetDailyUsageUsd(0).
		SetDailyWindowStart(newWindowStart).
		Save(ctx)
	return err
}
func (r *subscriptionEntRepo) ResetWeeklyUsage(context.Context, int64, *time.Time, time.Time) error {
	panic("unexpected ResetWeeklyUsage")
}
func (r *subscriptionEntRepo) ResetMonthlyUsage(context.Context, int64, *time.Time, time.Time) error {
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
		ID:                           m.ID,
		UserID:                       m.UserID,
		GroupID:                      m.GroupID,
		StartsAt:                     m.StartsAt,
		ExpiresAt:                    m.ExpiresAt,
		Status:                       m.Status,
		ValidityUnit:                 normalizeSubscriptionValidityUnit(m.ValidityUnit),
		DailyWindowStart:             m.DailyWindowStart,
		WeeklyWindowStart:            m.WeeklyWindowStart,
		MonthlyWindowStart:           m.MonthlyWindowStart,
		DailyUsageUSD:                m.DailyUsageUsd,
		WeeklyUsageUSD:               m.WeeklyUsageUsd,
		MonthlyUsageUSD:              m.MonthlyUsageUsd,
		AllowDailyOverdraft:          m.AllowDailyOverdraft,
		SkipWeekends:                 m.SkipWeekends,
		WeekendSkipOriginalExpiresAt: m.WeekendSkipOriginalExpiresAt,
		Notes:                        notes,
		Source:                       m.Source,
		CreatedAt:                    m.CreatedAt,
		UpdatedAt:                    m.UpdatedAt,
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

func TestAssignOrExtendSubscription_FixedExpiryDoesNotShortenActiveSubscription(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("sub-fixed-expiry-active@example.com").
		SetPasswordHash("hash").
		SetUsername("sub-fixed-expiry-active-user").
		Save(ctx)
	require.NoError(t, err)

	group, err := client.Group.Create().
		SetName("sub-fixed-expiry-active-group").
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SetRateMultiplier(1.0).
		Save(ctx)
	require.NoError(t, err)

	repo := &subscriptionEntRepo{client: client}
	now := time.Now()
	originalStart := now.Add(-24 * time.Hour)
	originalExpires := now.Add(72 * time.Hour)

	err = repo.Create(ctx, &UserSubscription{
		UserID:       user.ID,
		GroupID:      group.ID,
		StartsAt:     originalStart,
		ExpiresAt:    originalExpires,
		Status:       SubscriptionStatusActive,
		Notes:        "old-note",
		ValidityUnit: "day",
		Source:       domain.SubscriptionSourcePayment,
	})
	require.NoError(t, err)

	svc := NewSubscriptionService(&subscriptionGroupRepoStub{
		group: &Group{ID: group.ID, SubscriptionType: SubscriptionTypeSubscription},
	}, repo, nil, client, nil)

	earlyStart := now
	earlyExpires := now.Add(24 * time.Hour)
	sub, reused, err := svc.AssignOrExtendSubscription(ctx, &AssignSubscriptionInput{
		UserID:       user.ID,
		GroupID:      group.ID,
		ValidityDays: 1,
		StartsAt:     &earlyStart,
		ExpiresAt:    &earlyExpires,
		Notes:        "early-fixed-order",
		Source:       domain.SubscriptionSourcePayment,
	})
	require.NoError(t, err)
	require.True(t, reused)
	require.WithinDuration(t, originalStart, sub.StartsAt, time.Second)
	require.WithinDuration(t, originalExpires, sub.ExpiresAt, time.Second)
	require.Equal(t, "day", sub.ValidityUnit)
	require.Contains(t, sub.Notes, "early-fixed-order")

	laterStart := now
	laterExpires := now.Add(5 * 24 * time.Hour)
	sub, reused, err = svc.AssignOrExtendSubscription(ctx, &AssignSubscriptionInput{
		UserID:       user.ID,
		GroupID:      group.ID,
		ValidityDays: 5,
		StartsAt:     &laterStart,
		ExpiresAt:    &laterExpires,
		Notes:        "later-fixed-order",
		Source:       domain.SubscriptionSourcePayment,
	})
	require.NoError(t, err)
	require.True(t, reused)
	require.WithinDuration(t, originalStart, sub.StartsAt, time.Second)
	require.WithinDuration(t, laterExpires, sub.ExpiresAt, time.Second)
	require.Contains(t, sub.Notes, "later-fixed-order")
}

func TestAssignOrExtendSubscription_WeekendSkipRenewalAdvancesOriginalExpiresAt(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("sub-weekend-skip-renew@example.com").
		SetPasswordHash("hash").
		SetUsername("sub-weekend-skip-renew-user").
		Save(ctx)
	require.NoError(t, err)

	dailyLimit := 40.0
	group, err := client.Group.Create().
		SetName("sub-weekend-skip-renew-group").
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SetDailyLimitUsd(dailyLimit).
		SetAllowDailyOverdraft(true).
		SetRateMultiplier(1.0).
		Save(ctx)
	require.NoError(t, err)

	repo := &subscriptionEntRepo{client: client}
	startsAt := time.Now().Add(-24 * time.Hour)
	originalExpires := startsAt.AddDate(0, 0, 5)
	expiresAt := addWeekendSkippedDuration(startsAt, 5*24*time.Hour)
	err = repo.Create(ctx, &UserSubscription{
		UserID:                       user.ID,
		GroupID:                      group.ID,
		StartsAt:                     startsAt,
		ExpiresAt:                    expiresAt,
		Status:                       SubscriptionStatusActive,
		ValidityUnit:                 "day",
		AllowDailyOverdraft:          true,
		SkipWeekends:                 true,
		WeekendSkipOriginalExpiresAt: &originalExpires,
		Source:                       domain.SubscriptionSourcePayment,
	})
	require.NoError(t, err)

	svc := NewSubscriptionService(&subscriptionGroupRepoStub{
		group: &Group{
			ID:                  group.ID,
			SubscriptionType:    SubscriptionTypeSubscription,
			DailyLimitUSD:       &dailyLimit,
			AllowDailyOverdraft: true,
		},
	}, repo, nil, client, nil)

	sub, reused, err := svc.AssignOrExtendSubscription(ctx, &AssignSubscriptionInput{
		UserID:       user.ID,
		GroupID:      group.ID,
		ValidityDays: 5,
		ValidityUnit: "day",
		Source:       domain.SubscriptionSourcePayment,
	})
	require.NoError(t, err)
	require.True(t, reused)
	require.NotNil(t, sub.WeekendSkipOriginalExpiresAt)
	require.WithinDuration(t, originalExpires.AddDate(0, 0, 5), *sub.WeekendSkipOriginalExpiresAt, time.Second)
	require.Equal(t, 10, sub.OverdraftValidityDays())
	limit, ok := sub.DailyOverdraftLimitUSD(&Group{SubscriptionType: SubscriptionTypeSubscription, DailyLimitUSD: &dailyLimit, AllowDailyOverdraft: true})
	require.True(t, ok)
	require.InDelta(t, 400.0, limit, 0.0001)
}

func TestAssignOrExtendSubscription_WeekendSkipRestartResetsOriginalExpiresAt(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("sub-weekend-skip-restart@example.com").
		SetPasswordHash("hash").
		SetUsername("sub-weekend-skip-restart-user").
		Save(ctx)
	require.NoError(t, err)

	group, err := client.Group.Create().
		SetName("sub-weekend-skip-restart-group").
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SetRateMultiplier(1.0).
		Save(ctx)
	require.NoError(t, err)

	repo := &subscriptionEntRepo{client: client}
	startsAt := time.Now().Add(-10 * 24 * time.Hour)
	staleOriginalExpires := startsAt.AddDate(0, 0, 5)
	expiresAt := time.Now().Add(10 * 24 * time.Hour)
	err = repo.Create(ctx, &UserSubscription{
		UserID:                       user.ID,
		GroupID:                      group.ID,
		StartsAt:                     startsAt,
		ExpiresAt:                    expiresAt,
		Status:                       SubscriptionStatusQuotaExhausted,
		ValidityUnit:                 "day",
		SkipWeekends:                 true,
		WeekendSkipOriginalExpiresAt: &staleOriginalExpires,
		Source:                       domain.SubscriptionSourcePayment,
	})
	require.NoError(t, err)

	svc := NewSubscriptionService(&subscriptionGroupRepoStub{
		group: &Group{ID: group.ID, SubscriptionType: SubscriptionTypeSubscription},
	}, repo, nil, client, nil)

	before := time.Now()
	sub, reused, err := svc.AssignOrExtendSubscription(ctx, &AssignSubscriptionInput{
		UserID:        user.ID,
		GroupID:       group.ID,
		ValidityDays:  5,
		ValidityUnit:  "day",
		Source:        domain.SubscriptionSourcePayment,
		RestartPeriod: true,
	})
	require.NoError(t, err)
	require.True(t, reused)
	require.NotNil(t, sub.WeekendSkipOriginalExpiresAt)
	require.WithinDuration(t, before.AddDate(0, 0, 5), *sub.WeekendSkipOriginalExpiresAt, 3*time.Second)
	require.Equal(t, 5, sub.OverdraftValidityDays())
}

func TestAssignOrExtendSubscription_PaidOneDayRenewalResetsDailyUsageAndRestartsOneDayPeriod(t *testing.T) {
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
	require.WithinDuration(t, after.Add(24*time.Hour), sub.ExpiresAt, 3*time.Second)
	require.True(t, sub.ExpiresAt.Before(expiresAt.Add(23*time.Hour)), "1-day renewal should restart one fresh day instead of cumulative expiry plus a reset")
	sub.Group = &Group{ID: group.ID, SubscriptionType: SubscriptionTypeSubscription, DailyLimitUSD: &dailyLimit}
	_, err = svc.ValidateAndCheckLimits(context.Background(), sub, sub.Group)
	require.NoError(t, err)
	require.WithinDuration(t, sub.ExpiresAt, *sub.DailyResetTime(), 3*time.Second)
}

func TestAssignOrExtendSubscription_PaidOneDayRenewalClearsHistoricalDebtWhenOverdraftDisabled(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("sub-paid-renew-clear-debt@example.com").
		SetPasswordHash("hash").
		SetUsername("sub-paid-renew-clear-debt-user").
		Save(ctx)
	require.NoError(t, err)

	dailyLimit := 80.0
	group, err := client.Group.Create().
		SetName("sub-paid-renew-clear-debt-group").
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
		WeeklyUsageUSD:   dailyLimit,
		MonthlyUsageUSD:  dailyLimit,
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

	sub, reused, err := svc.AssignOrExtendSubscription(ctx, &AssignSubscriptionInput{
		UserID:       user.ID,
		GroupID:      group.ID,
		ValidityDays: 1,
		Notes:        "renew-note",
		Source:       "payment",
	})
	require.NoError(t, err)
	require.True(t, reused)
	require.NotNil(t, sub)
	require.Equal(t, 0.0, sub.DailyUsageUSD)
	require.Equal(t, 0.0, sub.WeeklyUsageUSD)
	require.Equal(t, 0.0, sub.MonthlyUsageUSD)
	sub.Group = &Group{ID: group.ID, SubscriptionType: SubscriptionTypeSubscription, DailyLimitUSD: &dailyLimit}
	_, err = svc.ValidateAndCheckLimits(context.Background(), sub, sub.Group)
	require.NoError(t, err)
}

func TestAssignOrExtendSubscription_PaidOneDayRenewalDoesNotShrinkOverdraftPool(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("sub-paid-renew-overdraft-keep-expiry@example.com").
		SetPasswordHash("hash").
		SetUsername("sub-paid-renew-overdraft-keep-expiry-user").
		Save(ctx)
	require.NoError(t, err)

	dailyLimit := 10.0
	group, err := client.Group.Create().
		SetName("sub-paid-renew-overdraft-keep-expiry-group").
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SetDailyLimitUsd(dailyLimit).
		SetAllowDailyOverdraft(true).
		SetRateMultiplier(1.0).
		Save(ctx)
	require.NoError(t, err)

	repo := &subscriptionEntRepo{client: client}
	startsAt := time.Now().Add(-8 * 24 * time.Hour)
	expiresAt := startsAt.Add(10 * 24 * time.Hour)
	dailyStart := startsAt
	err = repo.Create(ctx, &UserSubscription{
		UserID:              user.ID,
		GroupID:             group.ID,
		StartsAt:            startsAt,
		ExpiresAt:           expiresAt,
		Status:              SubscriptionStatusActive,
		DailyWindowStart:    &dailyStart,
		DailyUsageUSD:       dailyLimit,
		WeeklyUsageUSD:      95,
		MonthlyUsageUSD:     95,
		AllowDailyOverdraft: true,
		Source:              "payment",
	})
	require.NoError(t, err)

	svc := NewSubscriptionService(&subscriptionGroupRepoStub{
		group: &Group{
			ID:                  group.ID,
			SubscriptionType:    SubscriptionTypeSubscription,
			DailyLimitUSD:       &dailyLimit,
			AllowDailyOverdraft: true,
		},
	}, repo, nil, client, nil)

	sub, reused, err := svc.AssignOrExtendSubscription(ctx, &AssignSubscriptionInput{
		UserID:       user.ID,
		GroupID:      group.ID,
		ValidityDays: 1,
		Source:       "payment",
	})

	require.NoError(t, err)
	require.True(t, reused)
	require.Equal(t, 0.0, sub.DailyUsageUSD)
	require.False(t, sub.ExpiresAt.Before(expiresAt), "overdraft renewal must not shrink the period pool")
	require.NotNil(t, sub.DailyWindowStart)
	require.WithinDuration(t, sub.CurrentDailyWindowStart(time.Now()), *sub.DailyWindowStart, 3*time.Second)
	sub.Group = &Group{ID: group.ID, SubscriptionType: SubscriptionTypeSubscription, DailyLimitUSD: &dailyLimit, AllowDailyOverdraft: true}
	_, err = svc.ValidateAndCheckLimits(context.Background(), sub, sub.Group)
	require.NoError(t, err)
}

func TestAssignOrExtendSubscription_MultiDayPaidRenewalKeepsDailyUsage(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("sub-paid-renew-keep-daily@example.com").
		SetPasswordHash("hash").
		SetUsername("sub-paid-renew-keep-daily-user").
		Save(ctx)
	require.NoError(t, err)

	dailyLimit := 80.0
	group, err := client.Group.Create().
		SetName("sub-paid-renew-keep-daily-group").
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
		DailyUsageUSD:    61.96,
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

	sub, reused, err := svc.AssignOrExtendSubscription(ctx, &AssignSubscriptionInput{
		UserID:       user.ID,
		GroupID:      group.ID,
		ValidityDays: 5,
		Notes:        "renew-note",
		Source:       "payment",
	})

	require.NoError(t, err)
	require.True(t, reused)
	require.NotNil(t, sub)
	require.Equal(t, 61.96, sub.DailyUsageUSD)
	require.NotNil(t, sub.DailyWindowStart)
	require.Equal(t, dailyStart.Unix(), sub.DailyWindowStart.Unix())
	require.WithinDuration(t, expiresAt.AddDate(0, 0, 5), sub.ExpiresAt, time.Second)
}

func TestAssignOrExtendSubscription_QuotaExhaustedRenewalRestartsPeriodAndRestoresUsability(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("sub-paid-renew-quota-exhausted@example.com").
		SetPasswordHash("hash").
		SetUsername("sub-paid-renew-quota-exhausted-user").
		Save(ctx)
	require.NoError(t, err)

	dailyLimit := 80.0
	weeklyLimit := 560.0
	group, err := client.Group.Create().
		SetName("sub-paid-renew-quota-exhausted-group").
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SetDailyLimitUsd(dailyLimit).
		SetWeeklyLimitUsd(weeklyLimit).
		SetRateMultiplier(1.0).
		Save(ctx)
	require.NoError(t, err)

	repo := &subscriptionEntRepo{client: client}
	startsAt := time.Now().Add(-6 * 24 * time.Hour)
	expiresAt := time.Now().Add(3 * time.Hour)
	dailyStart := time.Now().Add(-12 * time.Hour)
	weeklyStart := startsAt
	err = repo.Create(ctx, &UserSubscription{
		UserID:            user.ID,
		GroupID:           group.ID,
		StartsAt:          startsAt,
		ExpiresAt:         expiresAt,
		Status:            SubscriptionStatusQuotaExhausted,
		DailyWindowStart:  &dailyStart,
		WeeklyWindowStart: &weeklyStart,
		DailyUsageUSD:     dailyLimit,
		WeeklyUsageUSD:    weeklyLimit,
		MonthlyUsageUSD:   weeklyLimit,
		Source:            "payment",
	})
	require.NoError(t, err)

	svc := NewSubscriptionService(&subscriptionGroupRepoStub{
		group: &Group{
			ID:               group.ID,
			SubscriptionType: SubscriptionTypeSubscription,
			DailyLimitUSD:    &dailyLimit,
			WeeklyLimitUSD:   &weeklyLimit,
		},
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
	require.Equal(t, SubscriptionStatusActive, sub.Status)
	require.WithinDuration(t, before, sub.StartsAt, 3*time.Second)
	require.True(t, !sub.ExpiresAt.Before(before.AddDate(0, 0, 7)))
	require.True(t, !sub.ExpiresAt.After(after.AddDate(0, 0, 7).Add(3*time.Second)))
	require.Nil(t, sub.DailyWindowStart)
	require.Nil(t, sub.WeeklyWindowStart)
	require.Nil(t, sub.MonthlyWindowStart)
	require.Equal(t, 0.0, sub.DailyUsageUSD)
	require.Equal(t, 0.0, sub.WeeklyUsageUSD)
	require.Equal(t, 0.0, sub.MonthlyUsageUSD)

	sub.Group = &Group{
		ID:               group.ID,
		SubscriptionType: SubscriptionTypeSubscription,
		DailyLimitUSD:    &dailyLimit,
		WeeklyLimitUSD:   &weeklyLimit,
	}
	_, err = svc.ValidateAndCheckLimits(context.Background(), sub, sub.Group)
	require.NoError(t, err)
}

func TestGetActiveSubscription_QuotaExhaustedDailyOverdraftRecoversAfterDailyWindowReset(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("sub-quota-exhausted-daily-window-recovers@example.com").
		SetPasswordHash("hash").
		SetUsername("sub-quota-exhausted-daily-window-recovers-user").
		Save(ctx)
	require.NoError(t, err)

	dailyLimit := 120.0
	groupEntity, err := client.Group.Create().
		SetName("sub-quota-exhausted-daily-window-recovers-group").
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SetDailyLimitUsd(dailyLimit).
		SetAllowDailyOverdraft(true).
		SetRateMultiplier(1.0).
		Save(ctx)
	require.NoError(t, err)

	repo := &subscriptionEntRepo{client: client}
	startsAt := time.Now().Add(-26 * time.Hour)
	expiresAt := startsAt.Add(5 * 24 * time.Hour)
	oldDailyStart := startsAt
	exhaustedSub := &UserSubscription{
		UserID:              user.ID,
		GroupID:             groupEntity.ID,
		StartsAt:            startsAt,
		ExpiresAt:           expiresAt,
		Status:              SubscriptionStatusQuotaExhausted,
		ValidityUnit:        "day",
		DailyWindowStart:    &oldDailyStart,
		DailyUsageUSD:       239.9820036,
		WeeklyUsageUSD:      571.85369254,
		MonthlyUsageUSD:     571.85369254,
		AllowDailyOverdraft: true,
		Source:              domain.SubscriptionSourcePayment,
	}
	err = repo.Create(ctx, exhaustedSub)
	require.NoError(t, err)

	group := &Group{
		ID:                  groupEntity.ID,
		SubscriptionType:    SubscriptionTypeSubscription,
		DailyLimitUSD:       &dailyLimit,
		AllowDailyOverdraft: true,
	}
	svc := NewSubscriptionService(&subscriptionGroupRepoStub{group: group}, repo, nil, client, nil)

	sub, err := svc.GetActiveSubscription(ctx, user.ID, groupEntity.ID)

	require.NoError(t, err)
	require.NotNil(t, sub)
	require.Equal(t, SubscriptionStatusActive, sub.Status)
	require.NotNil(t, sub.DailyWindowStart)
	require.WithinDuration(t, sub.CurrentDailyWindowStart(time.Now()), *sub.DailyWindowStart, 3*time.Second)
	require.Equal(t, 0.0, sub.DailyUsageUSD)
	sub.Group = group
	_, err = svc.ValidateAndCheckLimits(context.Background(), sub, sub.Group)
	require.NoError(t, err)

	persisted, err := repo.GetByID(ctx, sub.ID)
	require.NoError(t, err)
	require.Equal(t, SubscriptionStatusActive, persisted.Status)
	require.InDelta(t, 0.0, persisted.DailyUsageUSD, 1e-9)
}

func TestGetActiveSubscription_QuotaExhaustedDailyOverdraftStaysExhaustedWhenPeriodPoolUsed(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("sub-quota-exhausted-period-pool-stays@example.com").
		SetPasswordHash("hash").
		SetUsername("sub-quota-exhausted-period-pool-stays-user").
		Save(ctx)
	require.NoError(t, err)

	dailyLimit := 120.0
	groupEntity, err := client.Group.Create().
		SetName("sub-quota-exhausted-period-pool-stays-group").
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SetDailyLimitUsd(dailyLimit).
		SetAllowDailyOverdraft(true).
		SetRateMultiplier(1.0).
		Save(ctx)
	require.NoError(t, err)

	repo := &subscriptionEntRepo{client: client}
	startsAt := time.Now().Add(-26 * time.Hour)
	expiresAt := startsAt.Add(5 * 24 * time.Hour)
	oldDailyStart := startsAt
	exhaustedSub := &UserSubscription{
		UserID:              user.ID,
		GroupID:             groupEntity.ID,
		StartsAt:            startsAt,
		ExpiresAt:           expiresAt,
		Status:              SubscriptionStatusQuotaExhausted,
		ValidityUnit:        "day",
		DailyWindowStart:    &oldDailyStart,
		DailyUsageUSD:       239.9820036,
		WeeklyUsageUSD:      dailyLimit * 5,
		MonthlyUsageUSD:     dailyLimit * 5,
		AllowDailyOverdraft: true,
		Source:              domain.SubscriptionSourcePayment,
	}
	err = repo.Create(ctx, exhaustedSub)
	require.NoError(t, err)

	group := &Group{
		ID:                  groupEntity.ID,
		SubscriptionType:    SubscriptionTypeSubscription,
		DailyLimitUSD:       &dailyLimit,
		AllowDailyOverdraft: true,
	}
	svc := NewSubscriptionService(&subscriptionGroupRepoStub{group: group}, repo, nil, client, nil)

	_, err = svc.GetActiveSubscription(ctx, user.ID, groupEntity.ID)

	require.ErrorIs(t, err, ErrSubscriptionNotFound)
	persisted, err := repo.GetByID(ctx, exhaustedSub.ID)
	require.NoError(t, err)
	require.Equal(t, SubscriptionStatusQuotaExhausted, persisted.Status)
}

func TestAssignOrExtendSubscription_MultiDayPaidRestartResetsPeriod(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)

	user, err := client.User.Create().
		SetEmail("sub-paid-restart-period@example.com").
		SetPasswordHash("hash").
		SetUsername("sub-paid-restart-period-user").
		Save(ctx)
	require.NoError(t, err)

	dailyLimit := 80.0
	weeklyLimit := 560.0
	group, err := client.Group.Create().
		SetName("sub-paid-restart-period-group").
		SetPlatform(PlatformOpenAI).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SetDailyLimitUsd(dailyLimit).
		SetWeeklyLimitUsd(weeklyLimit).
		SetRateMultiplier(1.0).
		Save(ctx)
	require.NoError(t, err)

	repo := &subscriptionEntRepo{client: client}
	startsAt := time.Now().Add(-6 * 24 * time.Hour)
	expiresAt := time.Now().Add(3 * time.Hour)
	dailyStart := time.Now().Add(-21 * time.Hour)
	weeklyStart := startsAt
	err = repo.Create(ctx, &UserSubscription{
		UserID:            user.ID,
		GroupID:           group.ID,
		StartsAt:          startsAt,
		ExpiresAt:         expiresAt,
		Status:            SubscriptionStatusActive,
		DailyWindowStart:  &dailyStart,
		WeeklyWindowStart: &weeklyStart,
		DailyUsageUSD:     dailyLimit,
		WeeklyUsageUSD:    weeklyLimit,
		Source:            "payment",
	})
	require.NoError(t, err)

	svc := NewSubscriptionService(&subscriptionGroupRepoStub{
		group: &Group{
			ID:               group.ID,
			SubscriptionType: SubscriptionTypeSubscription,
			DailyLimitUSD:    &dailyLimit,
			WeeklyLimitUSD:   &weeklyLimit,
		},
	}, repo, nil, client, nil)

	before := time.Now()
	sub, reused, err := svc.AssignOrExtendSubscription(ctx, &AssignSubscriptionInput{
		UserID:        user.ID,
		GroupID:       group.ID,
		ValidityDays:  7,
		Notes:         "restart-note",
		Source:        "payment",
		RestartPeriod: true,
	})
	after := time.Now()

	require.NoError(t, err)
	require.True(t, reused)
	require.NotNil(t, sub)
	require.WithinDuration(t, before, sub.StartsAt, 3*time.Second)
	require.True(t, !sub.ExpiresAt.Before(before.AddDate(0, 0, 7)))
	require.True(t, !sub.ExpiresAt.After(after.AddDate(0, 0, 7).Add(3*time.Second)))
	require.Nil(t, sub.DailyWindowStart)
	require.Nil(t, sub.WeeklyWindowStart)
	require.Equal(t, 0.0, sub.DailyUsageUSD)
	require.Equal(t, 0.0, sub.WeeklyUsageUSD)
}

func TestCanRestartSubscriptionPeriod_UsesPlanUnitLimit(t *testing.T) {
	now := time.Now()
	daily := 80.0
	weekly := 560.0
	monthly := 2400.0
	sub := &UserSubscription{
		StartsAt:        now.Add(-29 * 24 * time.Hour),
		ExpiresAt:       now.Add(2 * time.Hour),
		Status:          SubscriptionStatusActive,
		DailyUsageUSD:   daily,
		WeeklyUsageUSD:  weekly - 1,
		MonthlyUsageUSD: monthly - 1,
	}
	group := &Group{
		SubscriptionType: SubscriptionTypeSubscription,
		DailyLimitUSD:    &daily,
		WeeklyLimitUSD:   &weekly,
		MonthlyLimitUSD:  &monthly,
	}

	require.False(t, canRestartSubscriptionPeriod(now, sub, group, "days"), "day-unit cards should not restart based on daily exhaustion")
	require.False(t, canRestartSubscriptionPeriod(now, sub, group, "weeks"), "week cards require weekly limit exhaustion")
	require.False(t, canRestartSubscriptionPeriod(now, sub, group, "months"), "month cards require monthly limit exhaustion")

	sub.WeeklyUsageUSD = weekly
	require.True(t, canRestartSubscriptionPeriod(now, sub, group, "weeks"))

	sub.MonthlyUsageUSD = monthly
	require.True(t, canRestartSubscriptionPeriod(now, sub, group, "months"))
}

func TestSubscriptionService_ValidateAndCheckLimits_DailyOverdraftUsesValidityDays(t *testing.T) {
	daily := 80.0
	weekly := 560.0
	now := time.Now()
	startsAt := now.Add(-time.Hour)
	sub := &UserSubscription{
		StartsAt:            startsAt,
		ExpiresAt:           startsAt.Add(5 * 24 * time.Hour),
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

	sub.WeeklyUsageUSD = 400
	_, err = svc.ValidateAndCheckLimits(context.Background(), sub, overdraftGroup)
	require.ErrorIs(t, err, ErrDailyLimitExceeded)
}
