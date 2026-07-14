package service

import (
	"math"
	"strings"
	"time"
)

const (
	dailyWindowDuration   = 24 * time.Hour
	weeklyWindowDuration  = 7 * 24 * time.Hour
	monthlyWindowDuration = 30 * 24 * time.Hour
)

type UserSubscription struct {
	ID      int64
	UserID  int64
	GroupID int64

	StartsAt  time.Time
	ExpiresAt time.Time
	Status    string
	// ValidityUnit records the original subscription card unit.
	// Only day-based cards use elapsed daily quota as effective overdraft usage;
	// week/month cards keep normal actual-usage accounting.
	ValidityUnit string

	DailyWindowStart   *time.Time
	WeeklyWindowStart  *time.Time
	MonthlyWindowStart *time.Time

	DailyUsageUSD       float64
	WeeklyUsageUSD      float64
	MonthlyUsageUSD     float64
	QuotaLimitUSD       *float64
	QuotaUsedUSD        float64
	AllowDailyOverdraft bool
	SkipWeekends        bool

	WeekendSkipUserChangedAt     *time.Time
	WeekendSkipOriginalExpiresAt *time.Time
	WeekendSkipAdminUpdatedAt    *time.Time
	WeekendSkipAdminUpdatedBy    *int64

	AssignedBy *int64
	AssignedAt time.Time
	Notes      string

	// Source 标识订阅来源（admin/redeem/payment），决定 AdminResetQuota 是否允许重置。
	Source string

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time

	// LastUsedAt 是非持久化字段，由 SubscriptionService.List/GetByID 等读路径
	// 通过聚合 usage_logs 后批量填充，与 user.LastUsedAt 同源同范式。
	// 写路径不应使用该字段。
	LastUsedAt *time.Time

	User           *User
	Group          *Group
	AssignedByUser *User
}

func (s *UserSubscription) IsActive() bool {
	now := time.Now()
	return s.Status == SubscriptionStatusActive && !now.Before(s.StartsAt) && now.Before(s.ExpiresAt)
}

func (s *UserSubscription) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

func (s *UserSubscription) DaysRemaining() int {
	if s.IsExpired() {
		return 0
	}
	return int(time.Until(s.ExpiresAt).Hours() / 24)
}

// EffectiveValidityDays returns the subscription's calendar validity length in
// days, rounded up so partial-day subscriptions still own at least one daily
// quota card. The value is derived from the persisted start/end anchors because
// historical subscriptions do not store the original plan unit.
//
// 注意：日透支额度池不要直接用本方法。跳过周末会拉长 ExpiresAt，若按日历跨度
// 计算会把周末不可用天数也算进透支总额。透支请用 OverdraftValidityDays。
func (s *UserSubscription) EffectiveValidityDays() int {
	if s == nil || !s.ExpiresAt.After(s.StartsAt) {
		return 0
	}
	return validityDaysBetween(s.StartsAt, s.ExpiresAt)
}

// OverdraftValidityDays 返回日透支总额度对应的“计划天数”。
//
// 规则：
//  1. 默认与日历有效期一致；
//  2. 若开启跳过周末，则按 starts~expires 之间的工作日可用时长折算天数，
//     避免把周末不可用天数算进透支池；
//  3. 若记录了原自然到期时间，则同时用它校准计划天数。历史续费数据可能
//     遗留过旧 original，因此取 original 与工作日反推值中更大的有效值。
func (s *UserSubscription) OverdraftValidityDays() int {
	if s == nil || !s.ExpiresAt.After(s.StartsAt) {
		return 0
	}
	days := validityDaysBetween(s.StartsAt, s.ExpiresAt)
	if s.SkipWeekends {
		plannedDays := 0
		usable := weekendSkippedDurationBetween(s.StartsAt, s.ExpiresAt)
		if usable > 0 {
			plannedDays = int(math.Ceil(usable.Hours() / 24))
			if plannedDays < 1 {
				plannedDays = 1
			}
		}
		if s.WeekendSkipOriginalExpiresAt != nil && s.WeekendSkipOriginalExpiresAt.After(s.StartsAt) {
			originalDays := validityDaysBetween(s.StartsAt, *s.WeekendSkipOriginalExpiresAt)
			if originalDays > plannedDays {
				plannedDays = originalDays
			}
		}
		if plannedDays > 0 {
			days = plannedDays
		}
	}
	if days < 1 {
		return 1
	}
	return days
}

func validityDaysBetween(startsAt, expiresAt time.Time) int {
	if !expiresAt.After(startsAt) {
		return 0
	}
	days := int(math.Ceil(expiresAt.Sub(startsAt).Hours() / 24))
	if days < 1 {
		return 1
	}
	return days
}

