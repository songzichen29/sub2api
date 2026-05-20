package service

import (
	"time"
)

// SubscriptionCacheData represents cached subscription data
type SubscriptionCacheData struct {
	Status              string
	StartsAt            time.Time
	ExpiresAt           time.Time
	ValidityUnit        string
	DailyWindowStart    *time.Time
	DailyUsage          float64
	WeeklyUsage         float64
	MonthlyUsage        float64
	AllowDailyOverdraft bool
	Version             int64
}
