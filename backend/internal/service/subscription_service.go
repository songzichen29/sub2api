package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/ent/usagelog"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/dgraph-io/ristretto"
	"golang.org/x/sync/singleflight"
)

// MaxExpiresAt is the maximum allowed expiration date (year 2099)
// This prevents time.Time JSON serialization errors (RFC 3339 requires year <= 9999)
var MaxExpiresAt = time.Date(2099, 12, 31, 23, 59, 59, 0, time.UTC)

// MaxValidityDays is the maximum allowed validity days for subscriptions (100 years)
const MaxValidityDays = 36500

var (
	ErrSubscriptionNotFound        = infraerrors.NotFound("SUBSCRIPTION_NOT_FOUND", "subscription not found")
	ErrSubscriptionExpired         = infraerrors.Forbidden("SUBSCRIPTION_EXPIRED", "subscription has expired")
	ErrSubscriptionNotStarted      = infraerrors.Forbidden("SUBSCRIPTION_NOT_STARTED", "subscription has not started")
	ErrSubscriptionSuspended       = infraerrors.Forbidden("SUBSCRIPTION_SUSPENDED", "subscription is suspended")
	ErrSubscriptionQuotaExhausted  = infraerrors.Forbidden("SUBSCRIPTION_QUOTA_EXHAUSTED", "subscription quota exhausted")
	ErrSubscriptionAlreadyExists   = infraerrors.Conflict("SUBSCRIPTION_ALREADY_EXISTS", "subscription already exists for this user and group")
	ErrSubscriptionAssignConflict  = infraerrors.Conflict("SUBSCRIPTION_ASSIGN_CONFLICT", "subscription exists but request conflicts with existing assignment semantics")
	ErrSubscriptionRestoreConflict = infraerrors.Conflict("SUBSCRIPTION_RESTORE_CONFLICT", "subscription already exists for this user and group")
	ErrGroupNotSubscriptionType    = infraerrors.BadRequest("GROUP_NOT_SUBSCRIPTION_TYPE", "group is not a subscription type")
	ErrInvalidInput                = infraerrors.BadRequest("INVALID_INPUT", "at least one of resetDaily, resetWeekly, or resetMonthly must be true")
	ErrDailyLimitExceeded          = infraerrors.TooManyRequests("DAILY_LIMIT_EXCEEDED", "daily usage limit exceeded")
	ErrWeeklyLimitExceeded         = infraerrors.TooManyRequests("WEEKLY_LIMIT_EXCEEDED", "weekly usage limit exceeded")
	ErrMonthlyLimitExceeded        = infraerrors.TooManyRequests("MONTHLY_LIMIT_EXCEEDED", "monthly usage limit exceeded")
	ErrSubscriptionNilInput        = infraerrors.BadRequest("SUBSCRIPTION_NIL_INPUT", "subscription input cannot be nil")
	ErrAdjustWouldExpire           = infraerrors.BadRequest("ADJUST_WOULD_EXPIRE", "adjustment would result in expired subscription (remaining days must be > 0)")
	// 重置配额相关错误
	ErrPaidSubscriptionImmutable    = infraerrors.Forbidden("SUBSCRIPTION_PAID_IMMUTABLE", "paid subscriptions cannot be reset")
	ErrNoLimitsConfigured           = infraerrors.BadRequest("SUBSCRIPTION_NO_LIMITS", "subscription group has no usage limits configured, nothing to reset")
	ErrInvalidResetTarget           = infraerrors.BadRequest("SUBSCRIPTION_INVALID_RESET_TARGET", "selected window cannot be reset (either not configured or is the upper-bound window)")
	ErrDailyLimitResetNotAvailable  = infraerrors.BadRequest("DAILY_LIMIT_RESET_NOT_AVAILABLE", "daily limit reset is not available for this subscription")
	ErrDailyOverdraftNotAvailable   = infraerrors.BadRequest("DAILY_OVERDRAFT_NOT_AVAILABLE", "daily overdraft is not available for this subscription")
	ErrWeekendSkipNotAllowed        = infraerrors.BadRequest("WEEKEND_SKIP_NOT_ALLOWED", "weekend skip is not available for this subscription")
	ErrWeekendSkipAlreadyChanged    = infraerrors.BadRequest("WEEKEND_SKIP_ALREADY_CHANGED", "weekend skip can only be changed once by user")
	ErrWeekendSkipAlreadyEnabled    = infraerrors.BadRequest("WEEKEND_SKIP_ALREADY_ENABLED", "weekend skip is already enabled")
	ErrWeekendSkipDisableNotAllowed = infraerrors.BadRequest("WEEKEND_SKIP_DISABLE_NOT_ALLOWED", "weekend skip cannot be disabled by user")
	ErrSubscriptionWeekendDisabled  = infraerrors.Forbidden("SUBSCRIPTION_WEEKEND_DISABLED", "subscription is not available on weekends")
)

// SubscriptionService 订阅服务
type SubscriptionService struct {
	groupRepo           GroupRepository
	userSubRepo         UserSubscriptionRepository
	billingCacheService *BillingCacheService
	entClient           *dbent.Client

	// L1 缓存：加速中间件热路径的订阅查询
	subCacheL1     *ristretto.Cache
	subCacheGroup  singleflight.Group
	subCacheTTL    time.Duration
	subCacheJitter int // 抖动百分比

	maintenanceQueue *SubscriptionMaintenanceQueue
}

// DailyLimitResetPaymentTarget is the server-side source of truth for a
// user-paid daily quota reset order.
type DailyLimitResetPaymentTarget struct {
	Subscription *UserSubscription
	Group        *Group
	Price        float64
}

type WeekendSkipPreview struct {
	SubscriptionID   int64     `json:"subscription_id"`
	Enabled          bool      `json:"enabled"`
	CurrentExpiresAt time.Time `json:"current_expires_at"`
	PreviewExpiresAt time.Time `json:"preview_expires_at"`
	DeltaSeconds     int64     `json:"delta_seconds"`
	Reason           string    `json:"reason"`
}

// NewSubscriptionService 创建订阅服务
func NewSubscriptionService(groupRepo GroupRepository, userSubRepo UserSubscriptionRepository, billingCacheService *BillingCacheService, entClient *dbent.Client, cfg *config.Config) *SubscriptionService {
	svc := &SubscriptionService{
		groupRepo:           groupRepo,
		userSubRepo:         userSubRepo,
		billingCacheService: billingCacheService,
		entClient:           entClient,
	}
	svc.initSubCache(cfg)
	svc.initMaintenanceQueue(cfg)
	svc.StartSubCacheInvalidationSubscriber(context.Background())
	return svc
}

func (s *SubscriptionService) initMaintenanceQueue(cfg *config.Config) {
	if cfg == nil {
		return
	}
	mc := cfg.SubscriptionMaintenance
	if mc.WorkerCount <= 0 || mc.QueueSize <= 0 {
		return
	}
	s.maintenanceQueue = NewSubscriptionMaintenanceQueue(mc.WorkerCount, mc.QueueSize)
}

// Stop stops the maintenance worker pool.
func (s *SubscriptionService) Stop() {
	if s == nil {
		return
	}
	if s.maintenanceQueue != nil {
		s.maintenanceQueue.Stop()
	}
}

// initSubCache 初始化订阅 L1 缓存
func (s *SubscriptionService) initSubCache(cfg *config.Config) {
	if cfg == nil {
		return
	}
	sc := cfg.SubscriptionCache
	if sc.L1Size <= 0 || sc.L1TTLSeconds <= 0 {
		return
	}
	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: int64(sc.L1Size) * 10,
		MaxCost:     int64(sc.L1Size),
		BufferItems: 64,
	})
	if err != nil {
		log.Printf("Warning: failed to init subscription L1 cache: %v", err)
		return
	}
	s.subCacheL1 = cache
	s.subCacheTTL = time.Duration(sc.L1TTLSeconds) * time.Second
	s.subCacheJitter = sc.JitterPercent
}

// subCacheKey 生成订阅缓存 key（热路径，避免 fmt.Sprintf 开销）
func subCacheKey(userID, groupID int64) string {
	return "sub:" + strconv.FormatInt(userID, 10) + ":" + strconv.FormatInt(groupID, 10)
}

// jitteredTTL 为 TTL 添加抖动，避免集中过期
func (s *SubscriptionService) jitteredTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 || s.subCacheJitter <= 0 {
		return ttl
	}
	pct := s.subCacheJitter
	if pct > 100 {
		pct = 100
	}
	delta := float64(pct) / 100
	factor := 1 - delta + rand.Float64()*(2*delta)
	if factor <= 0 {
		return ttl
	}
	return time.Duration(float64(ttl) * factor)
}

// InvalidateSubCache 失效指定用户+分组的订阅 L1 缓存
func (s *SubscriptionService) InvalidateSubCache(userID, groupID int64) {
	if s.subCacheL1 == nil {
		return
	}
	s.subCacheL1.Del(subCacheKey(userID, groupID))
}

// InvalidateSubCacheSync 失效订阅 L1 缓存并等待 Ristretto 删除操作生效。
func (s *SubscriptionService) InvalidateSubCacheSync(userID, groupID int64) {
	s.invalidateSubCacheKeySync(subCacheKey(userID, groupID))
}

func (s *SubscriptionService) invalidateSubCacheKeySync(key string) {
	if s.subCacheL1 == nil {
		return
	}
	s.subCacheL1.Del(key)
	s.subCacheL1.Wait()
}

// StartSubCacheInvalidationSubscriber 启动跨实例订阅 L1 缓存失效订阅。
func (s *SubscriptionService) StartSubCacheInvalidationSubscriber(ctx context.Context) {
	if s.billingCacheService == nil || s.subCacheL1 == nil {
		return
	}
	if err := s.billingCacheService.SubscribeSubscriptionCacheInvalidation(ctx, func(cacheKey string) {
		s.invalidateSubCacheKeySync(cacheKey)
	}); err != nil {
		log.Printf("Warning: failed to start subscription cache invalidation subscriber: %v", err)
	}
}

func (s *SubscriptionService) invalidateSubscriptionCaches(userID, groupID int64) error {
	s.InvalidateSubCacheSync(userID, groupID)
	if s.billingCacheService == nil {
		return nil
	}

	cacheCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.billingCacheService.InvalidateSubscription(cacheCtx, userID, groupID); err != nil {
		return fmt.Errorf("invalidate billing subscription cache: %w", err)
	}
	if err := s.billingCacheService.PublishSubscriptionCacheInvalidation(cacheCtx, subCacheKey(userID, groupID)); err != nil {
		return fmt.Errorf("publish subscription cache invalidation: %w", err)
	}
	return nil
}