func normalizeSubscriptionValidityUnit(unit string) string {
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

func (s *UserSubscription) IsDayValidityUnit() bool {
	if s == nil {
		return true
	}
	return normalizeSubscriptionValidityUnit(s.ValidityUnit) == "day"
}

func (s *UserSubscription) DailyOverdraftLimitUSD(group *Group) (float64, bool) {
	if s == nil || group == nil || !group.HasDailyLimit() {
		return 0, false
	}
	days := s.OverdraftValidityDays()
	if days <= 0 {
		return 0, false
	}
	return *group.DailyLimitUSD * float64(days), true
}

// DailyOverdraftConsumedUSD returns the consumed amount inside the subscription
// pool. Day-based daily-overdraft cards count fully elapsed daily quota as used;
// week/month cards keep normal actual cumulative spend accounting.
func (s *UserSubscription) DailyOverdraftConsumedUSD(group *Group, now time.Time) float64 {
	if s == nil || group == nil || !s.AllowsDailyOverdraft(group) || !group.HasDailyLimit() {
		return 0
	}
	return s.DailyOverdraftUsedUSDAt(group, now)
}

// DailyOverdraftUsedUSD returns the effective consumed amount as of now.
func (s *UserSubscription) DailyOverdraftUsedUSD(group *Group) float64 {
	return s.DailyOverdraftUsedUSDAt(group, time.Now())
}

// DailyOverdraftUsedUSDAt returns the effective consumed amount at a specific
// time. For day-based cards it intentionally counts fully elapsed daily cards
// as consumed even when the user did not spend them, because each day grants
// one daily card and unused cards expire after that day. Week/month cards do
// not use this elapsed-day accounting and return actual cumulative spend.
func (s *UserSubscription) DailyOverdraftUsedUSDAt(group *Group, now time.Time) float64 {
	if s == nil {
		return 0
	}
	actualUsed := s.WeeklyUsageUSD
	if group == nil || !s.AllowsDailyOverdraft(group) || !group.HasDailyLimit() || !s.IsDayValidityUnit() {
		return actualUsed
	}
	elapsedFullDays := s.dailyOverdraftElapsedFullDays(now)
	if maxDays := s.OverdraftValidityDays(); elapsedFullDays > maxDays {
		elapsedFullDays = maxDays
	}
	if elapsedFullDays < 0 {
		elapsedFullDays = 0
	}
	expiredQuota := *group.DailyLimitUSD * float64(elapsedFullDays)
	effectiveUsed := expiredQuota + s.currentDailyWindowUsage(now)
	if actualUsed > effectiveUsed {
		return actualUsed
	}
	return effectiveUsed
}

// DailyOverdraftDebtUSD returns historical borrowed quota that has not been
// paid back by elapsed daily cards yet. It is mainly used after a user turns
// daily-overdraft off: the subscription returns to normal daily-limit mode, but
// the next daily cards are reduced by this debt instead of forgiving the prior
// overdraft.
func (s *UserSubscription) DailyOverdraftDebtUSD(group *Group, now time.Time) float64 {
	if s == nil || group == nil || !group.AllowsDailyOverdraft() || !group.HasDailyLimit() || !s.IsDayValidityUnit() {
		return 0
	}
	elapsedFullDays := s.dailyOverdraftElapsedFullDays(now)
	if maxDays := s.OverdraftValidityDays(); elapsedFullDays > maxDays {
		elapsedFullDays = maxDays
	}
	if elapsedFullDays < 0 {
		elapsedFullDays = 0
	}
	repaidQuota := *group.DailyLimitUSD * float64(elapsedFullDays)
	priorActualUsage := s.WeeklyUsageUSD - s.currentDailyWindowUsage(now)
	if priorActualUsage < 0 {
		priorActualUsage = 0
	}
	debt := priorActualUsage - repaidQuota
	if debt < 0 {
		return 0
	}
	return debt
}

func (s *UserSubscription) dailyOverdraftElapsedFullDays(now time.Time) int {
	if s == nil || s.StartsAt.IsZero() || !s.ExpiresAt.After(s.StartsAt) || !now.After(s.StartsAt) {
		return 0
	}
	// 跳过周末时，不可用的周末不应消耗“已过日额度卡”。
	if s.SkipWeekends {
		usable := weekendSkippedDurationBetween(s.StartsAt, now)
		if usable <= 0 {
			return 0
		}
		return int(usable / dailyWindowDuration)
	}
	return int(now.Sub(s.StartsAt) / dailyWindowDuration)
}

func (s *UserSubscription) currentDailyWindowUsage(now time.Time) float64 {
	if s == nil || s.DailyWindowStart == nil || s.StartsAt.IsZero() {
		return 0
	}
	currentStart := s.CurrentDailyWindowStart(now)
	if !s.DailyWindowStart.Equal(currentStart) {
		return 0
	}
	if s.DailyUsageUSD < 0 {
		return 0
	}
	return s.DailyUsageUSD
}

func (s *UserSubscription) dailyOverdraftNormalDays(now time.Time) int {
	if s == nil || s.StartsAt.IsZero() || !s.ExpiresAt.After(s.StartsAt) || now.Before(s.StartsAt) {
		return 0
	}
	var days int
	if s.SkipWeekends {
		usable := weekendSkippedDurationBetween(s.StartsAt, now)
		if usable < 0 {
			usable = 0
		}
		days = int(usable/dailyWindowDuration) + 1
	} else {
		days = int(now.Sub(s.StartsAt)/dailyWindowDuration) + 1
	}
	maxDays := s.OverdraftValidityDays()
	if maxDays <= 0 {
		return 0
	}
	if days > maxDays {
		days = maxDays
	}
	return days
}

// DailyOverdraftBorrowedDays returns how many future daily cards have already
// been borrowed relative to the days that have normally arrived so far.
func (s *UserSubscription) DailyOverdraftBorrowedDays(group *Group, now time.Time) int {
	if s == nil || group == nil || !s.AllowsDailyOverdraft(group) || !group.HasDailyLimit() || !s.IsDayValidityUnit() {
		return 0
	}
	limit := *group.DailyLimitUSD
	if limit <= 0 {
		return 0
	}
	used := s.DailyOverdraftUsedUSDAt(group, now)
	if used <= 0 {
		return 0
	}
	totalDays := int(math.Ceil(used/limit - 1e-9))
	if totalDays < 0 {
		totalDays = 0
	}
	borrowedDays := totalDays - s.dailyOverdraftNormalDays(now)
	if maxBorrowed := s.OverdraftValidityDays() - s.dailyOverdraftNormalDays(now); borrowedDays > maxBorrowed {
		borrowedDays = maxBorrowed
	}
	if borrowedDays < 0 {
		return 0
	}
	return borrowedDays
}

func (s *UserSubscription) IsWindowActivated() bool {
	return s.DailyWindowStart != nil || s.WeeklyWindowStart != nil || s.MonthlyWindowStart != nil
}

func (s *UserSubscription) HasOneTimeDailyQuota() bool {
	if s == nil || s.StartsAt.IsZero() || s.ExpiresAt.IsZero() {
		return false
	}
	return !s.ExpiresAt.After(s.StartsAt.AddDate(0, 0, 1))
}

func (s *UserSubscription) NeedsDailyReset() bool {
	return s.NeedsDailyResetAt(time.Now())
}

func (s *UserSubscription) NeedsDailyResetAt(now time.Time) bool {
	if s.DailyWindowStart == nil {
		return false
	}
	if s.HasOneTimeDailyQuota() {
		return false
	}
	return windowNeedsReset(s.StartsAt, s.DailyWindowStart, now, dailyWindowDuration)
}

func (s *UserSubscription) NeedsWeeklyReset() bool {
	if s.WeeklyWindowStart == nil {
		return false
	}
	return windowNeedsReset(s.StartsAt, s.WeeklyWindowStart, time.Now(), weeklyWindowDuration)
}

func (s *UserSubscription) NeedsMonthlyReset() bool {
	if s.MonthlyWindowStart == nil {
		return false
	}
	return windowNeedsReset(s.StartsAt, s.MonthlyWindowStart, time.Now(), monthlyWindowDuration)
}

func (s *UserSubscription) DailyResetTime() *time.Time {
	if s.DailyWindowStart == nil {
		return nil
	}
	if s.HasOneTimeDailyQuota() {
		t := s.ExpiresAt
		return &t
	}
	return windowResetTime(s.StartsAt, s.DailyWindowStart, time.Now(), dailyWindowDuration)
}

func (s *UserSubscription) WeeklyResetTime() *time.Time {
	if s.WeeklyWindowStart == nil {
		return nil
	}
	return windowResetTime(s.StartsAt, s.WeeklyWindowStart, time.Now(), weeklyWindowDuration)
}

func (s *UserSubscription) MonthlyResetTime() *time.Time {
	if s.MonthlyWindowStart == nil {
		return nil
	}
	return windowResetTime(s.StartsAt, s.MonthlyWindowStart, time.Now(), monthlyWindowDuration)
}

func (s *UserSubscription) CurrentDailyWindowStart(now time.Time) time.Time {
	return currentWindowStart(s.StartsAt, now, dailyWindowDuration)
}

func (s *UserSubscription) CurrentWeeklyWindowStart(now time.Time) time.Time {
	return currentWindowStart(s.StartsAt, now, weeklyWindowDuration)
}

func (s *UserSubscription) CurrentMonthlyWindowStart(now time.Time) time.Time {
	return currentWindowStart(s.StartsAt, now, monthlyWindowDuration)
}

func (s *UserSubscription) AllowsDailyOverdraft(group *Group) bool {
	return s != nil && s.AllowDailyOverdraft && group != nil && group.AllowsDailyOverdraft()
}

func (s *UserSubscription) CheckDailyLimit(group *Group, additionalCost float64) bool {
	return s.checkDailyLimitAt(group, additionalCost, time.Now())
}

func (s *UserSubscription) checkDailyLimitAt(group *Group, additionalCost float64, now time.Time) bool {
	if group == nil || !group.HasDailyLimit() {
		return true
	}
	if s.AllowsDailyOverdraft(group) {
		limit, ok := s.DailyOverdraftLimitUSD(group)
		if !ok {
			return false
		}
		used := s.DailyOverdraftUsedUSDAt(group, now)
		if additionalCost <= 0 {
			return used < limit
		}
		return used+additionalCost <= limit
	}
	debt := s.DailyOverdraftDebtUSD(group, now)
	if additionalCost <= 0 {
		return s.DailyUsageUSD+debt < *group.DailyLimitUSD
	}
	return s.DailyUsageUSD+debt+additionalCost <= *group.DailyLimitUSD
}

func (s *UserSubscription) CheckWeeklyLimit(group *Group, additionalCost float64) bool {
	if group == nil || !group.HasWeeklyLimit() || s.AllowsDailyOverdraft(group) {
		return true
	}
	if additionalCost <= 0 {
		return s.WeeklyUsageUSD < *group.WeeklyLimitUSD
	}
	return s.WeeklyUsageUSD+additionalCost <= *group.WeeklyLimitUSD
}

func (s *UserSubscription) CheckMonthlyLimit(group *Group, additionalCost float64) bool {
	if group == nil || !group.HasMonthlyLimit() || s.AllowsDailyOverdraft(group) {
		return true
	}
	if additionalCost <= 0 {
		return s.MonthlyUsageUSD < *group.MonthlyLimitUSD
	}
	return s.MonthlyUsageUSD+additionalCost <= *group.MonthlyLimitUSD
}

func (s *UserSubscription) HasTotalQuotaLimit() bool {
	return s != nil && s.QuotaLimitUSD != nil && *s.QuotaLimitUSD > 0
}

func (s *UserSubscription) CheckTotalQuota(additionalCost float64) bool {
	if !s.HasTotalQuotaLimit() {
		return true
	}
	if additionalCost <= 0 {
		return s.QuotaUsedUSD < *s.QuotaLimitUSD
	}
	return s.QuotaUsedUSD+additionalCost <= *s.QuotaLimitUSD
}

func (s *UserSubscription) QuotaRemainingUSD() *float64 {
	if !s.HasTotalQuotaLimit() {
		return nil
	}
	remaining := *s.QuotaLimitUSD - s.QuotaUsedUSD
	if remaining < 0 {
		remaining = 0
	}
	return &remaining
}

func (s *UserSubscription) CheckAllLimits(group *Group, additionalCost float64) (daily, weekly, monthly bool) {
	daily = s.CheckDailyLimit(group, additionalCost)
	weekly = s.CheckWeeklyLimit(group, additionalCost)
	monthly = s.CheckMonthlyLimit(group, additionalCost)
	return
}

func currentWindowStart(anchor, now time.Time, duration time.Duration) time.Time {
	if !now.After(anchor) {
		return anchor
	}
	if duration <= 0 {
		return anchor
	}
	elapsed := now.Sub(anchor)
	return anchor.Add((elapsed / duration) * duration)
}

func effectiveStoredWindowStart(anchor time.Time, storedStart *time.Time) *time.Time {
	if storedStart == nil {
		return nil
	}
	return storedStart
}

func windowNeedsReset(anchor time.Time, storedStart *time.Time, now time.Time, duration time.Duration) bool {
	effective := effectiveStoredWindowStart(anchor, storedStart)
	if effective == nil {
		return false
	}
	if duration <= 0 {
		return false
	}
	return !now.Before(effective.Add(duration))
}

func windowResetTime(anchor time.Time, storedStart *time.Time, now time.Time, duration time.Duration) *time.Time {
	effective := effectiveStoredWindowStart(anchor, storedStart)
	if effective == nil {
		return nil
	}
	if duration <= 0 {
		resetAt := *effective
		return &resetAt
	}
	resetAt := effective.Add(duration)
	return &resetAt
}
