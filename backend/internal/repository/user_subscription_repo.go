package repository

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/ent/usersubscription"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type userSubscriptionRepository struct {
	client *dbent.Client
}

func NewUserSubscriptionRepository(client *dbent.Client) service.UserSubscriptionRepository {
	return &userSubscriptionRepository{client: client}
}

func (r *userSubscriptionRepository) Create(ctx context.Context, sub *service.UserSubscription) error {
	if sub == nil {
		return service.ErrSubscriptionNilInput
	}

	client := clientFromContext(ctx, r.client)
	builder := client.UserSubscription.Create().
		SetUserID(sub.UserID).
		SetGroupID(sub.GroupID).
		SetExpiresAt(sub.ExpiresAt).
		SetValidityUnit(normalizeRepoSubscriptionValidityUnit(sub.ValidityUnit)).
		SetNillableDailyWindowStart(sub.DailyWindowStart).
		SetNillableWeeklyWindowStart(sub.WeeklyWindowStart).
		SetNillableMonthlyWindowStart(sub.MonthlyWindowStart).
		SetDailyUsageUsd(sub.DailyUsageUSD).
		SetWeeklyUsageUsd(sub.WeeklyUsageUSD).
		SetMonthlyUsageUsd(sub.MonthlyUsageUSD).
		SetNillableQuotaLimitUsd(sub.QuotaLimitUSD).
		SetQuotaUsedUsd(sub.QuotaUsedUSD).
		SetAllowDailyOverdraft(sub.AllowDailyOverdraft).
		SetSkipWeekends(sub.SkipWeekends).
		SetNillableWeekendSkipUserChangedAt(sub.WeekendSkipUserChangedAt).
		SetNillableWeekendSkipOriginalExpiresAt(sub.WeekendSkipOriginalExpiresAt).
		SetNillableWeekendSkipAdminUpdatedAt(sub.WeekendSkipAdminUpdatedAt).
		SetNillableWeekendSkipAdminUpdatedBy(sub.WeekendSkipAdminUpdatedBy).
		SetNillableAssignedBy(sub.AssignedBy)

	if sub.StartsAt.IsZero() {
		builder.SetStartsAt(time.Now())
	} else {
		builder.SetStartsAt(sub.StartsAt)
	}
	if sub.Status != "" {
		builder.SetStatus(sub.Status)
	}
	if !sub.AssignedAt.IsZero() {
		builder.SetAssignedAt(sub.AssignedAt)
	}
	// Keep compatibility with historical behavior: always store notes as a string value.
	builder.SetNotes(sub.Notes)
	// Source 决定订阅是否可被管理员重置；为空时由 service 层规范化为 admin/redeem。
	if sub.Source != "" {
		builder.SetSource(sub.Source)
	}

	created, err := builder.Save(ctx)
	if err == nil {
		applyUserSubscriptionEntityToService(sub, created)
	}
	return translatePersistenceError(err, nil, service.ErrSubscriptionAlreadyExists)
}

func (r *userSubscriptionRepository) GetByID(ctx context.Context, id int64) (*service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.UserSubscription.Query().
		Where(usersubscription.IDEQ(id)).
		WithUser().
		WithGroup().
		WithAssignedByUser().
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	return userSubscriptionEntityToService(m), nil
}

func (r *userSubscriptionRepository) GetByIDIncludeDeleted(ctx context.Context, id int64) (*service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	queryCtx := mixins.SkipSoftDelete(ctx)
	m, err := client.UserSubscription.Query().
		Where(usersubscription.IDEQ(id)).
		WithUser().
		WithGroup().
		WithAssignedByUser().
		Only(queryCtx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	out := userSubscriptionEntityToService(m)
	if out != nil {
		out.Status = m.Status
	}
	return out, nil
}

func (r *userSubscriptionRepository) GetByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.UserSubscription.Query().
		Where(usersubscription.UserIDEQ(userID), usersubscription.GroupIDEQ(groupID)).
		WithGroup().
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	return userSubscriptionEntityToService(m), nil
}

func (r *userSubscriptionRepository) GetActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	now := time.Now()
	m, err := client.UserSubscription.Query().
		Where(
			usersubscription.UserIDEQ(userID),
			usersubscription.GroupIDEQ(groupID),
			usersubscription.StatusEQ(service.SubscriptionStatusActive),
			usersubscription.StartsAtLTE(now),
			usersubscription.ExpiresAtGT(now),
		).
		WithGroup().
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	return userSubscriptionEntityToService(m), nil
}