// AssignSubscriptionInput 分配订阅输入
type AssignSubscriptionInput struct {
	UserID       int64
	GroupID      int64
	ValidityDays int
	// ValidityUnit is the original unit from the subscription plan/card.
	// Empty means day for backward compatibility with admin/redeem/manual grants.
	ValidityUnit string
	StartsAt     *time.Time
	ExpiresAt    *time.Time
	AssignedBy   int64
	Notes        string
	// RestartPeriod 为 true 时，已有未过期订阅会从当前时间重新开一个新周期：
	// starts_at=now、expires_at=now+validityDays，并重置已配置窗口的用量。
	RestartPeriod bool
	// Source 标识订阅来源（admin/redeem/payment）。
	// 留空时按 AssignedBy 推断：>0 → admin，==0 → redeem。
	// payment 入口必须显式传 SubscriptionSourcePayment。
	Source string
	// QuotaLimitSpecified indicates that this assignment carries an explicit
	// period-level total quota. QuotaLimitUSD == nil with this flag set means
	// "unlimited / no total quota" for the purchased plan.
	QuotaLimitSpecified bool
	QuotaLimitUSD       *float64
}

// AssignSubscription 分配订阅给用户（不允许重复分配）
func (s *SubscriptionService) AssignSubscription(ctx context.Context, input *AssignSubscriptionInput) (*UserSubscription, error) {
	sub, _, err := s.assignSubscriptionWithReuse(ctx, input)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

// AssignOrExtendSubscription 分配或续期订阅（用于兑换码等场景）
// 如果用户已有同分组的订阅：
//   - 未过期：从当前过期时间累加天数
//   - 已过期：从当前时间开始计算新的过期时间，并激活订阅
//
// 如果没有订阅：创建新订阅
func (s *SubscriptionService) AssignOrExtendSubscription(ctx context.Context, input *AssignSubscriptionInput) (*UserSubscription, bool, error) {
	// 检查分组是否存在且为订阅类型
	group, err := s.groupRepo.GetByID(ctx, input.GroupID)
	if err != nil {
		return nil, false, fmt.Errorf("group not found: %w", err)
	}
	if !group.IsSubscriptionType() {
		return nil, false, ErrGroupNotSubscriptionType
	}

	// 查询是否已有订阅
	existingSub, err := s.userSubRepo.GetByUserIDAndGroupID(ctx, input.UserID, input.GroupID)
	if err != nil {
		// 不存在记录是正常情况，其他错误需要返回
		existingSub = nil
	}

	validityDays := input.ValidityDays
	if validityDays <= 0 {
		validityDays = 30
	}
	if validityDays > MaxValidityDays {
		validityDays = MaxValidityDays
	}

	// 已有订阅，执行续期（在事务中完成所有更新）
	if existingSub != nil {
		now := time.Now()
		if input.StartsAt != nil || input.ExpiresAt != nil {
			startsAt, expiresAt, err := resolveAssignTimeRange(input, now)
			if err != nil {
				return nil, false, err
			}
			return s.assignExistingSubscriptionTimeRange(ctx, input, existingSub, startsAt, expiresAt, now)
		}
		var newExpiresAt time.Time

		renewDuration := time.Duration(validityDays) * 24 * time.Hour
		if existingSub.ExpiresAt.After(now) {
			// 未过期：从当前过期时间累加
			if existingSub.SkipWeekends {
				newExpiresAt = addWeekendSkippedDuration(existingSub.ExpiresAt, renewDuration)
			} else {
				newExpiresAt = existingSub.ExpiresAt.AddDate(0, 0, validityDays)
			}
		} else {
			// 已过期：从当前时间开始计算
			if existingSub.SkipWeekends {
				newExpiresAt = addWeekendSkippedDuration(now, renewDuration)
			} else {
				newExpiresAt = now.AddDate(0, 0, validityDays)
			}
		}

		// 确保不超过最大过期时间
		if newExpiresAt.After(MaxExpiresAt) {
			newExpiresAt = MaxExpiresAt
		}

		// 开启事务：Update/UpdateStatus/UpdateNotes 在同一事务中完成。
		// 单元测试和部分非 ent 场景会用 nil entClient，此时直接复用原 ctx。
		txCtx, rollbackTx, commitTx, err := s.beginSubscriptionUpdateTx(ctx)
		if err != nil {
			return nil, false, fmt.Errorf("begin transaction: %w", err)
		}

		previousExpiresAt := existingSub.ExpiresAt
		wasExpired := !existingSub.ExpiresAt.After(now)
		wasQuotaExhausted := existingSub.Status == SubscriptionStatusQuotaExhausted
		existingSub.ExpiresAt = newExpiresAt
		existingSub.ValidityUnit = resolveSubscriptionValidityUnit(input.ValidityUnit, existingSub.ValidityUnit)
		s.advanceWeekendSkipOriginalExpiresAt(existingSub, previousExpiresAt, now, validityDays, wasExpired || wasQuotaExhausted || input.RestartPeriod)

		// Reactivating an expired / quota-exhausted subscription or explicitly
		// restarting a card must move the billing-window anchor to this purchase
		// time; otherwise old windows keep using stale starts_at and the renewed
		// subscription can remain immediately unusable.
		if wasExpired || wasQuotaExhausted || input.RestartPeriod {
			if (input.RestartPeriod || wasQuotaExhausted) && !wasExpired {
				newExpiresAt = now.AddDate(0, 0, validityDays)
				if newExpiresAt.After(MaxExpiresAt) {
					newExpiresAt = MaxExpiresAt
				}
			}
			existingSub.StartsAt = now
			existingSub.ExpiresAt = newExpiresAt
			existingSub.Status = SubscriptionStatusActive
			s.advanceWeekendSkipOriginalExpiresAt(existingSub, previousExpiresAt, now, validityDays, true)
			existingSub.DailyWindowStart = nil
			existingSub.WeeklyWindowStart = nil
			existingSub.MonthlyWindowStart = nil
			if validityDays == 1 {
				windowStart := now
				existingSub.DailyWindowStart = &windowStart
			}
			existingSub.DailyUsageUSD = 0
			existingSub.WeeklyUsageUSD = 0
			existingSub.MonthlyUsageUSD = 0
			applyFreshPeriodQuota(existingSub, input)
			existingSub.UpdatedAt = now
			if err := s.userSubRepo.Update(txCtx, existingSub); err != nil {
				rollbackTx()
				return nil, false, fmt.Errorf("reset expired subscription window anchor: %w", err)
			}
		} else if input.Source == domain.SubscriptionSourcePayment && validityDays == 1 && group.HasDailyLimit() {
			// A paid 1-day renewal behaves like opening one fresh day card: reset the
			// daily window immediately and cap the renewed period to now+1 day. If we
			// both reset usage and cumulatively extend the old expiry, users can receive
			// an extra automatic daily reset before the extended expiry. Multi-day
			// renewals only extend the current subscription and must not reset in-period
			// usage. In daily-overdraft mode, the period pool is derived from
			// expires_at-starts_at, so never shrink an active subscription's expiry
			// while opening the fresh daily card.
			cappedExpiresAt := now.AddDate(0, 0, validityDays)
			if existingSub.AllowsDailyOverdraft(group) && cappedExpiresAt.Before(existingSub.ExpiresAt) {
				newExpiresAt = existingSub.ExpiresAt
			} else {
				newExpiresAt = cappedExpiresAt
			}
			if newExpiresAt.After(MaxExpiresAt) {
				newExpiresAt = MaxExpiresAt
			}
			existingSub.ExpiresAt = newExpiresAt
			applyExtensionQuota(existingSub, input)
			if !existingSub.AllowsDailyOverdraft(group) {
				existingSub.WeeklyUsageUSD = 0
				existingSub.MonthlyUsageUSD = 0
			}
			if err := s.userSubRepo.Update(txCtx, existingSub); err != nil {
				rollbackTx()
				return nil, false, fmt.Errorf("cap 1-day renewal expiry: %w", err)
			}
			dailyWindowStart := now
			if existingSub.AllowsDailyOverdraft(group) {
				dailyWindowStart = existingSub.CurrentDailyWindowStart(now)
			}
			if err := s.userSubRepo.ResetDailyUsage(txCtx, existingSub.ID, existingSub.DailyWindowStart, dailyWindowStart); err != nil {
				rollbackTx()
				return nil, false, fmt.Errorf("reset daily usage on subscription renewal: %w", err)
			}
			existingSub.DailyWindowStart = &dailyWindowStart
			existingSub.DailyUsageUSD = 0
		} else {
			// 普通续期只延长过期时间，同时保存本次订阅来源的原始有效单位。后者只影响日透支
			// 统计口径：day 按每日额度到期计入，week/month 继续按真实累计用量。
			applyExtensionQuota(existingSub, input)
			if err := s.userSubRepo.Update(txCtx, existingSub); err != nil {
				rollbackTx()
				return nil, false, fmt.Errorf("extend subscription: %w", err)
			}
		}

		// Restore active status when a non-active subscription is renewed.
		if existingSub.ExpiresAt.After(now) && existingSub.Status != SubscriptionStatusActive {
			if err := s.userSubRepo.UpdateStatus(txCtx, existingSub.ID, SubscriptionStatusActive); err != nil {
				rollbackTx()
				return nil, false, fmt.Errorf("update subscription status: %w", err)
			}
		}

		// 追加备注
		if input.Notes != "" {
			newNotes := existingSub.Notes
			if newNotes != "" {
				newNotes += "\n"
			}
			newNotes += input.Notes
			if err := s.userSubRepo.UpdateNotes(txCtx, existingSub.ID, newNotes); err != nil {
				rollbackTx()
				return nil, false, fmt.Errorf("update subscription notes: %w", err)
			}
		}

		// 提交事务
		if err := commitTx(); err != nil {
			return nil, false, fmt.Errorf("commit transaction: %w", err)
		}

		// 失效订阅缓存。外层事务未提交时由事务拥有者提交后统一清理。
		s.maybeInvalidateAssignmentCaches(input.UserID, input.GroupID, dbent.TxFromContext(ctx) != nil)

		// 返回更新后的订阅
		sub, err := s.userSubRepo.GetByID(ctx, existingSub.ID)
		return sub, true, err // true 表示是续期
	}

	// 没有订阅，创建新订阅
	sub, err := s.createSubscription(ctx, input)
	if err != nil {
		return nil, false, err
	}

	// 失效订阅缓存。外层事务未提交时由事务拥有者提交后统一清理。
	s.maybeInvalidateAssignmentCaches(input.UserID, input.GroupID, dbent.TxFromContext(ctx) != nil)

	return sub, false, nil // false 表示是新建
}

func (s *SubscriptionService) assignExistingSubscriptionTimeRange(ctx context.Context, input *AssignSubscriptionInput, existingSub *UserSubscription, startsAt, expiresAt, now time.Time) (*UserSubscription, bool, error) {
	txCtx, rollbackTx, commitTx, err := s.beginSubscriptionUpdateTx(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("begin transaction: %w", err)
	}

	wasExpired := !existingSub.ExpiresAt.After(now)
	wasQuotaExhausted := existingSub.Status == SubscriptionStatusQuotaExhausted
	nextValidityUnit := resolveSubscriptionValidityUnit(input.ValidityUnit, existingSub.ValidityUnit)
	if wasExpired || wasQuotaExhausted || input.RestartPeriod {
		existingSub.StartsAt = startsAt
		existingSub.ExpiresAt = expiresAt
		existingSub.Status = SubscriptionStatusActive
		existingSub.ValidityUnit = nextValidityUnit
		s.setWeekendSkipOriginalExpiresAtForRange(existingSub, expiresAt)
		existingSub.DailyWindowStart = nil
		existingSub.WeeklyWindowStart = nil
		existingSub.MonthlyWindowStart = nil
		existingSub.DailyUsageUSD = 0
		existingSub.WeeklyUsageUSD = 0
		existingSub.MonthlyUsageUSD = 0
		applyFreshPeriodQuota(existingSub, input)
	} else if expiresAt.After(existingSub.ExpiresAt) {
		existingSub.ExpiresAt = expiresAt
		existingSub.ValidityUnit = nextValidityUnit
		s.setWeekendSkipOriginalExpiresAtForRange(existingSub, expiresAt)
	}
	if !(wasExpired || wasQuotaExhausted || input.RestartPeriod) {
		applyExtensionQuota(existingSub, input)
	}
	existingSub.UpdatedAt = now
	if err := s.userSubRepo.Update(txCtx, existingSub); err != nil {
		rollbackTx()
		return nil, false, fmt.Errorf("adjust subscription time range: %w", err)
	}

	if existingSub.ExpiresAt.After(now) && existingSub.Status != SubscriptionStatusActive {
		if err := s.userSubRepo.UpdateStatus(txCtx, existingSub.ID, SubscriptionStatusActive); err != nil {
			rollbackTx()
			return nil, false, fmt.Errorf("update subscription status: %w", err)
		}
	}

	if input.Notes != "" {
		newNotes := existingSub.Notes
		if newNotes != "" {
			newNotes += "\n"
		}
		newNotes += input.Notes
		if err := s.userSubRepo.UpdateNotes(txCtx, existingSub.ID, newNotes); err != nil {
			rollbackTx()
			return nil, false, fmt.Errorf("update subscription notes: %w", err)
		}
	}

	if err := commitTx(); err != nil {
		return nil, false, fmt.Errorf("commit transaction: %w", err)
	}

	s.maybeInvalidateAssignmentCaches(input.UserID, input.GroupID, dbent.TxFromContext(ctx) != nil)

	sub, err := s.userSubRepo.GetByID(ctx, existingSub.ID)
	return sub, true, err
}

func (s *SubscriptionService) beginSubscriptionUpdateTx(ctx context.Context) (context.Context, func(), func() error, error) {
	if dbent.TxFromContext(ctx) != nil {
		return ctx, func() {}, func() error { return nil }, nil
	}
	if s.entClient == nil {
		return ctx, func() {}, func() error { return nil }, nil
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, nil, nil, err
	}

	return dbent.NewTxContext(ctx, tx), func() {
		_ = tx.Rollback()
	}, tx.Commit, nil
}

func (s *SubscriptionService) maybeInvalidateAssignmentCaches(userID, groupID int64, deferred bool) {
	// When an outer transaction owns this assignment, it must invalidate after
	// commit. Invalidating inside that transaction can reload pre-commit state
	// into Redis/L1 and keep the renewed subscription stale.
	if deferred {
		return
	}
	if err := s.invalidateSubscriptionCaches(userID, groupID); err != nil {
		log.Printf("Warning: invalidate subscription assignment cache failed user=%d group=%d: %v", userID, groupID, err)
	}
}

func (s *SubscriptionService) withSubscriptionUpdateTx(ctx context.Context, fn func(context.Context) error) error {
	if dbent.TxFromContext(ctx) != nil {
		return fn(ctx)
	}
	txCtx, rollbackTx, commitTx, err := s.beginSubscriptionUpdateTx(ctx)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			rollbackTx()
		}
	}()
	if err := fn(txCtx); err != nil {
		return err
	}
	if err := commitTx(); err != nil {
		return err
	}
	committed = true
	return nil
}

