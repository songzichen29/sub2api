package migrations

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIndependentSubscriptionMigrationsPreserveExistingEntitlements(t *testing.T) {
	postgres, err := FS.ReadFile("256_independent_subscriptions.sql")
	require.NoError(t, err)
	mysql, err := MySQLFS.ReadFile("116_independent_subscriptions.sql")
	require.NoError(t, err)
	for name, data := range map[string][]byte{"postgres": postgres, "mysql": mysql} {
		t.Run(name, func(t *testing.T) {
			require.False(t, bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}))
			require.NotContains(t, string(data), "\r")
			text := strings.ToLower(string(data))
			require.Contains(t, text, "purchase_order_id")
			require.Contains(t, text, "create unique index")
			require.Contains(t, text, "having count(*) = 1")
			require.Contains(t, text, "k.subscription_id is null")
			require.NotContains(t, text, "update user_subscriptions")
			require.NotContains(t, text, "delete from")
			require.NotContains(t, text, "quota_used_usd =")
			require.NotContains(t, text, "expires_at =")
		})
	}
	require.Contains(t, string(postgres), "DROP INDEX IF EXISTS user_subscriptions_user_group_unique_active")
	require.Contains(t, string(postgres), "WHERE deleted_at IS NULL AND purchase_order_id IS NULL")
}