func (r *userSubscriptionRepository) Update(ctx context.Context, sub *service.UserSubscription) error {
	if sub == nil {
		return service.ErrSubscriptionNilInput
	}

	client := clientFromContext(ctx, r.client)
	builder := client.UserSubscription.UpdateOneID(sub.ID).
		SetUserID(sub.UserID).
		SetGroupID(sub.GroupID).
		SetStartsAt(sub.StartsAt).
		SetExpiresAt(sub.ExpiresAt).
		SetStatus(sub.Status).
		SetValidityUnit(normalizeRepoSubscriptionValidityUnit(sub.ValidityUnit)).
		SetDailyUsageUsd(sub.DailyUsageUSD).
		SetWeeklyUsageUsd(sub.WeeklyUsageUSD).
		SetMonthlyUsageUsd(sub.MonthlyUsageUSD).
		SetQuotaUsedUsd(sub.QuotaUsedUSD).
		SetAllowDailyOverdraft(sub.AllowDailyOverdraft).
		SetSkipWeekends(sub.SkipWeekends).
		SetNillableAssignedBy(sub.AssignedBy).
		SetAssignedAt(sub.AssignedAt).
		SetNotes(sub.Notes)
	if sub.WeekendSkipUserChangedAt != nil {
		builder.SetWeekendSkipUserChangedAt(*sub.WeekendSkipUserChangedAt)
	} else {
		builder.ClearWeekendSkipUserChangedAt()
	}
	if sub.WeekendSkipOriginalExpiresAt != nil {
		builder.SetWeekendSkipOriginalExpiresAt(*sub.WeekendSkipOriginalExpiresAt)
	} else {
		builder.ClearWeekendSkipOriginalExpiresAt()
	}
	if sub.WeekendSkipAdminUpdatedAt != nil {
		builder.SetWeekendSkipAdminUpdatedAt(*sub.WeekendSkipAdminUpdatedAt)
	} else {
		builder.ClearWeekendSkipAdminUpdatedAt()
	}
	if sub.WeekendSkipAdminUpdatedBy != nil {
		builder.SetWeekendSkipAdminUpdatedBy(*sub.WeekendSkipAdminUpdatedBy)
	} else {
		builder.ClearWeekendSkipAdminUpdatedBy()
	}
	if sub.QuotaLimitUSD != nil {
		builder.SetQuotaLimitUsd(*sub.QuotaLimitUSD)
	} else {
		builder.ClearQuotaLimitUsd()
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
	if sub.Source != "" {
		builder.SetSource(sub.Source)
	}

	updated, err := builder.Save(ctx)
	if err == nil {
		applyUserSubscriptionEntityToService(sub, updated)
		return nil
	}
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, service.ErrSubscriptionAlreadyExists)
}

func (r *userSubscriptionRepository) Delete(ctx context.Context, id int64) error {
	// Match GORM semantics: deleting a missing row is not an error.
	client := clientFromContext(ctx, r.client)
	_, err := client.UserSubscription.Delete().Where(usersubscription.IDEQ(id)).Exec(ctx)
	return err
}

func (r *userSubscriptionRepository) Restore(ctx context.Context, subscriptionID int64, restoredStatus string) (*service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	queryCtx := mixins.SkipSoftDelete(ctx)
	_, err := client.UserSubscription.UpdateOneID(subscriptionID).
		SetStatus(restoredStatus).
		ClearDeletedAt().
		SetUpdatedAt(time.Now()).
		Save(queryCtx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionNotFound, service.ErrSubscriptionRestoreConflict)
	}
	return r.GetByID(ctx, subscriptionID)
}