func (s *SubscriptionService) advanceWeekendSkipOriginalExpiresAt(sub *UserSubscription, previousExpiresAt, restartAnchor time.Time, validityDays int, restart bool) {
	if sub == nil || !sub.SkipWeekends || validityDays <= 0 {
		return
	}
	var base time.Time
	if restart {
		base = restartAnchor
	} else {
		base = inferWeekendSkipNaturalExpiresAt(sub, previousExpiresAt)
	}
	original := base.AddDate(0, 0, validityDays)
	if original.After(MaxExpiresAt) {
		original = MaxExpiresAt
	}
	sub.WeekendSkipOriginalExpiresAt = &original
}

func (s *SubscriptionService) setWeekendSkipOriginalExpiresAtForRange(sub *UserSubscription, expiresAt time.Time) {
	if sub == nil || !sub.SkipWeekends {
		return
	}
	original := expiresAt
	sub.WeekendSkipOriginalExpiresAt = &original
}

func inferWeekendSkipNaturalExpiresAt(sub *UserSubscription, skippedExpiresAt time.Time) time.Time {
	if sub == nil {
		return skippedExpiresAt
	}
	base := time.Time{}
	if sub.WeekendSkipOriginalExpiresAt != nil && sub.WeekendSkipOriginalExpiresAt.After(sub.StartsAt) {
		base = *sub.WeekendSkipOriginalExpiresAt
	}
	if skippedExpiresAt.After(sub.StartsAt) {
		usable := weekendSkippedDurationBetween(sub.StartsAt, skippedExpiresAt)
		if usable > 0 {
			days := int(math.Ceil(usable.Hours() / 24))
			if days < 1 {
				days = 1
			}
			inferred := sub.StartsAt.AddDate(0, 0, days)
			if inferred.After(base) {
				base = inferred
			}
		}
	}
	if base.IsZero() {
		base = skippedExpiresAt
	}
	return base
}

func cloneQuotaLimitPtr(v *float64) *float64 {
	if v == nil {
		return nil
	}
	cp := *v
	return &cp
}

func applyNewSubscriptionQuota(sub *UserSubscription, input *AssignSubscriptionInput) {
	if sub == nil || input == nil || !input.QuotaLimitSpecified {
		return
	}
	sub.QuotaLimitUSD = cloneQuotaLimitPtr(input.QuotaLimitUSD)
	sub.QuotaUsedUSD = 0
}

func applyFreshPeriodQuota(sub *UserSubscription, input *AssignSubscriptionInput) {
	if sub == nil || input == nil {
		return
	}
	if input.QuotaLimitSpecified {
		sub.QuotaLimitUSD = cloneQuotaLimitPtr(input.QuotaLimitUSD)
	}
	sub.QuotaUsedUSD = 0
}

func applyExtensionQuota(sub *UserSubscription, input *AssignSubscriptionInput) {
	if sub == nil || input == nil || !input.QuotaLimitSpecified {
		return
	}
	if input.QuotaLimitUSD == nil {
		sub.QuotaLimitUSD = nil
		sub.QuotaUsedUSD = 0
		return
	}
	grant := *input.QuotaLimitUSD
	if grant <= 0 {
		sub.QuotaLimitUSD = nil
		sub.QuotaUsedUSD = 0
		return
	}
	if sub.QuotaLimitUSD == nil || *sub.QuotaLimitUSD <= 0 {
		sub.QuotaLimitUSD = cloneQuotaLimitPtr(input.QuotaLimitUSD)
		return
	}
	next := *sub.QuotaLimitUSD + grant
	sub.QuotaLimitUSD = &next
}

// createSubscription 创建新订阅（内部方法）
func (s *SubscriptionService) createSubscription(ctx context.Context, input *AssignSubscriptionInput) (*UserSubscription, error) {
	now := time.Now()
	startsAt, expiresAt, err := resolveAssignTimeRange(input, now)
	if err != nil {
		return nil, err
	}

	sub := &UserSubscription{
		UserID:       input.UserID,
		GroupID:      input.GroupID,
		StartsAt:     startsAt,
		ExpiresAt:    expiresAt,
		Status:       SubscriptionStatusActive,
		ValidityUnit: normalizeSubscriptionValidityUnit(input.ValidityUnit),
		AssignedAt:   now,
		Notes:        input.Notes,
		Source:       resolveSubscriptionSource(input.Source, input.AssignedBy),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	applyNewSubscriptionQuota(sub, input)
	// 只有当 AssignedBy > 0 时才设置（0 表示系统分配，如兑换码）
	if input.AssignedBy > 0 {
		sub.AssignedBy = &input.AssignedBy
	}

	if err := s.userSubRepo.Create(ctx, sub); err != nil {
		return nil, err
	}

	// 重新获取完整订阅信息（包含关联）
	return s.userSubRepo.GetByID(ctx, sub.ID)
}

// BulkAssignSubscriptionInput 批量分配订阅输入
type BulkAssignSubscriptionInput struct {
	UserIDs      []int64
	GroupID      int64
	ValidityDays int
	ValidityUnit string
	AssignedBy   int64
	Notes        string
	// Source 同 AssignSubscriptionInput.Source；批量场景默认 admin。
	Source string
}

// BulkAssignResult 批量分配结果
type BulkAssignResult struct {
	SuccessCount  int
	CreatedCount  int
	ReusedCount   int
	FailedCount   int
	Subscriptions []UserSubscription
	Errors        []string
	Statuses      map[int64]string
}

// BulkAssignSubscription 批量分配订阅
func (s *SubscriptionService) BulkAssignSubscription(ctx context.Context, input *BulkAssignSubscriptionInput) (*BulkAssignResult, error) {
	result := &BulkAssignResult{
		Subscriptions: make([]UserSubscription, 0),
		Errors:        make([]string, 0),
		Statuses:      make(map[int64]string),
	}

	for _, userID := range input.UserIDs {
		sub, reused, err := s.assignSubscriptionWithReuse(ctx, &AssignSubscriptionInput{
			UserID:       userID,
			GroupID:      input.GroupID,
			ValidityDays: input.ValidityDays,
			ValidityUnit: input.ValidityUnit,
			AssignedBy:   input.AssignedBy,
			Notes:        input.Notes,
			Source:       input.Source,
		})
		if err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, fmt.Sprintf("user %d: %v", userID, err))
			result.Statuses[userID] = "failed"
		} else {
			result.SuccessCount++
			result.Subscriptions = append(result.Subscriptions, *sub)
			if reused {
				result.ReusedCount++
				result.Statuses[userID] = "reused"
			} else {
				result.CreatedCount++
				result.Statuses[userID] = "created"
			}
		}
	}

	return result, nil
}

func (s *SubscriptionService) assignSubscriptionWithReuse(ctx context.Context, input *AssignSubscriptionInput) (*UserSubscription, bool, error) {
	// 检查分组是否存在且为订阅类型
	group, err := s.groupRepo.GetByID(ctx, input.GroupID)
	if err != nil {
		return nil, false, fmt.Errorf("group not found: %w", err)
	}
	if !group.IsSubscriptionType() {
		return nil, false, ErrGroupNotSubscriptionType
	}

	// 检查是否已存在订阅；若已存在，则按幂等成功返回现有订阅
	exists, err := s.userSubRepo.ExistsByUserIDAndGroupID(ctx, input.UserID, input.GroupID)
	if err != nil {
		return nil, false, err
	}
	if exists {
		sub, getErr := s.userSubRepo.GetByUserIDAndGroupID(ctx, input.UserID, input.GroupID)
		if getErr != nil {
			return nil, false, getErr
		}
		if conflictReason, conflict := detectAssignSemanticConflict(sub, input); conflict {
			return nil, false, ErrSubscriptionAssignConflict.WithMetadata(map[string]string{
				"conflict_reason": conflictReason,
			})
		}
		return sub, true, nil
	}

	sub, err := s.createSubscription(ctx, input)
	if err != nil {
		return nil, false, err
	}

	// 失效订阅缓存。外层事务未提交时由事务拥有者提交后统一清理。
	s.maybeInvalidateAssignmentCaches(input.UserID, input.GroupID, dbent.TxFromContext(ctx) != nil)

	return sub, false, nil
}

