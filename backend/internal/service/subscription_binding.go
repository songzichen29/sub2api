package service

import "context"

func (s *SubscriptionService) GetAPIKeySubscription(ctx context.Context, key *APIKey) (*UserSubscription, error) {
	if key.SubscriptionID == nil {
		return s.GetActiveSubscription(ctx, key.User.ID, key.Group.ID)
	}
	return s.GetActiveSubscription(ctx, key.User.ID, key.Group.ID, *key.SubscriptionID)
}

func (s *SubscriptionService) ResolveAPIKeySubscriptionError(ctx context.Context, key *APIKey) error {
	if key.SubscriptionID == nil {
		return s.ResolveSubscriptionError(ctx, key.User.ID, key.Group.ID)
	}
	return s.ResolveSubscriptionError(ctx, key.User.ID, key.Group.ID, *key.SubscriptionID)
}

func (s *SubscriptionService) findBoundSubscription(ctx context.Context, userID, groupID int64, subscriptionID ...int64) (*UserSubscription, error) {
	if len(subscriptionID) == 0 {
		return s.userSubRepo.GetByUserIDAndGroupID(ctx, userID, groupID)
	}
	sub, err := s.userSubRepo.GetByID(ctx, subscriptionID[0])
	if err != nil {
		return nil, err
	}
	if sub.UserID != userID || sub.GroupID != groupID {
		return nil, ErrSubscriptionNotFound
	}
	return sub, nil
}

func (s *SubscriptionService) loadBoundSubscription(ctx context.Context, userID, groupID int64, subscriptionID ...int64) (*UserSubscription, error) {
	sub, err := s.findBoundSubscription(ctx, userID, groupID, subscriptionID...)
	if err != nil {
		return nil, err
	}
	if !sub.IsActive() {
		return nil, ErrSubscriptionNotFound
	}
	return sub, nil
}