func (r *userSubscriptionRepository) ListByUserID(ctx context.Context, userID int64) ([]service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	subs, err := client.UserSubscription.Query().
		Where(usersubscription.UserIDEQ(userID)).
		WithGroup().
		Order(dbent.Desc(usersubscription.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return userSubscriptionEntitiesToService(subs), nil
}

func (r *userSubscriptionRepository) ListActiveByUserID(ctx context.Context, userID int64) ([]service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	now := time.Now()
	subs, err := client.UserSubscription.Query().
		Where(
			usersubscription.UserIDEQ(userID),
			usersubscription.StatusEQ(service.SubscriptionStatusActive),
			usersubscription.StartsAtLTE(now),
			usersubscription.ExpiresAtGT(now),
		).
		WithGroup().
		Order(dbent.Desc(usersubscription.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return userSubscriptionEntitiesToService(subs), nil
}

func (r *userSubscriptionRepository) ListByGroupID(ctx context.Context, groupID int64, params pagination.PaginationParams) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	client := clientFromContext(ctx, r.client)
	q := client.UserSubscription.Query().Where(usersubscription.GroupIDEQ(groupID))

	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	subs, err := q.
		WithUser().
		WithGroup().
		Order(dbent.Desc(usersubscription.FieldCreatedAt)).
		Offset(params.Offset()).
		Limit(params.Limit()).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	return userSubscriptionEntitiesToService(subs), paginationResultFromTotal(int64(total), params), nil
}

func (r *userSubscriptionRepository) List(ctx context.Context, params pagination.PaginationParams, userID, groupID *int64, status, platform, sortBy, sortOrder string) ([]service.UserSubscription, *pagination.PaginationResult, error) {
	client := clientFromContext(ctx, r.client)
	q := client.UserSubscription.Query()
	includeSoftDeleted := status == "" || status == service.SubscriptionStatusRevoked
	if userID != nil {
		q = q.Where(usersubscription.UserIDEQ(*userID))
	}
	if groupID != nil {
		q = q.Where(usersubscription.GroupIDEQ(*groupID))
	}
	if platform != "" {
		groupPredicates := []predicate.Group{group.PlatformEQ(platform)}
		if includeSoftDeleted {
			groupPredicates = append(groupPredicates, group.DeletedAtIsNil())
		}
		q = q.Where(usersubscription.HasGroupWith(groupPredicates...))
	}

	// Status filtering with real-time expiration check
	now := time.Now()
	switch status {
	case service.SubscriptionStatusActive:
		// Active: status is active, already started, and not yet expired
		q = q.Where(
			usersubscription.StatusEQ(service.SubscriptionStatusActive),
			usersubscription.StartsAtLTE(now),
			usersubscription.ExpiresAtGT(now),
		)
	case service.SubscriptionStatusExpired:
		// Expired: status is expired OR (status is active but already expired)
		q = q.Where(
			usersubscription.Or(
				usersubscription.StatusEQ(service.SubscriptionStatusExpired),
				usersubscription.And(
					usersubscription.StatusIn(service.SubscriptionStatusActive, service.SubscriptionStatusQuotaExhausted),
					usersubscription.ExpiresAtLTE(now),
				),
			),
		)
	case service.SubscriptionStatusRevoked:
		// Revoked is a DTO/API display state backed by user_subscriptions.deleted_at.
		q = q.Where(usersubscription.DeletedAtNotNil())
	case "":
		// No filter. Use SkipSoftDelete below so admin "all status" includes revoked history.
	default:
		// Other persisted status.
		q = q.Where(usersubscription.StatusEQ(status))
	}

	queryCtx := ctx
	if includeSoftDeleted {
		queryCtx = mixins.SkipSoftDelete(ctx)
	} else {
		q = q.WithUser().WithGroup().WithAssignedByUser()
	}

	total, err := q.Clone().Count(queryCtx)
	if err != nil {
		return nil, nil, err
	}

	// last_used_at 需要按“最新使用时间”排序，不能直接依赖订阅表字段。
	// 这里先取全量订阅，再批量查询 usage_logs 中每个订阅的最新使用时间。
	// 这样可以复用分页逻辑，同时避免在主查询中拼复杂 SQL。
	if sortBy == "last_used_at" {
		all, err := q.All(queryCtx)
		if err != nil {
			return nil, nil, err
		}
		subs := userSubscriptionEntitiesToService(all)
		if includeSoftDeleted {
			if err := r.attachUserSubscriptionRelations(ctx, subs); err != nil {
				return nil, nil, err
			}
		}
		if err := r.fillLastUsedAt(ctx, subs); err != nil {
			return nil, nil, err
		}
		sortSubsByLastUsedAt(subs, sortOrder)
		page := paginateSlice(subs, params)
		return page, paginationResultFromTotal(int64(total), params), nil
	}

	// Determine sort field
	var field string
	switch sortBy {
	case "expires_at":
		field = usersubscription.FieldExpiresAt
	case "status":
		field = usersubscription.FieldStatus
	default:
		field = usersubscription.FieldCreatedAt
	}

	// Determine sort order (default: desc)
	if sortOrder == "asc" && sortBy != "" {
		q = q.Order(dbent.Asc(field))
	} else {
		q = q.Order(dbent.Desc(field))
	}

	subs, err := q.
		Offset(params.Offset()).
		Limit(params.Limit()).
		All(queryCtx)
	if err != nil {
		return nil, nil, err
	}

	results := userSubscriptionEntitiesToService(subs)
	if includeSoftDeleted {
		if err := r.attachUserSubscriptionRelations(ctx, results); err != nil {
			return nil, nil, err
		}
	}
	if err := r.fillLastUsedAt(ctx, results); err != nil {
		return nil, nil, err
	}
	return results, paginationResultFromTotal(int64(total), params), nil
}

// fillLastUsedAt 批量填充订阅的 LastUsedAt；查询失败时保持 nil。
// 该字段仅用于列表展示和 last_used_at 排序，不影响订阅主数据。
func (r *userSubscriptionRepository) fillLastUsedAt(ctx context.Context, subs []service.UserSubscription) error {
	if len(subs) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(subs))
	for i := range subs {
		ids = append(ids, subs[i].ID)
	}
	lastUsed, err := r.GetLatestUsedAtBySubscriptionIDs(ctx, ids)
	if err != nil {
		// 统计失败不阻断订阅列表，last_used_at 保持为空。
		return nil
	}
	for i := range subs {
		if ts, ok := lastUsed[subs[i].ID]; ok {
			subs[i].LastUsedAt = ts
		}
	}
	return nil
}

// sortSubsByLastUsedAt 按 LastUsedAt 稳定排序。
// nil 始终排在最后，非 nil 项按 sortOrder 指定方向排序。
func sortSubsByLastUsedAt(subs []service.UserSubscription, sortOrder string) {
	asc := sortOrder == "asc"
	sort.SliceStable(subs, func(i, j int) bool {
		li, lj := subs[i].LastUsedAt, subs[j].LastUsedAt
		// nil 统一排在最后。
		if li == nil && lj == nil {
			return false
		}
		if li == nil {
			return false
		}
		if lj == nil {
			return true
		}
		if asc {
			return li.Before(*lj)
		}
		return li.After(*lj)
	})
}

func (r *userSubscriptionRepository) ExistsByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (bool, error) {
	client := clientFromContext(ctx, r.client)
	return client.UserSubscription.Query().
		Where(usersubscription.UserIDEQ(userID), usersubscription.GroupIDEQ(groupID)).
		Exist(ctx)
}

func (r *userSubscriptionRepository) ExistsActiveByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (bool, error) {
	client := clientFromContext(ctx, r.client)
	now := time.Now()
	return client.UserSubscription.Query().
		Where(
			usersubscription.UserIDEQ(userID),
			usersubscription.GroupIDEQ(groupID),
			usersubscription.StatusEQ(service.SubscriptionStatusActive),
			usersubscription.StartsAtLTE(now),
			usersubscription.ExpiresAtGT(now),
		).
		Exist(ctx)
}

func (r *userSubscriptionRepository) ExtendExpiry(ctx context.Context, subscriptionID int64, newExpiresAt time.Time) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.UserSubscription.UpdateOneID(subscriptionID).
		SetExpiresAt(newExpiresAt).
		Save(ctx)
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
}

func (r *userSubscriptionRepository) UpdateStatus(ctx context.Context, subscriptionID int64, status string) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.UserSubscription.UpdateOneID(subscriptionID).
		SetStatus(status).
		Save(ctx)
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
}

