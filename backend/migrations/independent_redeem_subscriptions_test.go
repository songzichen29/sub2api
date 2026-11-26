package migrations

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIndependentRedeemSubscriptionMigrationsPreserveLegacyEntitlements(t *testing.T) {
	postgres, err := FS.ReadFile("257_independent_redeem_subscriptions.sql")
	require.NoError(t, err)
	mysql, err := MySQLFS.ReadFile("117_independent_redeem_subscriptions.sql")
	require.NoError(t, err)
	for name, data := range map[string][]byte{"postgres": postgres, "mysql": mysql} {
		t.Run(name, func(t *testing.T) {
			require.False(t, bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}))
			require.NotContains(t, string(data), "\r")
			text := strings.ToLower(string(data))
			require.Contains(t, text, "redeem_code_id")
			require.Contains(t, text, "create unique index")
			require.Contains(t, text, "user_subscriptions_redeem_code_unique")
			require.NotContains(t, text, "foreign key")
			require.NotContains(t, text, "references")
			require.NotContains(t, text, "update user_subscriptions")
			require.NotContains(t, text, "delete from")
			require.NotContains(t, text, "quota_used_usd =")
			require.NotContains(t, text, "expires_at =")
		})
	}
	require.Contains(t, string(postgres), "DROP INDEX IF EXISTS user_subscriptions_legacy_user_group_unique")
	require.Contains(t, string(postgres), "WHERE deleted_at IS NULL AND purchase_order_id IS NULL AND redeem_code_id IS NULL")
	require.Contains(t, string(mysql), "ADD COLUMN `redeem_code_id` BIGINT NULL")
}