func detectAssignSemanticConflict(existing *UserSubscription, input *AssignSubscriptionInput) (string, bool) {
	if existing == nil || input == nil {
		return "", false
	}

	if input.StartsAt != nil || input.ExpiresAt != nil {
		if input.StartsAt == nil || input.ExpiresAt == nil {
			return "time_range_incomplete", true
		}
		if !existing.StartsAt.Equal(*input.StartsAt) {
			return "starts_at_mismatch", true
		}
		if !existing.ExpiresAt.Equal(*input.ExpiresAt) {
			return "expires_at_mismatch", true
		}
	} else {
		normalizedDays := normalizeAssignValidityDays(input.ValidityDays)
		if !existing.StartsAt.IsZero() {
			expectedExpiresAt := existing.StartsAt.AddDate(0, 0, normalizedDays)
			if expectedExpiresAt.After(MaxExpiresAt) {
				expectedExpiresAt = MaxExpiresAt
			}
			if !existing.ExpiresAt.Equal(expectedExpiresAt) {
				return "validity_days_mismatch", true
			}
		}
	}

	existingNotes := strings.TrimSpace(existing.Notes)
	inputNotes := strings.TrimSpace(input.Notes)
	if existingNotes != inputNotes {
		return "notes_mismatch", true
	}

	return "", false
}

func normalizeAssignValidityDays(days int) int {
	if days <= 0 {
		days = 30
	}
	if days > MaxValidityDays {
		days = MaxValidityDays
	}
	return days
}

func validateExplicitTimeRange(startsAt, expiresAt, now time.Time) error {
	if !expiresAt.After(startsAt) {
		return infraerrors.BadRequest("INVALID_TIME_RANGE", "expires_at must be later than starts_at")
	}
	if !expiresAt.After(now) {
		return infraerrors.BadRequest("INVALID_TIME_RANGE", "expires_at must be later than now")
	}
	if expiresAt.After(MaxExpiresAt) {
		return infraerrors.BadRequest("INVALID_TIME_RANGE", "expires_at exceeds supported maximum (2099-12-31T23:59:59Z)")
	}
	return nil
}

func resolveAssignTimeRange(input *AssignSubscriptionInput, now time.Time) (time.Time, time.Time, error) {
	if input == nil {
		return time.Time{}, time.Time{}, ErrSubscriptionNilInput
	}
	hasStart := input.StartsAt != nil
	hasEnd := input.ExpiresAt != nil
	if hasStart != hasEnd {
		return time.Time{}, time.Time{}, infraerrors.BadRequest("INVALID_TIME_RANGE", "starts_at and expires_at must be provided together")
	}
	if hasStart && hasEnd {
		startsAt := *input.StartsAt
		expiresAt := *input.ExpiresAt
		if err := validateExplicitTimeRange(startsAt, expiresAt, now); err != nil {
			return time.Time{}, time.Time{}, err
		}
		return startsAt, expiresAt, nil
	}

	validityDays := normalizeAssignValidityDays(input.ValidityDays)
	startsAt := now
	expiresAt := now.AddDate(0, 0, validityDays)
	if expiresAt.After(MaxExpiresAt) {
		expiresAt = MaxExpiresAt
	}
	return startsAt, expiresAt, nil
}

// AdjustSubscriptionTimeRange 将订阅直接调整为显式时间段（开始/结束时间）。
func (s *SubscriptionService) AdjustSubscriptionTimeRange(ctx context.Context, subscriptionID int64, startsAt, expiresAt time.Time) (*UserSubscription, error) {
	sub, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, ErrSubscriptionNotFound
	}

	now := time.Now()
	if err := validateExplicitTimeRange(startsAt, expiresAt, now); err != nil {
		return nil, err
	}

	sub.StartsAt = startsAt
	sub.ExpiresAt = expiresAt
	if sub.Status == SubscriptionStatusExpired && expiresAt.After(now) {
		sub.Status = SubscriptionStatusActive
	}

	if err := s.userSubRepo.Update(ctx, sub); err != nil {
		return nil, err
	}

	s.invalidateSubscriptionRuntimeCache(ctx, sub)

	return s.userSubRepo.GetByID(ctx, subscriptionID)
}

// RevokeSubscription 撤销订阅
func (s *SubscriptionService) RevokeSubscription(ctx context.Context, subscriptionID int64) error {
	// 先获取订阅信息用于失效缓存
	sub, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return err
	}

	if err := s.userSubRepo.Delete(ctx, subscriptionID); err != nil {
		return err
	}

	if err := s.invalidateSubscriptionCaches(sub.UserID, sub.GroupID); err != nil {
		return err
	}

	return nil
}

// ExtendSubscription 调整订阅时长（正数延长，负数缩短）
func (s *SubscriptionService) ExtendSubscription(ctx context.Context, subscriptionID int64, days int) (*UserSubscription, error) {
	sub, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, ErrSubscriptionNotFound
	}

	// 限制调整天数范围
	if days > MaxValidityDays {
		days = MaxValidityDays
	}
	if days < -MaxValidityDays {
		days = -MaxValidityDays
	}

	now := time.Now()
	isExpired := !sub.ExpiresAt.After(now)

	// 如果订阅已过期，不允许负向调整
	if isExpired && days < 0 {
		return nil, infraerrors.BadRequest("CANNOT_SHORTEN_EXPIRED", "cannot shorten an expired subscription")
	}

	// 计算新的过期时间
	var newExpiresAt time.Time
	if sub.SkipWeekends && days > 0 {
		adjustDuration := time.Duration(days) * 24 * time.Hour
		if isExpired {
			newExpiresAt = addWeekendSkippedDuration(now, adjustDuration)
		} else {
			newExpiresAt = addWeekendSkippedDuration(sub.ExpiresAt, adjustDuration)
		}
	} else if isExpired {
		// 已过期：从当前时间开始增加天数
		newExpiresAt = now.AddDate(0, 0, days)
	} else {
		// 未过期：从原过期时间增加/减少天数
		newExpiresAt = sub.ExpiresAt.AddDate(0, 0, days)
	}

	if newExpiresAt.After(MaxExpiresAt) {
		newExpiresAt = MaxExpiresAt
	}

	// 检查新的过期时间必须大于当前时间
	if !newExpiresAt.After(now) {
		return nil, ErrAdjustWouldExpire
	}

	if err := s.userSubRepo.ExtendExpiry(ctx, subscriptionID, newExpiresAt); err != nil {
		return nil, err
	}

	// 如果订阅已过期，恢复为active状态
	if sub.Status == SubscriptionStatusExpired || sub.Status == SubscriptionStatusQuotaExhausted {
		if err := s.userSubRepo.UpdateStatus(ctx, subscriptionID, SubscriptionStatusActive); err != nil {
			return nil, err
		}
	}

	s.invalidateSubscriptionRuntimeCache(ctx, sub)

	return s.userSubRepo.GetByID(ctx, subscriptionID)
}

// GetByID 根据ID获取订阅
func (s *SubscriptionService) GetByID(ctx context.Context, id int64) (*UserSubscription, error) {
	return s.userSubRepo.GetByID(ctx, id)
}

// GetActiveSubscription 获取用户对特定分组的有效订阅
// 使用 L1 缓存 + singleflight 加速中间件热路径。
// 返回缓存对象的浅拷贝，调用方可安全修改字段而不会污染缓存或触发 data race。
func (s *SubscriptionService) GetActiveSubscription(ctx context.Context, userID, groupID int64) (*UserSubscription, error) {
	key := subCacheKey(userID, groupID)

	// L1 缓存命中：返回浅拷贝
	if s.subCacheL1 != nil {
		if v, ok := s.subCacheL1.Get(key); ok {
			if sub, ok := v.(*UserSubscription); ok {
				cp := *sub
				return &cp, nil
			}
		}
	}

	// singleflight 防止并发击穿
	value, err, _ := s.subCacheGroup.Do(key, func() (any, error) {
		sub, err := s.userSubRepo.GetActiveByUserIDAndGroupID(ctx, userID, groupID)
		if err != nil {
			if !errors.Is(err, ErrSubscriptionNotFound) {
				return nil, err // 直接透传 repo 已翻译的错误（NotFound → ErrSubscriptionNotFound，其他错误原样返回）
			}
			reactivated, reactivateErr := s.reactivateQuotaExhaustedSubscriptionIfRecoverable(ctx, userID, groupID)
			if reactivateErr != nil || reactivated == nil {
				return nil, err
			}
			sub = reactivated
		}
		// 写入 L1 缓存
		if s.subCacheL1 != nil {
			_ = s.subCacheL1.SetWithTTL(key, sub, 1, s.jitteredTTL(s.subCacheTTL))
		}
		return sub, nil
	})
	if err != nil {
		return nil, err
	}
	// singleflight 返回的也是缓存指针，需要浅拷贝
	sub, ok := value.(*UserSubscription)
	if !ok || sub == nil {
		return nil, ErrSubscriptionNotFound
	}
	cp := *sub
	return &cp, nil
}

// reactivateQuotaExhaustedSubscriptionIfRecoverable handles subscriptions that
// were marked quota_exhausted by a previous request but whose rolling usage
// windows have since expired. The hot auth path first queries only active
// subscriptions for speed; without this recovery pass, quota_exhausted rows can
// never reach CheckAndResetWindows and remain stuck until manual intervention.
func (s *SubscriptionService) reactivateQuotaExhaustedSubscriptionIfRecoverable(ctx context.Context, userID, groupID int64) (*UserSubscription, error) {
	if s == nil || s.userSubRepo == nil {
		return nil, ErrSubscriptionNotFound
	}
	sub, err := s.userSubRepo.GetByUserIDAndGroupID(ctx, userID, groupID)
	if err != nil {
		return nil, err
	}
	if sub.Status != SubscriptionStatusQuotaExhausted {
		return nil, ErrSubscriptionNotFound
	}

	now := time.Now()
	if now.Before(sub.StartsAt) {
		return nil, ErrSubscriptionNotStarted
	}
	if !sub.ExpiresAt.After(now) {
		return nil, ErrSubscriptionExpired
	}

	group := sub.Group
	if group == nil && s.groupRepo != nil {
		group, err = s.groupRepo.GetByID(ctx, sub.GroupID)
		if err != nil {
			return nil, err
		}
		sub.Group = group
	}
	if group == nil {
		return nil, ErrSubscriptionQuotaExhausted
	}

	if err := s.CheckAndResetWindows(ctx, sub); err != nil {
		return nil, err
	}
	if !sub.CheckTotalQuota(0) || !sub.CheckDailyLimit(group, 0) || !sub.CheckWeeklyLimit(group, 0) || !sub.CheckMonthlyLimit(group, 0) {
		return nil, ErrSubscriptionQuotaExhausted
	}

	if err := s.userSubRepo.UpdateStatus(ctx, sub.ID, SubscriptionStatusActive); err != nil {
		return nil, err
	}
	sub.Status = SubscriptionStatusActive

	s.invalidateSubscriptionRuntimeCache(ctx, sub)
	return sub, nil
}

