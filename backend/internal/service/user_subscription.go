package service

import "time"

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

	DailyWindowStart   *time.Time
	WeeklyWindowStart  *time.Time
	MonthlyWindowStart *time.Time

	DailyUsageUSD   float64
	WeeklyUsageUSD  float64
	MonthlyUsageUSD float64

	AssignedBy *int64
	AssignedAt time.Time
	Notes      string

	// Source 标识订阅来源（admin/redeem/payment），决定 AdminResetQuota 是否允许重置。
	Source string

	CreatedAt time.Time
	UpdatedAt time.Time

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

func (s *UserSubscription) IsWindowActivated() bool {
	return s.DailyWindowStart != nil || s.WeeklyWindowStart != nil || s.MonthlyWindowStart != nil
}

func (s *UserSubscription) NeedsDailyReset() bool {
	if s.DailyWindowStart == nil {
		return false
	}
	return windowNeedsReset(s.StartsAt, s.DailyWindowStart, time.Now(), dailyWindowDuration)
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

func (s *UserSubscription) CheckDailyLimit(group *Group, additionalCost float64) bool {
	if !group.HasDailyLimit() {
		return true
	}
	return s.DailyUsageUSD+additionalCost <= *group.DailyLimitUSD
}

func (s *UserSubscription) CheckWeeklyLimit(group *Group, additionalCost float64) bool {
	if !group.HasWeeklyLimit() {
		return true
	}
	return s.WeeklyUsageUSD+additionalCost <= *group.WeeklyLimitUSD
}

func (s *UserSubscription) CheckMonthlyLimit(group *Group, additionalCost float64) bool {
	if !group.HasMonthlyLimit() {
		return true
	}
	return s.MonthlyUsageUSD+additionalCost <= *group.MonthlyLimitUSD
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
	start := *storedStart
	if start.Before(anchor) {
		start = anchor
	}
	return &start
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
