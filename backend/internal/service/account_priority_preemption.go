package service

import "context"

// hasAvailableHigherPriorityAccount reports whether a statically eligible
// account with a numerically lower priority can accept work. Sticky sessions
// may keep using the current account only while no such account is available.
func hasAvailableHigherPriorityAccount(
	ctx context.Context,
	concurrencyService *ConcurrencyService,
	sticky *Account,
	candidates []*Account,
) bool {
	if sticky == nil || len(candidates) == 0 {
		return false
	}

	higherPriority := make([]*Account, 0, len(candidates))
	loads := make([]AccountWithConcurrency, 0, len(candidates))
	seen := make(map[int64]struct{}, len(candidates))
	for _, candidate := range candidates {
		if candidate == nil || candidate.ID <= 0 || candidate.ID == sticky.ID || candidate.Priority >= sticky.Priority {
			continue
		}
		if _, exists := seen[candidate.ID]; exists {
			continue
		}
		seen[candidate.ID] = struct{}{}
		higherPriority = append(higherPriority, candidate)
		loads = append(loads, AccountWithConcurrency{
			ID:             candidate.ID,
			MaxConcurrency: candidate.EffectiveLoadFactor(),
		})
	}
	if len(higherPriority) == 0 {
		return false
	}
	if concurrencyService == nil {
		return true
	}

	loadMap, err := concurrencyService.GetAccountsLoadBatch(ctx, loads)
	if err != nil {
		// The normal selection path already has an acquisition-based fallback
		// when batch load lookup fails. Let it make the authoritative decision.
		return true
	}
	for _, candidate := range higherPriority {
		loadInfo := loadMap[candidate.ID]
		if loadInfo == nil || loadInfo.LoadRate < 100 {
			return true
		}
	}
	return false
}