// ResolveSubscriptionError 在 GetActiveSubscription 返回 ErrSubscriptionNotFound 时被调用，
// 通过宽松查询（不限状态/过期）回查该用户在该分组下的订阅记录，推断出更准确的错误原因：
//   - 记录不存在（撤销=物理删除 或 从未订阅）→ 返回 ErrSubscriptionNotFound
//   - 状态为 expired，或 active 但 expires_at 已过 → 返回 ErrSubscriptionExpired
//   - 状态为 suspended → 返回 ErrSubscriptionSuspended
//   - starts_at 在未来 → 返回 ErrSubscriptionNotStarted
//   - 其他异常 → 返回 ErrSubscriptionNotFound 兜底
//
// 此方法不走 L1 缓存，仅用于错误诊断（冷路径），频次低且只在 active 查询失败后触发。
func (s *SubscriptionService) ResolveSubscriptionError(ctx context.Context, userID, groupID int64) error {
	sub, err := s.userSubRepo.GetByUserIDAndGroupID(ctx, userID, groupID)
	if err != nil {
		// 记录确实不存在（撤销=物理删除 或 从未订阅过此分组），统一为 NotFound
		return ErrSubscriptionNotFound
	}
	now := time.Now()
	switch sub.Status {
	case SubscriptionStatusExpired:
		return ErrSubscriptionExpired
	case SubscriptionStatusSuspended:
		return ErrSubscriptionSuspended
	case SubscriptionStatusQuotaExhausted:
		return ErrSubscriptionQuotaExhausted
	case SubscriptionStatusActive:
		// 状态是 active，但 GetActiveSubscription 失败，说明 expires_at 已过或 starts_at 未到
		if !sub.ExpiresAt.After(now) {
			return ErrSubscriptionExpired
		}
		if now.Before(sub.StartsAt) {
			return ErrSubscriptionNotStarted
		}
		// 理论上不会落到这里（active + 未过期 + 已开始 应能被 GetActiveSubscription 命中）
		return ErrSubscriptionNotFound
	default:
		return ErrSubscriptionNotFound
	}
}

func (s *SubscriptionService) SetUserDailyOverdraft(ctx context.Context, userID, subscriptionID int64, enabled bool) (*UserSubscription, error) {
	if subscriptionID <= 0 {
		return nil, ErrSubscriptionNotFound
	}
	sub, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}
	if sub.UserID != userID {
		return nil, ErrSubscriptionNotFound
	}
	now := time.Now()
	if sub.Status != SubscriptionStatusActive || !sub.ExpiresAt.After(now) || now.Before(sub.StartsAt) {
		return nil, ErrSubscriptionNotFound
	}
	if enabled {
		if sub.Group == nil || !sub.Group.AllowsDailyOverdraft() {
			return nil, ErrDailyOverdraftNotAvailable
		}
		if _, ok := sub.DailyOverdraftLimitUSD(sub.Group); !ok {
			return nil, ErrDailyOverdraftNotAvailable
		}
	}
	if sub.AllowDailyOverdraft == enabled {
		return sub, nil
	}
	if err := s.userSubRepo.UpdateDailyOverdraft(ctx, sub.ID, enabled); err != nil {
		return nil, err
	}
	s.invalidateSubscriptionRuntimeCache(ctx, sub)
	return s.userSubRepo.GetByID(ctx, sub.ID)
}

func isWeekendTime(t time.Time) bool {
	t = t.In(timezone.Location())
	return t.Weekday() == time.Saturday || t.Weekday() == time.Sunday
}

var weekendSkipNow = timezone.Now

func nextWorkingTime(t time.Time) time.Time {
	loc := timezone.Location()
	t = t.In(loc)
	for isWeekendTime(t) {
		start := timezone.StartOfDay(t)
		t = start.AddDate(0, 0, 1)
	}
	return t
}

func addWeekendSkippedDuration(start time.Time, duration time.Duration) time.Time {
	if duration <= 0 {
		return start
	}
	current := nextWorkingTime(start)
	remaining := duration
	for remaining > 0 {
		if isWeekendTime(current) {
			current = nextWorkingTime(current)
			continue
		}
		dayEnd := timezone.StartOfDay(current).AddDate(0, 0, 1)
		available := dayEnd.Sub(current)
		if available <= 0 {
			current = dayEnd
			continue
		}
		if remaining <= available {
			return current.Add(remaining)
		}
		remaining -= available
		current = dayEnd
	}
	return current
}

func weekendSkippedDurationBetween(start, end time.Time) time.Duration {
	if !end.After(start) {
		return 0
	}
	current := start.In(timezone.Location())
	end = end.In(timezone.Location())
	var total time.Duration
	for current.Before(end) {
		if isWeekendTime(current) {
			next := timezone.StartOfDay(current).AddDate(0, 0, 1)
			if next.After(end) {
				next = end
			}
			current = next
			continue
		}
		dayEnd := timezone.StartOfDay(current).AddDate(0, 0, 1)
		if dayEnd.After(end) {
			dayEnd = end
		}
		if dayEnd.After(current) {
			total += dayEnd.Sub(current)
		}
		current = dayEnd
	}
	return total
}

func (s *SubscriptionService) EnableUserWeekendSkip(ctx context.Context, userID, subscriptionID int64) (*UserSubscription, error) {
	if subscriptionID <= 0 {
		return nil, ErrSubscriptionNotFound
	}
	sub, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}
	if sub.UserID != userID {
		return nil, ErrSubscriptionNotFound
	}
	now := timezone.Now()
	if sub.Status != SubscriptionStatusActive || !sub.ExpiresAt.After(now) || now.Before(sub.StartsAt) {
		return nil, ErrSubscriptionNotFound
	}
	if sub.Group == nil || !sub.Group.AllowWeekendSkip || !sub.Group.IsSubscriptionType() {
		return nil, ErrWeekendSkipNotAllowed
	}
	if sub.WeekendSkipUserChangedAt != nil {
		return nil, ErrWeekendSkipAlreadyChanged
	}
	if sub.SkipWeekends {
		return nil, ErrWeekendSkipAlreadyEnabled
	}
	oldExpiresAt := sub.ExpiresAt
	sub.SkipWeekends = true
	sub.WeekendSkipUserChangedAt = &now
	if sub.WeekendSkipOriginalExpiresAt == nil {
		original := oldExpiresAt
		sub.WeekendSkipOriginalExpiresAt = &original
	}
	sub.ExpiresAt = addWeekendSkippedDuration(now, oldExpiresAt.Sub(now))
	if err := s.userSubRepo.Update(ctx, sub); err != nil {
		return nil, err
	}
	s.invalidateSubscriptionRuntimeCache(ctx, sub)
	return s.userSubRepo.GetByID(ctx, sub.ID)
}

func (s *SubscriptionService) AdminSetWeekendSkip(ctx context.Context, adminID, subscriptionID int64, enabled bool) (*UserSubscription, error) {
	if subscriptionID <= 0 {
		return nil, ErrSubscriptionNotFound
	}
	sub, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}
	now := timezone.Now()
	if sub.Status != SubscriptionStatusActive || !sub.ExpiresAt.After(now) {
		return nil, ErrSubscriptionExpired
	}
	if enabled && !sub.SkipWeekends {
		oldExpiresAt := sub.ExpiresAt
		if sub.WeekendSkipOriginalExpiresAt == nil {
			original := oldExpiresAt
			sub.WeekendSkipOriginalExpiresAt = &original
		}
		sub.ExpiresAt = addWeekendSkippedDuration(now, oldExpiresAt.Sub(now))
	} else if !enabled && sub.SkipWeekends {
		remainingUsable := weekendSkippedDurationBetween(now, sub.ExpiresAt)
		sub.ExpiresAt = now.Add(remainingUsable)
	}
	sub.SkipWeekends = enabled
	sub.WeekendSkipAdminUpdatedAt = &now
	sub.WeekendSkipAdminUpdatedBy = &adminID
	if err := s.userSubRepo.Update(ctx, sub); err != nil {
		return nil, err
	}
	s.invalidateSubscriptionRuntimeCache(ctx, sub)
	return s.userSubRepo.GetByID(ctx, sub.ID)
}

func (s *SubscriptionService) AdminPreviewWeekendSkip(ctx context.Context, subscriptionID int64, enabled bool) (*WeekendSkipPreview, error) {
	if subscriptionID <= 0 {
		return nil, ErrSubscriptionNotFound
	}
	sub, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}
	now := timezone.Now()
	if sub.Status != SubscriptionStatusActive || !sub.ExpiresAt.After(now) {
		return nil, ErrSubscriptionExpired
	}
	previewExpiresAt := sub.ExpiresAt
	reason := "unchanged"
	if enabled && !sub.SkipWeekends {
		previewExpiresAt = addWeekendSkippedDuration(now, sub.ExpiresAt.Sub(now))
		reason = "enable_compensates_weekends"
	} else if !enabled && sub.SkipWeekends {
		remainingUsable := weekendSkippedDurationBetween(now, sub.ExpiresAt)
		previewExpiresAt = now.Add(remainingUsable)
		reason = "disable_converts_to_natural_time"
	}
	return &WeekendSkipPreview{
		SubscriptionID:   sub.ID,
		Enabled:          enabled,
		CurrentExpiresAt: sub.ExpiresAt,
		PreviewExpiresAt: previewExpiresAt,
		DeltaSeconds:     int64(previewExpiresAt.Sub(sub.ExpiresAt).Seconds()),
		Reason:           reason,
	}, nil
}

func (s *SubscriptionService) AdminResetWeekendSkipUserChange(ctx context.Context, adminID, subscriptionID int64) (*UserSubscription, error) {
	if subscriptionID <= 0 {
		return nil, ErrSubscriptionNotFound
	}
	sub, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}
	now := timezone.Now()
	sub.WeekendSkipUserChangedAt = nil
	sub.WeekendSkipAdminUpdatedAt = &now
	sub.WeekendSkipAdminUpdatedBy = &adminID
	if err := s.userSubRepo.Update(ctx, sub); err != nil {
		return nil, err
	}
	s.invalidateSubscriptionRuntimeCache(ctx, sub)
	return s.userSubRepo.GetByID(ctx, sub.ID)
}

func (s *SubscriptionService) invalidateSubscriptionRuntimeCache(ctx context.Context, sub *UserSubscription) {
	if s == nil || sub == nil {
		return
	}
	if err := s.invalidateSubscriptionCaches(sub.UserID, sub.GroupID); err != nil {
		log.Printf("Warning: invalidate subscription runtime cache failed user=%d group=%d: %v", sub.UserID, sub.GroupID, err)
	}
}

// ListUserSubscriptions 获取用户的所有订阅
func (s *SubscriptionService) ListUserSubscriptions(ctx context.Context, userID int64) ([]UserSubscription, error) {
	subs, err := s.userSubRepo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	normalizeExpiredWindows(subs)
	normalizeSubscriptionStatus(subs)
	return subs, nil
}

