package service

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBuildOAuthMetadataUserID_FallbackWithoutAccountUUID(t *testing.T) {
	svc := &GatewayService{}

	parsed := &ParsedRequest{
		Model:          "claude-sonnet-4-5",
		Stream:         true,
		MetadataUserID: "",
		System:         nil,
		Messages:       nil,
	}

	account := &Account{
		ID:    123,
		Type:  AccountTypeOAuth,
		Extra: map[string]any{}, // intentionally missing account_uuid / claude_user_id
	}

	fp := &Fingerprint{ClientID: "deadbeef"} // should be used as user id in legacy format

	got := svc.buildOAuthMetadataUserID(parsed, account, fp)
	require.NotEmpty(t, got)

	// Legacy format: user_{client}_account__session_{uuid}
	re := regexp.MustCompile(`^user_[a-zA-Z0-9]+_account__session_[a-f0-9-]{36}$`)
	require.True(t, re.MatchString(got), "unexpected user_id format: %s", got)
}

func TestBuildOAuthMetadataUserID_UsesAccountUUIDWhenPresent(t *testing.T) {
	svc := &GatewayService{}

	parsed := &ParsedRequest{
		Model:          "claude-sonnet-4-5",
		Stream:         true,
		MetadataUserID: "",
	}

	account := &Account{
		ID:   123,
		Type: AccountTypeOAuth,
		Extra: map[string]any{
			"account_uuid":      "acc-uuid",
			"claude_user_id":    "clientid123",
			"anthropic_user_id": "",
		},
	}

	got := svc.buildOAuthMetadataUserID(parsed, account, nil)
	require.NotEmpty(t, got)

	// New format: user_{client}_account_{account_uuid}_session_{uuid}
	re := regexp.MustCompile(`^user_clientid123_account_acc-uuid_session_[a-f0-9-]{36}$`)
	require.True(t, re.MatchString(got), "unexpected user_id format: %s", got)
}

func TestBuildOAuthMetadataUserID_ReplacesInvalidExistingMetadata(t *testing.T) {
	svc := &GatewayService{}

	parsed := &ParsedRequest{
		Model:          "claude-sonnet-4-5",
		Stream:         true,
		MetadataUserID: "not-a-claude-code-user-id",
	}
	account := &Account{
		ID:   123,
		Type: AccountTypeOAuth,
		Extra: map[string]any{
			"account_uuid": "acc-uuid",
		},
	}
	fp := &Fingerprint{
		ClientID:  "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		UserAgent: "claude-cli/2.1.197 (external, cli)",
	}

	got := svc.buildOAuthMetadataUserID(parsed, account, fp)
	require.NotEmpty(t, got)
	parsedUID := ParseMetadataUserID(got)
	require.NotNil(t, parsedUID)
	require.Equal(t, fp.ClientID, parsedUID.DeviceID)
	require.Equal(t, "acc-uuid", parsedUID.AccountUUID)
}

func TestEnsureClaudeOAuthMetadataUserID_OverwritesInvalidExisting(t *testing.T) {
	body := []byte(`{"metadata":{"user_id":"not-a-claude-code-user-id","other":"keep"},"messages":[]}`)
	uid := `{"device_id":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","account_uuid":"acc-uuid","session_id":"123e4567-e89b-12d3-a456-426614174000"}`

	got, changed := ensureClaudeOAuthMetadataUserID(body, uid)
	require.True(t, changed)
	require.Equal(t, uid, gjson.GetBytes(got, "metadata.user_id").String())
	require.Equal(t, "keep", gjson.GetBytes(got, "metadata.other").String())
}
