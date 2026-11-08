package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service/usage_provider"
	"github.com/stretchr/testify/require"
)

type apiKeyPoolStateRepo struct {
	AccountRepository
	paused, disabled int
	until            time.Time
}

func (r *apiKeyPoolStateRepo) SetTempUnschedulable(_ context.Context, _ int64, until time.Time, _ string) error {
	r.paused++
	r.until = until
	return nil
}

func (r *apiKeyPoolStateRepo) SetError(context.Context, int64, string) error {
	r.disabled++
	return nil
}
func (r *apiKeyPoolStateRepo) SetSchedulable(context.Context, int64, bool) error {
	r.disabled++
	return nil
}

func TestOrderedAccountAPIKeysExhaustionRemainsRecoverable(t *testing.T) {
	for _, status := range []int{401, 402, 429, 503} {
		repo := &apiKeyPoolStateRepo{}
		svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
		_, account := keyPoolRequest(t)
		account.Platform = PlatformOpenAI
		started := time.Now()
		require.True(t, svc.HandleUpstreamError(context.Background(), account, status, http.Header{}, []byte(`{"error":"insufficient_quota"}`)))
		require.Equal(t, 1, repo.paused)
		require.Zero(t, repo.disabled)
		require.WithinDuration(t, started.Add(15*time.Second), repo.until, time.Second)
	}
}

func TestOrderedAccountAPIKeysPrimaryQuotaDoesNotDisablePool(t *testing.T) {
	_, account := keyPoolRequest(t)
	account.Platform, account.Schedulable = PlatformOpenAI, true
	repo := &apiKeyPoolStateRepo{}
	svc := &AccountUsageService{accountRepo: repo}
	svc.disableSchedulingForZeroThirdPartyQuota(context.Background(), account, &usage_provider.QuotaInfo{Remaining: 0})
	require.Zero(t, repo.disabled)
	require.True(t, account.Schedulable)
	account.Extra = map[string]any{"codex_5h_used_percent": 100.0}
	paused, _ := shouldAutoPauseOpenAIAccountByQuota(context.Background(), account)
	require.False(t, paused)
	require.False(t, EvaluateAccountSchedulingThreshold(account, map[string]int{PlatformOpenAI: 80}, time.Now()).ShouldPause)
	check := &CNProviderBalanceCheckService{accountRepo: repo}
	require.Equal(t, cnBalanceNoChange, check.applyBalanceResult(context.Background(), account, &CNProviderBalanceResult{}, 1))
	require.Zero(t, repo.paused)
}

func TestOrderedAccountAPIKeysCreateRateSyncParity(t *testing.T) {
	enabled, disabled := true, false
	input := &CreateAccountInput{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "one"}, RateSyncEnabled: &enabled}
	account, err := buildAccountForCreate(input, nil)
	require.NoError(t, err)
	require.Equal(t, true, account.Extra[UpstreamBillingProbeEnabledExtraKey])
	require.Equal(t, true, account.Extra[UpstreamBillingRateSyncEnabledExtraKey])
	input.ProbeEnabled = &disabled
	_, err = buildAccountForCreate(input, nil)
	require.Error(t, err)
	input.ProbeEnabled, input.Type = nil, AccountTypeOAuth
	_, err = buildAccountForCreate(input, nil)
	require.Error(t, err)
}

func TestOrderedAccountAPIKeysBulkReplacements(t *testing.T) {
	for _, tc := range []struct {
		name  string
		patch map[string]any
		want  []string
	}{
		{"single", map[string]any{"api_key": "new"}, []string{"new"}},
		{"pool", map[string]any{"api_keys": []any{"new", "backup"}}, []string{"new", "backup"}},
		{"metadata", map[string]any{"base_url": "https://new.example"}, []string{"first", "second", "third"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, account := keyPoolRequest(t)
			repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: account}}
			_, err := (&adminServiceImpl{accountRepo: repo}).BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{AccountIDs: []int64{account.ID}, Credentials: tc.patch})
			require.NoError(t, err)
			require.Len(t, repo.bulkUpdates, 1)
			// Repository bulk updates merge JSON keys, unlike single-account PUT.
			merged := mergeMap(account.Credentials, repo.bulkUpdates[0].Credentials)
			require.Equal(t, tc.want, (&Account{Type: AccountTypeAPIKey, Credentials: merged}).OrderedAPIKeys())
		})
	}
	_, account := keyPoolRequest(t)
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{1: account, 2: {ID: 2, Type: AccountTypeOAuth}}}
	_, err := (&adminServiceImpl{accountRepo: repo}).BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{AccountIDs: []int64{1, 2}, Credentials: map[string]any{"api_keys": []any{"only"}}})
	require.Error(t, err)
	require.Empty(t, repo.bulkUpdates)
}

func TestOrderedAccountAPIKeysBackupRotationInvalidatesProbeIdentity(t *testing.T) {
	_, before := keyPoolRequest(t)
	after := *before
	after.Credentials = map[string]any{"api_key": "first", "api_keys": []string{"first", "replacement"}}
	require.NotEqual(t, upstreamBillingProbeIdentity(before), upstreamBillingProbeIdentity(&after))
	require.True(t, updatesUpstreamBillingProbeIdentity(map[string]any{"api_keys": []string{"first", "replacement"}}))
}

type apiKeyPoolWSDialer struct {
	calls  []string
	status int
}

func (d *apiKeyPoolWSDialer) Dial(_ context.Context, _ string, headers http.Header, _ string) (openAIWSClientConn, int, http.Header, error) {
	d.calls = append(d.calls, headers.Get("Authorization"))
	if headers.Get("Authorization") == "Bearer first" {
		return nil, d.status, nil, errors.New("handshake rejected")
	}
	return nil, http.StatusSwitchingProtocols, nil, nil
}

func TestOrderedAccountAPIKeysWSHandshakeRetryOnly(t *testing.T) {
	req, account := keyPoolRequest(t)
	for _, status := range []int{401, 429, 400, 0} {
		dialer := &apiKeyPoolWSDialer{status: status}
		_, _, _, _ = dialOpenAIWSWithAccountKeys(context.Background(), account, dialer, "wss://upstream.example/v1/responses", req.Header, "")
		if status == 401 || status == 429 {
			require.Equal(t, []string{"Bearer first", "Bearer second"}, dialer.calls)
		} else {
			require.Equal(t, []string{"Bearer first"}, dialer.calls)
		}
		require.Equal(t, "Bearer first", req.Header.Get("Authorization"))
	}
}