// ListActiveUserSubscriptions 获取用户的所有有效订阅
func (s *SubscriptionService) ListActiveUserSubscriptions(ctx context.Context, userID int64) ([]UserSubscription, error) {
	subs, err := s.userSubRepo.ListActiveByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	normalizeExpiredWindows(subs)
	return subs, nil
}

// ListGroupSubscriptions 获取分组的所有订阅
func (s *SubscriptionService) ListGroupSubscriptions(ctx context.Context, groupID int64, page, pageSize int) ([]UserSubscription, *pagination.PaginationResult, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	subs, pag, err := s.userSubRepo.ListByGroupID(ctx, groupID, params)
	if err != nil {
		return nil, nil, err
	}
	normalizeExpiredWindows(subs)
	normalizeSubscriptionStatus(subs)
	return subs, pag, nil
}

// List 获取所有订阅（分页，支持筛选和排序）
func (s *SubscriptionService) List(ctx context.Context, page, pageSize int, userID, groupID *int64, status, platform, sortBy, sortOrder string) ([]UserSubscription, *pagination.PaginationResult, error) {
	params := pagination.PaginationParams{Page: page, PageSize: pageSize}
	subs, pag, err := s.userSubRepo.List(ctx, params, userID, groupID, status, platform, sortBy, sortOrder)
	if err != nil {
		return nil, nil, err
	}
	normalizeExpiredWindows(subs)
	normalizeSubscriptionStatus(subs)
	return subs, pag, nil
}

// HydrateSubscriptionPeriodUsage fills non-persistent aggregate usage used by
// admin subscription displays. It intentionally reads usage_logs instead of
// daily/weekly/monthly window counters, because those counters reset by window
// and do not represent the whole purchased subscription pool.
func (s *SubscriptionService) HydrateSubscriptionPeriodUsage(ctx context.Context, subs []UserSubscription) error {
	if s == nil || s.entClient == nil || len(subs) == 0 {
		return nil
	}
	periodPredicates := make([]predicate.UsageLog, 0, len(subs))
	byID := make(map[int64]float64, len(subs))
	for i := range subs {
		if subs[i].ID > 0 {
			byID[subs[i].ID] = 0
			periodPredicates = append(periodPredicates, usagelog.And(
				usagelog.SubscriptionIDEQ(subs[i].ID),
				usagelog.CreatedAtGTE(subs[i].StartsAt),
				usagelog.CreatedAtLT(subs[i].ExpiresAt),
			))
		}
	}
	if len(periodPredicates) == 0 {
		return nil
	}

	type row struct {
		SubscriptionID int64   `json:"subscription_id"`
		UsedUSD        float64 `json:"used_usd"`
	}
	var rows []row
	if err := s.entClient.UsageLog.Query().
		Where(
			usagelog.BillingTypeEQ(BillingTypeSubscription),
			usagelog.Or(periodPredicates...),
		).
		GroupBy(usagelog.FieldSubscriptionID).
		Aggregate(dbent.As(dbent.Sum(usagelog.FieldActualCost), "used_usd")).
		Scan(ctx, &rows); err != nil {
		return err
	}
	for _, row := range rows {
		byID[row.SubscriptionID] = row.UsedUSD
	}
	for i := range subs {
		if _, ok := byID[subs[i].ID]; ok {
			used := byID[subs[i].ID]
			subs[i].AdminTotalPoolUsedUSD = &used
		}
	}
	return nil
}

// normalizeExpiredWindows 将已过期窗口的数据清零（仅影响返回数据，不影响数据库）
// 这确保前端显示正确的当前窗口状态，而不是过期窗口的历史数据
func normalizeExpiredWindows(subs []UserSubscription) {
	for i := range subs {
		sub := &subs[i]
		overdraftMode := sub.Group != nil && sub.AllowsDailyOverdraft(sub.Group)
		// 日窗口过期：清零展示数据
		if sub.NeedsDailyReset() {
			sub.DailyWindowStart = nil
			sub.DailyUsageUSD = 0
		}
		// 周窗口过期：清零展示数据。日额度透支模式下 weekly_usage_usd
		// 被复用为“订阅有效期累计用量”，不能按自然周清零，否则后续天
		// 会重新获得已透支掉的未来日额度。
		if !overdraftMode && sub.NeedsWeeklyReset() {
			sub.WeeklyWindowStart = nil
			sub.WeeklyUsageUSD = 0
		}
		// 月窗口过期：清零展示数据。日额度透支模式下 monthly_usage_usd
		// 同样表示订阅周期内累计消耗，不能按月窗口清零。
		if !overdraftMode && sub.NeedsMonthlyReset() {
			sub.MonthlyWindowStart = nil
			sub.MonthlyUsageUSD = 0
		}
	}
}

// normalizeSubscriptionStatus 根据实际过期时间修正状态（仅影响返回数据，不影响数据库）
// 这确保前端显示正确的状态，即使定时任务尚未更新数据库
func normalizeSubscriptionStatus(subs []UserSubscription) {
	now := time.Now()
	for i := range subs {
		sub := &subs[i]
		if (sub.Status == SubscriptionStatusActive || sub.Status == SubscriptionStatusQuotaExhausted) && !sub.ExpiresAt.After(now) {
			sub.Status = SubscriptionStatusExpired
		}
	}
}

// startOfDay 返回给定时间所在时区的当天 00:00:00。
func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// CheckAndActivateWindow 检查并激活窗口（首次使用时）
func (s *SubscriptionService) CheckAndActivateWindow(ctx context.Context, sub *UserSubscription) error {
	if sub.IsWindowActivated() {
		return nil
	}

	now := time.Now()
	dailyStart := sub.CurrentDailyWindowStart(now)
	weeklyStart := sub.CurrentWeeklyWindowStart(now)
	monthlyStart := sub.CurrentMonthlyWindowStart(now)
	return s.userSubRepo.ActivateWindows(ctx, sub.ID, dailyStart, weeklyStart, monthlyStart)
}

// AdminResetQuota manually resets the daily, weekly, and/or monthly usage windows.
// Uses the current rolling window anchored at subscription.StartsAt.
//
// 业务规则（与前端 UI 对齐）：
//  1. 付费订阅（source = payment）永不可重置，返回 ErrPaidSubscriptionImmutable。
//  2. 订阅 group 必须至少配置一档限额，否则没有窗口需要重置（ErrNoLimitsConfigured）。
//  3. 不允许重置「上限窗口」——即 group 配置中的最长窗口（monthly > weekly > daily）。
//     因为重置上限会绕过订阅的真正约束。
//  4. 不允许重置 group 中未配置 limit 的档位（无意义）。
//  5. 余下「短于上限」且「已配置」的档位允许被勾选重置。
func (s *SubscriptionService) AdminResetQuota(ctx context.Context, subscriptionID int64, resetDaily, resetWeekly, resetMonthly bool) (*UserSubscription, error) {
	if !resetDaily && !resetWeekly && !resetMonthly {
		return nil, ErrInvalidInput
	}
	sub, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}
	// 规则 1：付费订阅完全锁定
	if sub.Source == domain.SubscriptionSourcePayment {
		return nil, ErrPaidSubscriptionImmutable
	}
	// 规则 2-4：根据 group 限额配置校验请求的档位是否合法
	if sub.Group == nil {
		// 防御：缺关联时拒绝执行
		return nil, ErrNoLimitsConfigured
	}
	if err := validateResetTargets(sub.Group, resetDaily, resetWeekly, resetMonthly); err != nil {
		return nil, err
	}

	now := time.Now()
	if resetDaily {
		windowStart := sub.CurrentDailyWindowStart(now)
		if err := s.userSubRepo.ResetDailyUsage(ctx, sub.ID, sub.DailyWindowStart, windowStart); err != nil {
			return nil, err
		}
	}
	if resetWeekly {
		if sub.AllowsDailyOverdraft(sub.Group) {
			return nil, ErrInvalidResetTarget
		}
		windowStart := sub.CurrentWeeklyWindowStart(now)
		if err := s.userSubRepo.ResetWeeklyUsage(ctx, sub.ID, sub.WeeklyWindowStart, windowStart); err != nil {
			return nil, err
		}
	}
	if resetMonthly {
		if sub.AllowsDailyOverdraft(sub.Group) {
			return nil, ErrInvalidResetTarget
		}
		windowStart := sub.CurrentMonthlyWindowStart(now)
		if err := s.userSubRepo.ResetMonthlyUsage(ctx, sub.ID, sub.MonthlyWindowStart, windowStart); err != nil {
			return nil, err
		}
	}
	if err := s.invalidateSubscriptionCaches(sub.UserID, sub.GroupID); err != nil {
		return nil, err
	}
	// Return the refreshed subscription from DB
	return s.userSubRepo.GetByID(ctx, subscriptionID)
}

// GetDailyLimitResetPaymentTarget validates a user's subscription and returns
// the price that must be used to create a daily-limit reset payment order.
func (s *SubscriptionService) GetDailyLimitResetPaymentTarget(ctx context.Context, userID, subscriptionID int64) (*DailyLimitResetPaymentTarget, error) {
	if subscriptionID <= 0 {
		return nil, ErrSubscriptionNotFound
	}
	sub, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}
	if sub.UserID != userID {
		return nil, ErrSubscriptionNotFound
	}
	if err := s.validateDailyLimitResetTarget(sub); err != nil {
		return nil, err
	}
	return &DailyLimitResetPaymentTarget{
		Subscription: sub,
		Group:        sub.Group,
		Price:        *sub.Group.DailyLimitResetPrice,
	}, nil
}

// PaidResetDailyQuota resets only the daily usage amount after a successful
// daily-limit reset payment. It intentionally does not reuse AdminResetQuota,
// because the admin entry has different business rules for paid subscriptions.
func (s *SubscriptionService) PaidResetDailyQuota(ctx context.Context, userID, subscriptionID int64) (*UserSubscription, error) {
	target, err := s.GetDailyLimitResetPaymentTarget(ctx, userID, subscriptionID)
	if err != nil {
		return nil, err
	}
	sub := target.Subscription
	now := time.Now()
	windowStart := sub.CurrentDailyWindowStart(now)
	if err := s.userSubRepo.ResetDailyUsage(ctx, sub.ID, sub.DailyWindowStart, windowStart); err != nil {
		return nil, err
	}
	s.invalidateSubscriptionRuntimeCache(ctx, sub)
	return s.userSubRepo.GetByID(ctx, sub.ID)
}

// FulfillPaidDailyQuotaReset applies a paid daily reset using the order-frozen
// subscription target. Unlike PaidResetDailyQuota, it does not re-check current
// group pricing / reset switch to avoid "paid but fulfillment failed" after the
// admin changes group config during the payment window.
func (s *SubscriptionService) FulfillPaidDailyQuotaReset(ctx context.Context, userID, subscriptionID int64) (*UserSubscription, error) {
	if subscriptionID <= 0 {
		return nil, ErrSubscriptionNotFound
	}
	sub, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}
	if sub.UserID != userID {
		return nil, ErrSubscriptionNotFound
	}
	if sub.Status != SubscriptionStatusActive || !sub.ExpiresAt.After(time.Now()) {
		return nil, ErrSubscriptionNotFound
	}
	now := time.Now()
	windowStart := sub.CurrentDailyWindowStart(now)
	if err := s.userSubRepo.ResetDailyUsage(ctx, sub.ID, sub.DailyWindowStart, windowStart); err != nil {
		return nil, err
	}
	s.invalidateSubscriptionRuntimeCache(ctx, sub)
	return s.userSubRepo.GetByID(ctx, sub.ID)
}

