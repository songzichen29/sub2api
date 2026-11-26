package service

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var ErrSubscriptionSelectionRequired = infraerrors.BadRequest("SUBSCRIPTION_SELECTION_REQUIRED", "请选择要使用的具体订阅")

// resolveKeySubscription 在密钥写入边界核对所有权与分组，避免同名订阅被暗中切换。
func (s *APIKeyService) resolveKeySubscription(ctx context.Context, user *User, groupID, selected *int64) (*int64, error) {
	if groupID == nil {
		if selected != nil {
			return nil, ErrGroupNotAllowed
		}
		return nil, nil
	}
	group, err := s.groupRepo.GetByID(ctx, *groupID)
	if err != nil {
		return nil, err
	}
	if !group.IsSubscriptionType() {
		if selected != nil {
			return nil, ErrGroupNotAllowed
		}
		if !user.CanBindGroup(group.ID, group.IsExclusive) {
			return nil, ErrGroupNotAllowed
		}
		return nil, nil
	}
	if selected != nil {
		sub, err := s.userSubRepo.GetByID(ctx, *selected)
		if err != nil {
			return nil, err
		}
		if sub.UserID != user.ID || sub.GroupID != group.ID {
			return nil, ErrGroupNotAllowed
		}
		if sub.Status != SubscriptionStatusActive || time.Now().Before(sub.StartsAt) || !sub.ExpiresAt.After(time.Now()) {
			return nil, ErrSubscriptionNotFound
		}
		return selected, nil
	}
	subscriptions, err := s.userSubRepo.ListActiveByUserID(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	var result *int64
	for _, sub := range subscriptions {
		if sub.GroupID != group.ID {
			continue
		}
		if result != nil {
			return nil, ErrSubscriptionSelectionRequired
		}
		id := sub.ID
		result = &id
	}
	if result == nil {
		return nil, ErrGroupNotAllowed
	}
	return result, nil
}
