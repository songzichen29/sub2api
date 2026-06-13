//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type createAccountGroupRepoStub struct {
	groupRepoStubForAdmin
	exists map[int64]bool
}

func (s *createAccountGroupRepoStub) ExistsByIDs(_ context.Context, ids []int64) (map[int64]bool, error) {
	out := make(map[int64]bool, len(ids))
	for _, id := range ids {
		out[id] = s.exists[id]
	}
	return out, nil
}

func TestAdminService_CreateAccount_RejectsMissingGroupBeforeCreate(t *testing.T) {
	accountRepo := newInMemoryAccountRepo()
	svc := &adminServiceImpl{
		accountRepo: accountRepo,
		groupRepo: &createAccountGroupRepoStub{
			exists: map[int64]bool{5: true},
		},
	}

	created, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name:                  "acc",
		Platform:              PlatformAnthropic,
		Type:                  AccountTypeAPIKey,
		Credentials:           map[string]any{"api_key": "tok"},
		Concurrency:           1,
		Priority:              1,
		GroupIDs:              []int64{5, 99},
		SkipDefaultGroupBind:  true,
		SkipMixedChannelCheck: true,
	})

	require.Nil(t, created)
	require.ErrorIs(t, err, ErrGroupNotFound)
	require.Empty(t, accountRepo.store)
}