func (s *SubscriptionService) validateDailyLimitResetTarget(sub *UserSubscription) error {
	if sub == nil || sub.Group == nil {
		return ErrDailyLimitResetNotAvailable
	}
	if sub.Status != SubscriptionStatusActive || !sub.ExpiresAt.After(time.Now()) {
		return ErrSubscriptionNotFound
	}
	if !sub.Group.IsSubscriptionType() || !sub.Group.HasDailyLimit() {
		return ErrDailyLimitResetNotAvailable
	}
	if sub.Group.DailyLimitResetPrice == nil || *sub.Group.DailyLimitResetPrice <= 0 {
		return ErrDailyLimitResetNotAvailable
	}
	return nil
}

// resolveSubscriptionSource 把入参 Source 标准化为最终落库值。
// 显式传入合法 source 优先；否则按 AssignedBy 推断（>0=admin，==0=redeem）。
// payment 入口必须显式传入，不会被自动推断出来。
func resolveSubscriptionSource(explicit string, assignedBy int64) string {
	switch explicit {
	case domain.SubscriptionSourceAdmin,
		domain.SubscriptionSourceRedeem,
		domain.SubscriptionSourcePayment:
		return explicit
	}
	if assignedBy > 0 {
		return domain.SubscriptionSourceAdmin
	}
	return domain.SubscriptionSourceRedeem
}

func resolveSubscriptionValidityUnit(inputUnit, existingUnit string) string {
	if normalized := normalizeSubscriptionValidityUnit(inputUnit); normalized != "day" || inputUnit != "" {
		return normalized
	}
	if existingUnit != "" {
		return normalizeSubscriptionValidityUnit(existingUnit)
	}
	return "day"
}

// validateResetTargets 校验请求的重置档位组合是否合法。
// 规则：
//   - 至少配置了一档限额（否则没什么可重置的）
//   - 不允许重置上限档（最长窗口）
//   - 不允许重置 group 中未配置的档（无意义）
func validateResetTargets(group *Group, resetDaily, resetWeekly, resetMonthly bool) error {
	hasDaily := group.HasDailyLimit()
	hasWeekly := group.HasWeeklyLimit()
	hasMonthly := group.HasMonthlyLimit()
	if !hasDaily && !hasWeekly && !hasMonthly {
		return ErrNoLimitsConfigured
	}
	// 上限档 = 已配置的最长窗口
	upperBound := ""
	switch {
	case hasMonthly:
		upperBound = "monthly"
	case hasWeekly:
		upperBound = "weekly"
	case hasDaily:
		upperBound = "daily"
	}
	if resetDaily && (!hasDaily || upperBound == "daily") {
		return ErrInvalidResetTarget
	}
	if resetWeekly && (!hasWeekly || upperBound == "weekly") {
		return ErrInvalidResetTarget
	}
	if resetMonthly && (!hasMonthly || upperBound == "monthly") {
		return ErrInvalidResetTarget
	}
	return nil
}

// CheckAndResetWindows 检查并重置过期的窗口
func (s *SubscriptionService) CheckAndResetWindows(ctx context.Context, sub *UserSubscription) error {
	now := time.Now()
	needsInvalidateCache := false

	// 日窗口重置（订阅锚点滚动 24 小时）
	if sub.NeedsDailyReset() {
		windowStart := sub.CurrentDailyWindowStart(now)
		if err := s.userSubRepo.ResetDailyUsage(ctx, sub.ID, sub.DailyWindowStart, windowStart); err != nil {
			return err
		}
		sub.DailyWindowStart = &windowStart
		sub.DailyUsageUSD = 0
		needsInvalidateCache = true
	}

	// 周窗口重置（订阅锚点滚动 7 天）。日额度透支模式下 weekly_usage_usd
	// 承载订阅周期累计用量，必须一直保留到订阅过期/续期，不能每周重置。
	if sub.NeedsWeeklyReset() && (sub.Group == nil || !sub.AllowsDailyOverdraft(sub.Group)) {
		windowStart := sub.CurrentWeeklyWindowStart(now)
		if err := s.userSubRepo.ResetWeeklyUsage(ctx, sub.ID, sub.WeeklyWindowStart, windowStart); err != nil {
			return err
		}
		sub.WeeklyWindowStart = &windowStart
		sub.WeeklyUsageUSD = 0
		needsInvalidateCache = true
	}

	// 月窗口重置（订阅锚点滚动 30 天）。日额度透支模式下 monthly_usage_usd
	// 表示订阅周期累计用量，不能按月窗口清零。
	if sub.NeedsMonthlyReset() && (sub.Group == nil || !sub.AllowsDailyOverdraft(sub.Group)) {
		windowStart := sub.CurrentMonthlyWindowStart(now)
		if err := s.userSubRepo.ResetMonthlyUsage(ctx, sub.ID, sub.MonthlyWindowStart, windowStart); err != nil {
			return err
		}
		sub.MonthlyWindowStart = &windowStart
		sub.MonthlyUsageUSD = 0
		needsInvalidateCache = true
	}

	// 如果有窗口被重置，失效缓存以保持一致性
	if needsInvalidateCache {
		s.invalidateSubscriptionRuntimeCache(ctx, sub)
	}

	return nil
}

// MarkQuotaExhausted immediately marks an active, not-yet-expired subscription as quota_exhausted.
// It is only used for period-level pools (weekly/monthly), never for daily-only limits,
// because daily quota can reset without renewing the subscription.
func (s *SubscriptionService) MarkQuotaExhausted(ctx context.Context, sub *UserSubscription) {
	if s == nil || sub == nil || sub.ID <= 0 || sub.Status == SubscriptionStatusQuotaExhausted {
		return
	}
	now := time.Now()
	if sub.Status != SubscriptionStatusActive || !sub.ExpiresAt.After(now) || now.Before(sub.StartsAt) {
		return
	}
	if err := s.userSubRepo.UpdateStatus(ctx, sub.ID, SubscriptionStatusQuotaExhausted); err != nil {
		return
	}
	s.afterSubscriptionQuotaExhausted(ctx, sub)
}

func (s *SubscriptionService) afterSubscriptionQuotaExhausted(ctx context.Context, sub *UserSubscription) {
	if s == nil || sub == nil {
		return
	}
	s.invalidateSubscriptionRuntimeCache(ctx, sub)
	sub.Status = SubscriptionStatusQuotaExhausted
}

// CheckUsageLimits 检查使用限额（返回错误如果超限）
// 用于中间件的快速预检查，additionalCost 通常为 0
func (s *SubscriptionService) CheckUsageLimits(ctx context.Context, sub *UserSubscription, group *Group, additionalCost float64) error {
	if !sub.CheckDailyLimit(group, additionalCost) {
		return ErrDailyLimitExceeded
	}
	if !sub.CheckWeeklyLimit(group, additionalCost) {
		s.MarkQuotaExhausted(ctx, sub)
		return ErrWeeklyLimitExceeded
	}
	if !sub.CheckMonthlyLimit(group, additionalCost) {
		s.MarkQuotaExhausted(ctx, sub)
		return ErrMonthlyLimitExceeded
	}
	return nil
}

// ValidateAndCheckLimits 合并验证+限额检查（中间件热路径专用）
// 仅做内存检查，不触发 DB 写入。窗口重置的 DB 写入由 DoWindowMaintenance 异步完成。
// 返回 needsMaintenance 表示是否需要异步执行窗口维护。
func (s *SubscriptionService) ValidateAndCheckLimits(ctx context.Context, sub *UserSubscription, group *Group) (needsMaintenance bool, err error) {
	return s.validateAndCheckLimits(ctx, sub, group, true)
}

func (s *SubscriptionService) validateAndCheckLimits(ctx context.Context, sub *UserSubscription, group *Group, allowDBRecheck bool) (needsMaintenance bool, err error) {
	now := time.Now()

	// 1. 验证订阅状态
	if sub.Status == SubscriptionStatusExpired {
		return false, ErrSubscriptionExpired
	}
	if sub.Status == SubscriptionStatusSuspended {
		return false, ErrSubscriptionSuspended
	}
	if sub.Status == SubscriptionStatusQuotaExhausted {
		return false, ErrSubscriptionQuotaExhausted
	}
	if now.Before(sub.StartsAt) {
		return false, ErrSubscriptionNotStarted
	}
	if sub.IsExpired() {
		return false, ErrSubscriptionExpired
	}
	if sub.SkipWeekends && isWeekendTime(now) {
		return false, ErrSubscriptionWeekendDisabled
	}

	// 2. 内存中修正过期窗口的用量，确保 CheckUsageLimits 不会误拒绝用户
	//    实际的 DB 窗口重置由 DoWindowMaintenance 异步完成
	if sub.NeedsDailyReset() {
		sub.DailyUsageUSD = 0
		needsMaintenance = true
	}
	if sub.NeedsWeeklyReset() && (group == nil || !sub.AllowsDailyOverdraft(group)) {
		sub.WeeklyUsageUSD = 0
		needsMaintenance = true
	}
	if sub.NeedsMonthlyReset() && (group == nil || !sub.AllowsDailyOverdraft(group)) {
		sub.MonthlyUsageUSD = 0
		needsMaintenance = true
	}
	if !sub.IsWindowActivated() {
		needsMaintenance = true
	}

	// 3. 检查用量限额
	if !sub.CheckTotalQuota(0) {
		if allowDBRecheck {
			return s.recheckSubscriptionLimitFromDB(ctx, sub, group, needsMaintenance, ErrSubscriptionQuotaExhausted)
		}
		s.MarkQuotaExhausted(ctx, sub)
		return needsMaintenance, ErrSubscriptionQuotaExhausted
	}
	if !sub.CheckDailyLimit(group, 0) {
		if allowDBRecheck {
			return s.recheckSubscriptionLimitFromDB(ctx, sub, group, needsMaintenance, ErrDailyLimitExceeded)
		}
		if sub.AllowsDailyOverdraft(group) {
			s.MarkQuotaExhausted(ctx, sub)
		}
		return needsMaintenance, ErrDailyLimitExceeded
	}
	if !sub.CheckWeeklyLimit(group, 0) {
		if allowDBRecheck {
			return s.recheckSubscriptionLimitFromDB(ctx, sub, group, needsMaintenance, ErrWeeklyLimitExceeded)
		}
		s.MarkQuotaExhausted(ctx, sub)
		return needsMaintenance, ErrWeeklyLimitExceeded
	}
	if !sub.CheckMonthlyLimit(group, 0) {
		if allowDBRecheck {
			return s.recheckSubscriptionLimitFromDB(ctx, sub, group, needsMaintenance, ErrMonthlyLimitExceeded)
		}
		s.MarkQuotaExhausted(ctx, sub)
		return needsMaintenance, ErrMonthlyLimitExceeded
	}

	return needsMaintenance, nil
}