func (r *userSubscriptionRepository) UpdateNotes(ctx context.Context, subscriptionID int64, notes string) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.UserSubscription.UpdateOneID(subscriptionID).
		SetNotes(notes).
		Save(ctx)
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
}

func (r *userSubscriptionRepository) ActivateWindows(ctx context.Context, id int64, dailyStart, weeklyStart, monthlyStart time.Time) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.UserSubscription.UpdateOneID(id).
		SetDailyWindowStart(dailyStart).
		SetWeeklyWindowStart(weeklyStart).
		SetMonthlyWindowStart(monthlyStart).
		Save(ctx)
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
}

func (r *userSubscriptionRepository) ResetUsageWindows(ctx context.Context, id int64, resetDaily, resetWeekly, resetMonthly bool, newWindowStart time.Time) error {
	client := clientFromContext(ctx, r.client)
	update := client.UserSubscription.UpdateOneID(id)
	if resetDaily {
		update.SetDailyUsageUsd(0).SetDailyWindowStart(newWindowStart)
	}
	if resetWeekly {
		update.SetWeeklyUsageUsd(0).SetWeeklyWindowStart(newWindowStart)
	}
	if resetMonthly {
		update.SetMonthlyUsageUsd(0).SetMonthlyWindowStart(newWindowStart)
	}
	_, err := update.Save(ctx)
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
}

func (r *userSubscriptionRepository) ResetDailyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error {
	client := clientFromContext(ctx, r.client)
	query := client.UserSubscription.Update().Where(usersubscription.IDEQ(id))
	if expectedWindowStart == nil {
		query = query.Where(usersubscription.DailyWindowStartIsNil())
	} else {
		query = query.Where(usersubscription.DailyWindowStartEQ(*expectedWindowStart))
	}
	n, err := query.
		SetDailyUsageUsd(0).
		SetDailyWindowStart(newWindowStart).
		Save(ctx)
	return r.translateConditionalWindowReset(ctx, client, id, n, err)
}

func (r *userSubscriptionRepository) ResetWeeklyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error {
	client := clientFromContext(ctx, r.client)
	query := client.UserSubscription.Update().Where(usersubscription.IDEQ(id))
	if expectedWindowStart == nil {
		query = query.Where(usersubscription.WeeklyWindowStartIsNil())
	} else {
		query = query.Where(usersubscription.WeeklyWindowStartEQ(*expectedWindowStart))
	}
	n, err := query.
		SetWeeklyUsageUsd(0).
		SetWeeklyWindowStart(newWindowStart).
		Save(ctx)
	return r.translateConditionalWindowReset(ctx, client, id, n, err)
}

func (r *userSubscriptionRepository) ResetMonthlyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error {
	client := clientFromContext(ctx, r.client)
	query := client.UserSubscription.Update().Where(usersubscription.IDEQ(id))
	if expectedWindowStart == nil {
		query = query.Where(usersubscription.MonthlyWindowStartIsNil())
	} else {
		query = query.Where(usersubscription.MonthlyWindowStartEQ(*expectedWindowStart))
	}
	n, err := query.
		SetMonthlyUsageUsd(0).
		SetMonthlyWindowStart(newWindowStart).
		Save(ctx)
	return r.translateConditionalWindowReset(ctx, client, id, n, err)
}

