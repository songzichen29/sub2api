package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type apiKeyManagementRepo struct {
	AccountRepository
	account *Account
	updates int
}

func (r *apiKeyManagementRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}

func (r *apiKeyManagementRepo) Update(_ context.Context, account *Account) error {
	r.account = account
	r.updates++
	return nil
}

func TestAccountAPIKeyManagementUpdatePreservesOtherCredentials(t *testing.T) {
	for _, credentials := range []map[string]any{nil, {"base_url": "https://upstream.example", "model_mapping": map[string]string{"client": "upstream"}}} {
		repo := &apiKeyManagementRepo{account: &Account{ID: 412, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: map[string]any{
			"base_url": "https://upstream.example", "api_key": "first-secret", "api_keys": []string{"first-secret", "second-secret"},
			"model_mapping": map[string]string{"client": "upstream"},
		}}}
		pool := repo.account.APIKeyPoolSummary()
		update := &AccountAPIKeyUpdate{Revision: pool.Revision, Entries: []AccountAPIKeyEntry{{ID: pool.Keys[1].ID}}}
		svc := &adminServiceImpl{accountRepo: repo}
		result, err := svc.UpdateAccount(context.Background(), repo.account.ID, &UpdateAccountInput{Credentials: credentials, APIKeyUpdate: update})
		require.NoError(t, err)
		require.Equal(t, 1, repo.updates)
		require.Equal(t, "second-secret", result.Credentials["api_key"])
		require.NotContains(t, result.Credentials, "api_keys")
		require.Equal(t, "https://upstream.example", result.Credentials["base_url"])
		require.Equal(t, map[string]string{"client": "upstream"}, result.Credentials["model_mapping"])
		_, err = svc.UpdateAccount(context.Background(), repo.account.ID, &UpdateAccountInput{APIKeyUpdate: update})
		require.ErrorContains(t, err, "reload")
		require.Equal(t, 1, repo.updates)
	}
}

func TestAccountAPIKeyManagementPreservesOrdersAndReplacesKeys(t *testing.T) {
	account := &Account{ID: 407, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_key": "private-primary", "api_keys": []string{"private-primary", "private-backup"}, "base_url": "https://upstream.example",
	}}
	pool := account.APIKeyPoolSummary()
	encoded, err := json.Marshal(pool)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "private-primary")
	require.NotContains(t, string(encoded), "private-backup")
	require.Len(t, pool.Keys, 2)
	require.Equal(t, "••••••••mary", pool.Keys[0].Masked)

	resolved, err := account.ResolveAPIKeyUpdate(&AccountAPIKeyUpdate{Revision: pool.Revision, Entries: []AccountAPIKeyEntry{
		{ID: pool.Keys[1].ID}, {Value: "new-primary"}, {ID: pool.Keys[0].ID},
	}})
	require.NoError(t, err)
	require.Equal(t, []string{"private-backup", "new-primary", "private-primary"}, resolved["api_keys"])
	require.Equal(t, "private-backup", resolved["api_key"])
	require.Equal(t, []string{"private-primary", "private-backup"}, account.OrderedAPIKeys())

	resolved, err = account.ResolveAPIKeyUpdate(&AccountAPIKeyUpdate{Revision: pool.Revision, Entries: []AccountAPIKeyEntry{{ID: pool.Keys[1].ID}}})
	require.NoError(t, err)
	merged := MergePreservingSensitiveCreds(account.Credentials, resolved)
	require.Equal(t, "private-backup", merged["api_key"])
	require.NotContains(t, merged, "api_keys")
	key, err := account.APIKeyByID(pool.Keys[1].ID)
	require.NoError(t, err)
	require.Equal(t, "private-backup", key)
}

func TestAccountAPIKeyManagementRejectsStaleInvalidAndDuplicateEntries(t *testing.T) {
	account := &Account{ID: 408, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "private-primary"}}
	pool := account.APIKeyPoolSummary()
	for _, entries := range [][]AccountAPIKeyEntry{
		{}, {{ID: "unknown"}}, {{ID: pool.Keys[0].ID, Value: "replacement"}},
		{{ID: pool.Keys[0].ID}, {Value: "private-primary"}}, {{Value: "has spaces"}},
	} {
		_, err := account.ResolveAPIKeyUpdate(&AccountAPIKeyUpdate{Revision: pool.Revision, Entries: entries})
		require.Error(t, err)
		require.NotContains(t, err.Error(), "private-primary")
	}
	_, err := account.ResolveAPIKeyUpdate(&AccountAPIKeyUpdate{Revision: "stale", Entries: []AccountAPIKeyEntry{{ID: pool.Keys[0].ID}}})
	require.ErrorContains(t, err, "reload")
	account.ID++
	_, err = account.APIKeyByID(pool.Keys[0].ID)
	require.Error(t, err)
}

func TestAccountAPIKeySummaryOnlyUsesMatchingRecentSuccessfulRequest(t *testing.T) {
	account := &Account{ID: 409, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_keys": []string{"first", "second"}}}
	now := time.Now()
	recentAccountAPIKeyUses.Store(account.ID, accountAPIKeyUse{index: 1, usedAt: now, poolFingerprint: accountAPIKeyPoolFingerprint("", account.OrderedAPIKeys())})
	t.Cleanup(func() { recentAccountAPIKeyUses.Delete(account.ID) })
	pool := account.APIKeyPoolSummary()
	require.Nil(t, pool.Keys[0].LastUsedAt)
	require.Equal(t, &now, pool.Keys[1].LastUsedAt)
	account.Credentials["api_keys"] = []string{"second", "first"}
	require.Nil(t, account.APIKeyPoolSummary().Keys[1].LastUsedAt)
}
