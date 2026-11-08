package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOrderedAccountAPIKeysNormalizeAndPreserve(t *testing.T) {
	credentials := map[string]any{"api_keys": []any{" first ", "second"}, "base_url": "https://upstream.example"}
	require.NoError(t, NormalizeAccountAPIKeys(AccountTypeAPIKey, credentials))
	require.Equal(t, "first", credentials["api_key"])
	account := &Account{Type: AccountTypeAPIKey, Credentials: credentials}
	require.Equal(t, []string{"first", "second"}, account.OrderedAPIKeys())
	copy := account.OrderedAPIKeys()
	copy[0] = "changed"
	require.Equal(t, "first", account.OrderedAPIKeys()[0])

	merged := MergePreservingSensitiveCreds(credentials, map[string]any{"base_url": "https://new.example"})
	require.NoError(t, NormalizeAccountAPIKeys(AccountTypeAPIKey, merged))
	require.Equal(t, []string{"first", "second"}, merged["api_keys"])

	merged = MergePreservingSensitiveCreds(credentials, map[string]any{"api_key": "replacement"})
	require.NoError(t, NormalizeAccountAPIKeys(AccountTypeAPIKey, merged))
	require.NotContains(t, merged, "api_keys")
	require.Equal(t, "replacement", merged["api_key"])
	require.Equal(t, []string{"first", "second"}, credentials["api_keys"])

	merged = MergePreservingSensitiveCreds(credentials, map[string]any{"api_keys": []any{"new-first", "new-second"}})
	require.NoError(t, NormalizeAccountAPIKeys(AccountTypeAPIKey, merged))
	require.Equal(t, "new-first", merged["api_key"])
	require.True(t, IsSensitiveCredentialKey("api_keys"))
}

func TestOrderedAccountAPIKeysValidation(t *testing.T) {
	for name, raw := range map[string]any{
		"empty": []any{}, "not_array": "secret-value", "non_string": []any{123},
		"blank": []any{"first", " "}, "duplicate": []any{"secret-value", "secret-value"},
		"whitespace": []any{"secret-value bad"}, "control": []any{"secret-value\r\n" + "bad"},
		"too_many": make([]string, MaxAccountAPIKeys+1),
	} {
		t.Run(name, func(t *testing.T) {
			err := NormalizeAccountAPIKeys(AccountTypeAPIKey, map[string]any{"api_keys": raw})
			require.Error(t, err)
			require.NotContains(t, err.Error(), "secret-value")
		})
	}
	require.Error(t, NormalizeAccountAPIKeys(AccountTypeOAuth, map[string]any{"api_keys": []string{"a", "b"}}))
	credentials := map[string]any{"api_keys": []string{"one"}}
	require.NoError(t, NormalizeAccountAPIKeys(AccountTypeAPIKey, credentials))
	require.Equal(t, map[string]any{"api_key": "one"}, credentials)
	require.NoError(t, NormalizeAccountAPIKeys(AccountTypeOAuth, nil))
}
