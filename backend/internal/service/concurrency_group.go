package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// UserGroupConcurrencyCache keeps both limits in one atomic reservation. The
// global dimension continues to include other groups and active Live calls.
type UserGroupConcurrencyCache interface {
	AcquireUserGroupSlot(context.Context, int64, int64, int, int, string) (bool, error)
	ReleaseUserGroupSlot(context.Context, int64, int64, string) error
}

func (s *ConcurrencyService) acquireUserGroupSlot(ctx context.Context, userID, groupID int64, userMax, groupMax int) (*AcquireResult, error) {
	if s == nil || s.cache == nil {
		return nil, errors.New("concurrency cache is unavailable")
	}
	cache, ok := s.cache.(UserGroupConcurrencyCache)
	if !ok {
		return nil, errors.New("group concurrency is unsupported by the configured cache")
	}
	requestID := generateRequestID()
	acquired, err := cache.AcquireUserGroupSlot(ctx, userID, groupID, userMax, groupMax, requestID)
	if err != nil {
		return nil, err
	}
	if !acquired {
		return &AcquireResult{}, nil
	}
	var once sync.Once
	return &AcquireResult{Acquired: true, ReleaseFunc: func() {
		once.Do(func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := cache.ReleaseUserGroupSlot(cleanup, userID, groupID, requestID); err != nil {
				logger.LegacyPrintf("service.concurrency", "release group slot user=%d group=%d failed: %v", userID, groupID, err)
			}
		})
	}}, nil
}

// LiveUserGroupConcurrencyCache transfers the short request's occupancy to a
// renewable Live lease. A configured group limit must never silently fall back
// to a cache that only enforces the global dimension.
type LiveUserGroupConcurrencyCache interface {
	AcquireLiveLeaseForGroup(ctx context.Context, accountID int64, accountMax int, userID int64, userMax int, groupID int64, groupMax int, apiKeyID int64, leaseID string, replacingRegularSlots bool) (bool, error)
	RefreshLiveLeaseForGroup(ctx context.Context, accountID, userID, groupID, apiKeyID int64, leaseID string) (bool, error)
	ReleaseLiveLeaseForGroup(ctx context.Context, accountID, userID, groupID, apiKeyID int64, leaseID string) error
}

func liveGroupConcurrency(ctx context.Context, groupID *int64) (int64, int) {
	group, _ := ctx.Value(ctxkey.Group).(*Group)
	if group == nil || groupID == nil || group.ID != *groupID || group.ID <= 0 || group.UserConcurrencyLimit <= 0 {
		return 0, 0
	}
	return group.ID, group.UserConcurrencyLimit
}