func (s *SubscriptionService) recheckSubscriptionLimitFromDB(ctx context.Context, sub *UserSubscription, group *Group, staleNeedsMaintenance bool, staleErr error) (bool, error) {
	if s == nil || s.userSubRepo == nil || sub == nil || sub.UserID <= 0 || sub.GroupID <= 0 {
		return staleNeedsMaintenance, staleErr
	}

	freshSub, err := s.userSubRepo.GetActiveByUserIDAndGroupID(ctx, sub.UserID, sub.GroupID)
	if err != nil || freshSub == nil {
		return staleNeedsMaintenance, staleErr
	}

	freshNeedsMaintenance, freshErr := s.validateAndCheckLimits(ctx, freshSub, group, false)
	if freshErr != nil {
		return freshNeedsMaintenance, freshErr
	}

	// 当前快照判定超限，但 DB 权威数据仍可用：说明 L1/热路径快照已陈旧。
	// 覆盖调用方持有的订阅对象，并清掉相关缓存，避免后续请求继续命中旧快照。
	*sub = *freshSub
	s.invalidateSubscriptionRuntimeCache(ctx, sub)
	return freshNeedsMaintenance, nil
}

// DoWindowMaintenance 异步执行窗口维护（激活+重置）
// 使用独立 context，不受请求取消影响。
// 注意：此方法仅在 ValidateAndCheckLimits 返回 needsMaintenance=true 时调用，
// 而 IsExpired()=true 的订阅在 ValidateAndCheckLimits 中已被拦截返回错误，
// 因此进入此方法的订阅一定未过期，无需处理过期状态同步。
func (s *SubscriptionService) DoWindowMaintenance(sub *UserSubscription) {
	if s == nil {
		return
	}
	if s.maintenanceQueue != nil {
		err := s.maintenanceQueue.TryEnqueue(func() {
			s.doWindowMaintenance(sub)
		})
		if err != nil {
			log.Printf("Subscription maintenance enqueue failed: %v", err)
		}
		return
	}

	s.doWindowMaintenance(sub)
}

func (s *SubscriptionService) doWindowMaintenance(sub *UserSubscription) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 激活窗口（首次使用时）
	if !sub.IsWindowActivated() {
		if err := s.CheckAndActivateWindow(ctx, sub); err != nil {
			log.Printf("Failed to activate subscription windows: %v", err)
		}
	}

	// 重置过期窗口
	if err := s.CheckAndResetWindows(ctx, sub); err != nil {
		log.Printf("Failed to reset subscription windows: %v", err)
	}

	// 失效缓存，确保后续请求拿到更新后的数据
	s.invalidateSubscriptionRuntimeCache(ctx, sub)
}

// RecordUsage 记录使用量到订阅
func (s *SubscriptionService) RecordUsage(ctx context.Context, subscriptionID int64, costUSD float64) error {
	return s.userSubRepo.IncrementUsage(ctx, subscriptionID, costUSD)
}

// SubscriptionProgress 订阅进度
type SubscriptionProgress struct {
	ID            int64                `json:"id"`
	GroupName     string               `json:"group_name"`
	ExpiresAt     time.Time            `json:"expires_at"`
	ExpiresInDays int                  `json:"expires_in_days"`
	Total         *UsageWindowProgress `json:"total,omitempty"`
	Daily         *UsageWindowProgress `json:"daily,omitempty"`
	Weekly        *UsageWindowProgress `json:"weekly,omitempty"`
	Monthly       *UsageWindowProgress `json:"monthly,omitempty"`
}

// UsageWindowProgress 使用窗口进度
type UsageWindowProgress struct {
	LimitUSD        float64   `json:"limit_usd"`
	UsedUSD         float64   `json:"used_usd"`
	RemainingUSD    float64   `json:"remaining_usd"`
	Percentage      float64   `json:"percentage"`
	WindowStart     time.Time `json:"window_start"`
	ResetsAt        time.Time `json:"resets_at"`
	ResetsInSeconds int64     `json:"resets_in_seconds"`
}

// GetSubscriptionProgress 获取订阅使用进度
func (s *SubscriptionService) GetSubscriptionProgress(ctx context.Context, subscriptionID int64) (*SubscriptionProgress, error) {
	sub, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, ErrSubscriptionNotFound
	}

	group := sub.Group
	if group == nil {
		group, err = s.groupRepo.GetByID(ctx, sub.GroupID)
		if err != nil {
			return nil, err
		}
	}

	return s.calculateProgress(sub, group), nil
}

// calculateProgress 根据已加载的订阅和分组数据计算使用进度（纯内存计算，无 DB 查询）
func (s *SubscriptionService) calculateProgress(sub *UserSubscription, group *Group) *SubscriptionProgress {
	now := time.Now()
	progress := &SubscriptionProgress{
		ID:            sub.ID,
		GroupName:     group.Name,
		ExpiresAt:     sub.ExpiresAt,
		ExpiresInDays: sub.DaysRemaining(),
	}

	if sub.HasTotalQuotaLimit() {
		remaining := *sub.QuotaLimitUSD - sub.QuotaUsedUSD
		if remaining < 0 {
			remaining = 0
		}
		percentage := (sub.QuotaUsedUSD / *sub.QuotaLimitUSD) * 100
		if percentage > 100 {
			percentage = 100
		}
		resetsIn := int64(sub.ExpiresAt.Sub(now).Seconds())
		if resetsIn < 0 {
			resetsIn = 0
		}
		progress.Total = &UsageWindowProgress{
			LimitUSD:        *sub.QuotaLimitUSD,
			UsedUSD:         sub.QuotaUsedUSD,
			RemainingUSD:    remaining,
			Percentage:      percentage,
			WindowStart:     sub.StartsAt,
			ResetsAt:        sub.ExpiresAt,
			ResetsInSeconds: resetsIn,
		}
	}

	// 日进度
	if group.HasDailyLimit() && sub.DailyWindowStart != nil {
		limit := *group.DailyLimitUSD
		resetsAt := sub.DailyResetTime()
		if resetsAt == nil {
			goto weeklyProgress
		}
		progress.Daily = &UsageWindowProgress{
			LimitUSD:        limit,
			UsedUSD:         sub.DailyUsageUSD,
			RemainingUSD:    limit - sub.DailyUsageUSD,
			Percentage:      (sub.DailyUsageUSD / limit) * 100,
			WindowStart:     *sub.DailyWindowStart,
			ResetsAt:        *resetsAt,
			ResetsInSeconds: int64(time.Until(*resetsAt).Seconds()),
		}
		if progress.Daily.RemainingUSD < 0 {
			progress.Daily.RemainingUSD = 0
		}
		if progress.Daily.Percentage > 100 {
			progress.Daily.Percentage = 100
		}
		if progress.Daily.ResetsInSeconds < 0 {
			progress.Daily.ResetsInSeconds = 0
		}
	}

weeklyProgress:
	// 周进度；日额度透支模式下复用此进度展示订阅有效期总额度。
	if (group.HasWeeklyLimit() || sub.AllowsDailyOverdraft(group)) && sub.WeeklyWindowStart != nil {
		var limit float64
		var used float64
		windowStart := *sub.WeeklyWindowStart
		var resetsAt *time.Time
		if sub.AllowsDailyOverdraft(group) {
			if overdraftLimit, ok := sub.DailyOverdraftLimitUSD(group); ok {
				limit = overdraftLimit
			} else {
				goto monthlyProgress
			}
			used = sub.DailyOverdraftUsedUSDAt(group, now)
			windowStart = sub.StartsAt
			resetsAt = &sub.ExpiresAt
		} else {
			limit = *group.WeeklyLimitUSD
			used = sub.WeeklyUsageUSD
			resetsAt = sub.WeeklyResetTime()
		}
		if resetsAt == nil {
			goto monthlyProgress
		}
		progress.Weekly = &UsageWindowProgress{
			LimitUSD:        limit,
			UsedUSD:         used,
			RemainingUSD:    limit - used,
			Percentage:      (used / limit) * 100,
			WindowStart:     windowStart,
			ResetsAt:        *resetsAt,
			ResetsInSeconds: int64(resetsAt.Sub(now).Seconds()),
		}
		if progress.Weekly.RemainingUSD < 0 {
			progress.Weekly.RemainingUSD = 0
		}
		if progress.Weekly.Percentage > 100 {
			progress.Weekly.Percentage = 100
		}
		if progress.Weekly.ResetsInSeconds < 0 {
			progress.Weekly.ResetsInSeconds = 0
		}
	}

monthlyProgress:
	// 月进度
	if group.HasMonthlyLimit() && sub.MonthlyWindowStart != nil {
		limit := *group.MonthlyLimitUSD
		resetsAt := sub.MonthlyResetTime()
		if resetsAt == nil {
			return progress
		}
		progress.Monthly = &UsageWindowProgress{
			LimitUSD:        limit,
			UsedUSD:         sub.MonthlyUsageUSD,
			RemainingUSD:    limit - sub.MonthlyUsageUSD,
			Percentage:      (sub.MonthlyUsageUSD / limit) * 100,
			WindowStart:     *sub.MonthlyWindowStart,
			ResetsAt:        *resetsAt,
			ResetsInSeconds: int64(time.Until(*resetsAt).Seconds()),
		}
		if progress.Monthly.RemainingUSD < 0 {
			progress.Monthly.RemainingUSD = 0
		}
		if progress.Monthly.Percentage > 100 {
			progress.Monthly.Percentage = 100
		}
		if progress.Monthly.ResetsInSeconds < 0 {
			progress.Monthly.ResetsInSeconds = 0
		}
	}

	return progress
}

// GetUserSubscriptionsWithProgress 获取用户所有订阅及进度
func (s *SubscriptionService) GetUserSubscriptionsWithProgress(ctx context.Context, userID int64) ([]SubscriptionProgress, error) {
	// ListActiveByUserID 已使用 .WithGroup() eager-load Group 关联，1 次查询获取所有数据
	subs, err := s.userSubRepo.ListActiveByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	progresses := make([]SubscriptionProgress, 0, len(subs))
	for i := range subs {
		sub := &subs[i]
		group := sub.Group
		if group == nil {
			continue
		}
		progresses = append(progresses, *s.calculateProgress(sub, group))
	}

	return progresses, nil
}

// ValidateSubscription 验证订阅是否有效
func (s *SubscriptionService) ValidateSubscription(ctx context.Context, sub *UserSubscription) error {
	if time.Now().Before(sub.StartsAt) {
		return ErrSubscriptionNotStarted
	}
	if sub.Status == SubscriptionStatusExpired {
		return ErrSubscriptionExpired
	}
	if sub.Status == SubscriptionStatusSuspended {
		return ErrSubscriptionSuspended
	}
	if sub.Status == SubscriptionStatusQuotaExhausted {
		return ErrSubscriptionQuotaExhausted
	}
	if sub.IsExpired() {
		// 更新状态
		if err := s.userSubRepo.UpdateStatus(ctx, sub.ID, SubscriptionStatusExpired); err == nil {
			s.invalidateSubscriptionRuntimeCache(ctx, sub)
		}
		return ErrSubscriptionExpired
	}
	return nil
}
