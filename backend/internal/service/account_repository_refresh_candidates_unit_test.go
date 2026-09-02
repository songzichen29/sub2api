//go:build unit

package service

import "context"

func (s *accountRepoStub) ListOAuthRefreshCandidates(context.Context) ([]Account, error) {
	panic("unexpected ListOAuthRefreshCandidates call")
}

func (r *openAIAccountTestRepo) ListOAuthRefreshCandidates(context.Context) ([]Account, error) {
	panic("unexpected ListOAuthRefreshCandidates call")
}

func (m *groupAwareMockAccountRepo) ListOAuthRefreshCandidates(context.Context) ([]Account, error) {
	panic("unexpected ListOAuthRefreshCandidates call")
}

func (m *mockAccountRepoForPlatform) ListOAuthRefreshCandidates(context.Context) ([]Account, error) {
	panic("unexpected ListOAuthRefreshCandidates call")
}

func (m *mockAccountRepoForGemini) ListAllTags(context.Context) ([]string, error) {
	return nil, nil
}

func (r *thresholdSelectionAccountRepoStub) ListAllTags(context.Context) ([]string, error) {
	return nil, nil
}

func (r *inMemoryAccountRepo) ResetQuotaUsedAndClearRateLimitCooldown(context.Context, int64) error {
	return nil
}