func (r *userSubscriptionRepository) translateConditionalWindowReset(ctx context.Context, client *dbent.Client, id int64, affected int, err error) error {
	if err != nil {
		return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	if affected > 0 {
		return nil
	}

	// A stale reset is an expected no-op: another request already advanced the
	// window. Preserve not-found semantics for callers that target a missing row.
	exists, err := client.UserSubscription.Query().Where(usersubscription.IDEQ(id)).Exist(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
	}
	if !exists {
		return service.ErrSubscriptionNotFound
	}
	return nil
}

// IncrementUsage atomically records subscription usage in the legacy fallback path.
// Production uses usageBillingRepository.Apply; this method keeps the same limit
// guard semantics so degraded/test paths cannot silently overrun configured pools.
func (r *userSubscriptionRepository) IncrementUsage(ctx context.Context, id int64, costUSD float64) error {
	now := time.Now()
	const baseUpdateSQL = `
		UPDATE user_subscriptions us
		JOIN ` + quotedGroupsTable + ` g ON us.group_id = g.id AND g.deleted_at IS NULL
		CROSS JOIN (SELECT ? AS now_ts) clock
		SET
			us.daily_usage_usd = us.daily_usage_usd + ?,
			us.weekly_usage_usd = us.weekly_usage_usd + ?,
			us.monthly_usage_usd = us.monthly_usage_usd + ?,
			us.quota_used_usd = us.quota_used_usd + ?,
			us.status = CASE
				WHEN COALESCE(us.quota_limit_usd, 0) > 0 AND us.quota_used_usd + ? >= us.quota_limit_usd THEN ?
				WHEN g.allow_daily_overdraft = TRUE
					AND us.allow_daily_overdraft = TRUE
					AND COALESCE(g.daily_limit_usd, 0) > 0
					AND (
						CASE
							WHEN COALESCE(us.validity_unit, 'day') IN ('day', 'days') THEN GREATEST(
								us.weekly_usage_usd + ?,
								g.daily_limit_usd * LEAST(
									GREATEST(0, FLOOR(TIMESTAMPDIFF(SECOND, us.starts_at, NOW()) / 86400)),
									GREATEST(1, CEIL(TIMESTAMPDIFF(SECOND, us.starts_at, us.expires_at) / 86400))
								) + CASE
									WHEN us.daily_window_start IS NOT NULL
										AND us.daily_window_start = TIMESTAMPADD(
											SECOND,
											FLOOR(TIMESTAMPDIFF(SECOND, us.starts_at, NOW()) / 86400) * 86400,
											us.starts_at
										)
										THEN us.daily_usage_usd + ?
									ELSE ?
								END
							)
							ELSE us.weekly_usage_usd + ?
						END
					) >= g.daily_limit_usd * GREATEST(1, CEIL(TIMESTAMPDIFF(SECOND, us.starts_at, us.expires_at) / 86400))
					THEN ?
				WHEN COALESCE(g.daily_limit_usd, 0) > 0
					AND NOT (g.allow_daily_overdraft = TRUE AND us.allow_daily_overdraft = TRUE)
					AND us.expires_at <= DATE_ADD(us.starts_at, INTERVAL 1 DAY)
					AND us.daily_usage_usd + ? >= g.daily_limit_usd THEN ?
				WHEN COALESCE(g.weekly_limit_usd, 0) > 0
					AND NOT (g.allow_daily_overdraft = TRUE AND us.allow_daily_overdraft = TRUE)
					AND us.weekly_usage_usd + ? >= g.weekly_limit_usd THEN ?
				WHEN COALESCE(g.monthly_limit_usd, 0) > 0
					AND NOT (g.allow_daily_overdraft = TRUE AND us.allow_daily_overdraft = TRUE)
					AND us.monthly_usage_usd + ? >= g.monthly_limit_usd THEN ?
				ELSE us.status
			END,
			us.updated_at = NOW()
		WHERE us.id = ?
			AND us.deleted_at IS NULL
			AND (
				COALESCE(us.quota_limit_usd, 0) <= 0
				OR us.quota_used_usd + ? <= us.quota_limit_usd
				OR (? AND us.quota_used_usd < us.quota_limit_usd)
			)
			AND (
				COALESCE(g.daily_limit_usd, 0) <= 0
				OR (
					g.allow_daily_overdraft = TRUE
					AND us.allow_daily_overdraft = TRUE
					AND (
						CASE
							WHEN COALESCE(us.validity_unit, 'day') IN ('day', 'days') THEN GREATEST(
								us.weekly_usage_usd + ?,
								g.daily_limit_usd * LEAST(
									GREATEST(0, FLOOR(TIMESTAMPDIFF(SECOND, us.starts_at, NOW()) / 86400)),
									GREATEST(1, CEIL(TIMESTAMPDIFF(SECOND, us.starts_at, us.expires_at) / 86400))
								) + CASE
									WHEN us.daily_window_start IS NOT NULL
										AND us.daily_window_start = TIMESTAMPADD(
											SECOND,
											FLOOR(TIMESTAMPDIFF(SECOND, us.starts_at, NOW()) / 86400) * 86400,
											us.starts_at
										)
										THEN us.daily_usage_usd + ?
									ELSE ?
								END
							)
							ELSE us.weekly_usage_usd + ?
						END
					) <= g.daily_limit_usd * GREATEST(1, CEIL(TIMESTAMPDIFF(SECOND, us.starts_at, us.expires_at) / 86400))
				)
				OR (
					? AND g.allow_daily_overdraft = TRUE
					AND us.allow_daily_overdraft = TRUE
					AND (
						CASE
							WHEN COALESCE(us.validity_unit, 'day') IN ('day', 'days') THEN GREATEST(
								us.weekly_usage_usd,
								g.daily_limit_usd * LEAST(
									GREATEST(0, FLOOR(TIMESTAMPDIFF(SECOND, us.starts_at, NOW()) / 86400)),
									GREATEST(1, CEIL(TIMESTAMPDIFF(SECOND, us.starts_at, us.expires_at) / 86400))
								) + CASE
									WHEN us.daily_window_start IS NOT NULL
										AND us.daily_window_start = TIMESTAMPADD(
											SECOND,
											FLOOR(TIMESTAMPDIFF(SECOND, us.starts_at, NOW()) / 86400) * 86400,
											us.starts_at
										)
										THEN us.daily_usage_usd
									ELSE 0
								END
							)
							ELSE us.weekly_usage_usd
						END
					) < g.daily_limit_usd * GREATEST(1, CEIL(TIMESTAMPDIFF(SECOND, us.starts_at, us.expires_at) / 86400))
				)
				OR (
					NOT (g.allow_daily_overdraft = TRUE AND COALESCE(us.validity_unit, 'day') IN ('day', 'days'))
					AND us.daily_usage_usd + ? <= g.daily_limit_usd
				)
				OR (
					? AND NOT (g.allow_daily_overdraft = TRUE AND COALESCE(us.validity_unit, 'day') IN ('day', 'days'))
					AND us.daily_usage_usd < g.daily_limit_usd
				)
				OR (
					g.allow_daily_overdraft = TRUE
					AND us.allow_daily_overdraft = FALSE
					AND COALESCE(us.validity_unit, 'day') IN ('day', 'days')
					AND us.daily_usage_usd + GREATEST(
						0,
						us.weekly_usage_usd - CASE
							WHEN us.daily_window_start IS NOT NULL
								AND us.daily_window_start = TIMESTAMPADD(
									SECOND,
									FLOOR(TIMESTAMPDIFF(SECOND, us.starts_at, NOW()) / 86400) * 86400,
									us.starts_at
								)
								THEN us.daily_usage_usd
							ELSE 0
						END - g.daily_limit_usd * LEAST(
							GREATEST(0, FLOOR(TIMESTAMPDIFF(SECOND, us.starts_at, NOW()) / 86400)),
							GREATEST(1, CEIL(TIMESTAMPDIFF(SECOND, us.starts_at, us.expires_at) / 86400))
						)
					) + ? <= g.daily_limit_usd
				)
				OR (
					? AND g.allow_daily_overdraft = TRUE
					AND us.allow_daily_overdraft = FALSE
					AND COALESCE(us.validity_unit, 'day') IN ('day', 'days')
					AND us.daily_usage_usd + GREATEST(
						0,
						us.weekly_usage_usd - CASE
							WHEN us.daily_window_start IS NOT NULL
								AND us.daily_window_start = TIMESTAMPADD(
									SECOND,
									FLOOR(TIMESTAMPDIFF(SECOND, us.starts_at, NOW()) / 86400) * 86400,
									us.starts_at
								)
								THEN us.daily_usage_usd
							ELSE 0
						END - g.daily_limit_usd * LEAST(
							GREATEST(0, FLOOR(TIMESTAMPDIFF(SECOND, us.starts_at, NOW()) / 86400)),
							GREATEST(1, CEIL(TIMESTAMPDIFF(SECOND, us.starts_at, us.expires_at) / 86400))
						)
					) < g.daily_limit_usd
				)
			)
			AND (
				COALESCE(g.weekly_limit_usd, 0) <= 0
				OR (g.allow_daily_overdraft = TRUE AND us.allow_daily_overdraft = TRUE)
				OR us.weekly_usage_usd + ? <= g.weekly_limit_usd
				OR (? AND us.weekly_usage_usd < g.weekly_limit_usd)
			)
			AND (
				COALESCE(g.monthly_limit_usd, 0) <= 0
				OR (g.allow_daily_overdraft = TRUE AND us.allow_daily_overdraft = TRUE)
				OR us.monthly_usage_usd + ? <= g.monthly_limit_usd
				OR (? AND us.monthly_usage_usd < g.monthly_limit_usd)
			)
	`

	updateSQL := replaceSQLNowWithClock(baseUpdateSQL)
	client := clientFromContext(ctx, r.client)
	allowOverLimit := false
	result, err := client.ExecContext(ctx, updateSQL,
		now, costUSD, costUSD, costUSD, costUSD,
		costUSD, service.SubscriptionStatusQuotaExhausted,
		costUSD, costUSD, costUSD, costUSD, service.SubscriptionStatusQuotaExhausted,
		costUSD, service.SubscriptionStatusQuotaExhausted,
		costUSD, service.SubscriptionStatusQuotaExhausted,
		costUSD, service.SubscriptionStatusQuotaExhausted,
		id,
		costUSD, allowOverLimit,
		costUSD, costUSD, costUSD, costUSD,
		allowOverLimit,
		costUSD, allowOverLimit, costUSD, allowOverLimit,
		costUSD, allowOverLimit,
		costUSD, allowOverLimit,
	)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected > 0 {
		return nil
	}

	exists, err := client.UserSubscription.Query().
		Where(
			usersubscription.IDEQ(id),
			usersubscription.DeletedAtIsNil(),
			usersubscription.HasGroupWith(group.DeletedAtIsNil()),
		).
		Exist(ctx)
	if err != nil {
		return err
	}
	if exists {
		return service.ErrUsageBillingSubscriptionLimitExceeded
	}
	return service.ErrSubscriptionNotFound
}

func (r *userSubscriptionRepository) UpdateDailyOverdraft(ctx context.Context, id int64, enabled bool) error {
	client := clientFromContext(ctx, r.client)
	_, err := client.UserSubscription.UpdateOneID(id).
		SetAllowDailyOverdraft(enabled).
		Save(ctx)
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
}

func (r *userSubscriptionRepository) UpdateWeekendSkip(ctx context.Context, sub *service.UserSubscription) error {
	if sub == nil {
		return service.ErrSubscriptionNilInput
	}
	client := clientFromContext(ctx, r.client)
	builder := client.UserSubscription.UpdateOneID(sub.ID).
		SetSkipWeekends(sub.SkipWeekends).
		SetExpiresAt(sub.ExpiresAt)
	if sub.WeekendSkipUserChangedAt != nil {
		builder.SetWeekendSkipUserChangedAt(*sub.WeekendSkipUserChangedAt)
	} else {
		builder.ClearWeekendSkipUserChangedAt()
	}
	if sub.WeekendSkipOriginalExpiresAt != nil {
		builder.SetWeekendSkipOriginalExpiresAt(*sub.WeekendSkipOriginalExpiresAt)
	} else {
		builder.ClearWeekendSkipOriginalExpiresAt()
	}
	if sub.WeekendSkipAdminUpdatedAt != nil {
		builder.SetWeekendSkipAdminUpdatedAt(*sub.WeekendSkipAdminUpdatedAt)
	} else {
		builder.ClearWeekendSkipAdminUpdatedAt()
	}
	if sub.WeekendSkipAdminUpdatedBy != nil {
		builder.SetWeekendSkipAdminUpdatedBy(*sub.WeekendSkipAdminUpdatedBy)
	} else {
		builder.ClearWeekendSkipAdminUpdatedBy()
	}
	_, err := builder.Save(ctx)
	return translatePersistenceError(err, service.ErrSubscriptionNotFound, nil)
}

func (r *userSubscriptionRepository) BatchUpdateExpiredStatus(ctx context.Context) (int64, error) {
	client := clientFromContext(ctx, r.client)
	n, err := client.UserSubscription.Update().
		Where(
			usersubscription.StatusIn(service.SubscriptionStatusActive, service.SubscriptionStatusQuotaExhausted),
			usersubscription.ExpiresAtLTE(time.Now()),
		).
		SetStatus(service.SubscriptionStatusExpired).
		Save(ctx)
	return int64(n), err
}

// GetLatestUsedAtBySubscriptionIDs 批量返回订阅在 usage_logs 上聚合的最近使用时间，
// 与 userRepository.GetLatestUsedAtByUserIDs 同范式：MAX(created_at) GROUP BY subscription_id。
// 此实现不依赖 user_subscriptions 表上的字段，避免 schema 漂移。
func (r *userSubscriptionRepository) GetLatestUsedAtBySubscriptionIDs(ctx context.Context, subscriptionIDs []int64) (map[int64]*time.Time, error) {
	result := make(map[int64]*time.Time, len(subscriptionIDs))
	if len(subscriptionIDs) == 0 {
		return result, nil
	}

	placeholders := make([]string, len(subscriptionIDs))
	args := make([]any, 0, len(subscriptionIDs))
	for i, id := range subscriptionIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}

	query := fmt.Sprintf(`
		SELECT subscription_id, MAX(created_at) AS last_used_at
		FROM usage_logs
		WHERE subscription_id IN (%s)
		GROUP BY subscription_id
	`, strings.Join(placeholders, ","))

	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			subID      int64
			lastUsedAt time.Time
		)
		if scanErr := rows.Scan(&subID, &lastUsedAt); scanErr != nil {
			return nil, scanErr
		}
		ts := lastUsedAt.UTC()
		result[subID] = &ts
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// Extra repository helpers (currently used only by integration tests).

