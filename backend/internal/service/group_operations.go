package service

import (
	"context"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Group operations are explicit additions. Zero disables the extra dimension;
// they never replace the authenticated group's pricing or permission identity.
func (s *adminServiceImpl) validateGroupOperations(ctx context.Context, id int64, platform string, concurrency int, fallback *int64) (*int64, error) {
	if concurrency < 0 || concurrency > 100000 {
		return nil, infraerrors.BadRequest("INVALID_GROUP_CONCURRENCY", "user_concurrency_limit must be between 0 and 100000")
	}
	if fallback == nil || *fallback == 0 {
		return nil, nil
	}
	invalid := func(message string) (*int64, error) {
		return nil, infraerrors.BadRequest("INVALID_UNAVAILABLE_FALLBACK_GROUP", message)
	}
	if *fallback < 0 || platform == PlatformComposite {
		return invalid("account fallback requires a positive target and a non-composite platform")
	}
	seen := map[int64]bool{id: true}
	next := *fallback
	for depth := 0; depth < 32; depth++ {
		if seen[next] {
			return invalid("account fallback groups must not form a cycle")
		}
		seen[next] = true
		target, err := s.groupRepo.GetByIDLite(ctx, next)
		if err != nil || target == nil {
			return invalid("account fallback group does not exist")
		}
		if target.Platform != platform || !target.IsActive() {
			return invalid("account fallback group must be active and use the same platform")
		}
		if target.FallbackGroupIDOnUnavailable == nil || *target.FallbackGroupIDOnUnavailable <= 0 {
			return cloneGroupValuePointer(fallback), nil
		}
		next = *target.FallbackGroupIDOnUnavailable
	}
	return invalid("account fallback chain is too long")
}