func (r *userSubscriptionRepository) ListExpired(ctx context.Context) ([]service.UserSubscription, error) {
	client := clientFromContext(ctx, r.client)
	subs, err := client.UserSubscription.Query().
		Where(
			usersubscription.StatusIn(service.SubscriptionStatusActive, service.SubscriptionStatusQuotaExhausted),
			usersubscription.ExpiresAtLTE(time.Now()),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return userSubscriptionEntitiesToService(subs), nil
}

func (r *userSubscriptionRepository) CountByGroupID(ctx context.Context, groupID int64) (int64, error) {
	client := clientFromContext(ctx, r.client)
	count, err := client.UserSubscription.Query().Where(usersubscription.GroupIDEQ(groupID)).Count(ctx)
	return int64(count), err
}

func (r *userSubscriptionRepository) CountActiveByGroupID(ctx context.Context, groupID int64) (int64, error) {
	client := clientFromContext(ctx, r.client)
	now := time.Now()
	count, err := client.UserSubscription.Query().
		Where(
			usersubscription.GroupIDEQ(groupID),
			usersubscription.StatusEQ(service.SubscriptionStatusActive),
			usersubscription.StartsAtLTE(now),
			usersubscription.ExpiresAtGT(now),
		).
		Count(ctx)
	return int64(count), err
}

func (r *userSubscriptionRepository) DeleteByGroupID(ctx context.Context, groupID int64) (int64, error) {
	client := clientFromContext(ctx, r.client)
	n, err := client.UserSubscription.Delete().Where(usersubscription.GroupIDEQ(groupID)).Exec(ctx)
	return int64(n), err
}

func (r *userSubscriptionRepository) attachUserSubscriptionRelations(ctx context.Context, subs []service.UserSubscription) error {
	if len(subs) == 0 {
		return nil
	}

	userIDs := make([]int64, 0, len(subs))
	groupIDs := make([]int64, 0, len(subs))
	assignedByIDs := make([]int64, 0, len(subs))
	for i := range subs {
		userIDs = append(userIDs, subs[i].UserID)
		groupIDs = append(groupIDs, subs[i].GroupID)
		if subs[i].AssignedBy != nil {
			assignedByIDs = append(assignedByIDs, *subs[i].AssignedBy)
		}
	}

	client := clientFromContext(ctx, r.client)
	users, err := client.User.Query().Where(user.IDIn(uniqueInt64s(userIDs)...)).All(ctx)
	if err != nil {
		return err
	}
	userByID := make(map[int64]*service.User, len(users))
	for _, u := range users {
		userByID[u.ID] = userEntityToService(u)
	}

	groups, err := client.Group.Query().Where(group.IDIn(uniqueInt64s(groupIDs)...)).All(ctx)
	if err != nil {
		return err
	}
	groupByID := make(map[int64]*service.Group, len(groups))
	for _, g := range groups {
		groupByID[g.ID] = groupEntityToService(g)
	}

	assignedByID := map[int64]*service.User{}
	if len(assignedByIDs) > 0 {
		assignedUsers, err := client.User.Query().Where(user.IDIn(uniqueInt64s(assignedByIDs)...)).All(ctx)
		if err != nil {
			return err
		}
		assignedByID = make(map[int64]*service.User, len(assignedUsers))
		for _, u := range assignedUsers {
			assignedByID[u.ID] = userEntityToService(u)
		}
	}

	for i := range subs {
		subs[i].User = userByID[subs[i].UserID]
		subs[i].Group = groupByID[subs[i].GroupID]
		if subs[i].AssignedBy != nil {
			subs[i].AssignedByUser = assignedByID[*subs[i].AssignedBy]
		}
	}
	return nil
}

func uniqueInt64s(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	out := make([]int64, 0, len(values))
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func userSubscriptionEntityToService(m *dbent.UserSubscription) *service.UserSubscription {
	if m == nil {
		return nil
	}
	status := m.Status
	if m.DeletedAt != nil {
		status = service.SubscriptionStatusRevoked
	}
	out := &service.UserSubscription{
		ID:                           m.ID,
		UserID:                       m.UserID,
		GroupID:                      m.GroupID,
		StartsAt:                     m.StartsAt,
		ExpiresAt:                    m.ExpiresAt,
		Status:                       status,
		ValidityUnit:                 normalizeRepoSubscriptionValidityUnit(m.ValidityUnit),
		DailyWindowStart:             m.DailyWindowStart,
		WeeklyWindowStart:            m.WeeklyWindowStart,
		MonthlyWindowStart:           m.MonthlyWindowStart,
		DailyUsageUSD:                m.DailyUsageUsd,
		WeeklyUsageUSD:               m.WeeklyUsageUsd,
		MonthlyUsageUSD:              m.MonthlyUsageUsd,
		QuotaLimitUSD:                m.QuotaLimitUsd,
		QuotaUsedUSD:                 m.QuotaUsedUsd,
		AllowDailyOverdraft:          m.AllowDailyOverdraft,
		SkipWeekends:                 m.SkipWeekends,
		WeekendSkipUserChangedAt:     m.WeekendSkipUserChangedAt,
		WeekendSkipOriginalExpiresAt: m.WeekendSkipOriginalExpiresAt,
		WeekendSkipAdminUpdatedAt:    m.WeekendSkipAdminUpdatedAt,
		WeekendSkipAdminUpdatedBy:    m.WeekendSkipAdminUpdatedBy,
		AssignedBy:                   m.AssignedBy,
		AssignedAt:                   m.AssignedAt,
		Notes:                        derefString(m.Notes),
		Source:                       m.Source,
		CreatedAt:                    m.CreatedAt,
		UpdatedAt:                    m.UpdatedAt,
		DeletedAt:                    m.DeletedAt,
	}
	if m.Edges.User != nil {
		out.User = userEntityToService(m.Edges.User)
	}
	if m.Edges.Group != nil {
		out.Group = groupEntityToService(m.Edges.Group)
	}
	if m.Edges.AssignedByUser != nil {
		out.AssignedByUser = userEntityToService(m.Edges.AssignedByUser)
	}
	return out
}

func userSubscriptionEntitiesToService(models []*dbent.UserSubscription) []service.UserSubscription {
	out := make([]service.UserSubscription, 0, len(models))
	for i := range models {
		if s := userSubscriptionEntityToService(models[i]); s != nil {
			out = append(out, *s)
		}
	}
	return out
}

func applyUserSubscriptionEntityToService(dst *service.UserSubscription, src *dbent.UserSubscription) {
	if dst == nil || src == nil {
		return
	}
	dst.ID = src.ID
	dst.ValidityUnit = normalizeRepoSubscriptionValidityUnit(src.ValidityUnit)
	dst.QuotaLimitUSD = src.QuotaLimitUsd
	dst.QuotaUsedUSD = src.QuotaUsedUsd
	dst.SkipWeekends = src.SkipWeekends
	dst.WeekendSkipUserChangedAt = src.WeekendSkipUserChangedAt
	dst.WeekendSkipOriginalExpiresAt = src.WeekendSkipOriginalExpiresAt
	dst.WeekendSkipAdminUpdatedAt = src.WeekendSkipAdminUpdatedAt
	dst.WeekendSkipAdminUpdatedBy = src.WeekendSkipAdminUpdatedBy
	dst.CreatedAt = src.CreatedAt
	dst.UpdatedAt = src.UpdatedAt
}

func normalizeRepoSubscriptionValidityUnit(unit string) string {
	switch strings.TrimSpace(strings.ToLower(unit)) {
	case "", "day", "days":
		return "day"
	case "week", "weeks":
		return "week"
	case "month", "months":
		return "month"
	default:
		return strings.TrimSpace(strings.ToLower(unit))
	}
}
