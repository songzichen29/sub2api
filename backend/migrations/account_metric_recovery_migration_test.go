package migrations

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountMetricRecoveryMigrationsAreAdditiveAndDisabledByDefault(t *testing.T) {
	postgres, err := FS.ReadFile("253_account_metric_recovery.sql")
	require.NoError(t, err)
	mysql, err := MySQLFS.ReadFile("112_account_metric_recovery.sql")
	require.NoError(t, err)
	require.False(t, bytes.HasPrefix(mysql, []byte{0xEF, 0xBB, 0xBF}), "MySQL migration must not start with UTF-8 BOM")
	require.False(t, bytes.HasPrefix(postgres, []byte{0xEF, 0xBB, 0xBF}), "PostgreSQL migration must not start with UTF-8 BOM")
	// Every monitor INSERT supplies reason explicitly. Do not use a literal
	// TEXT default: MySQL requires expression syntax for that data type.
	require.Contains(t, string(mysql), "`reason` TEXT NOT NULL,")
	require.NotContains(t, string(mysql), "TEXT NOT NULL DEFAULT ''")

	for name, content := range map[string]string{"postgres": string(postgres), "mysql": string(mysql)} {
		t.Run(name, func(t *testing.T) {
			require.Contains(t, content, "account_performance_monitors")
			require.Contains(t, content, "account_performance_events")
			require.Contains(t, content, "DEFAULT FALSE")
			require.Contains(t, content, "pause_generation")
			require.Contains(t, content, "trial_requests")
			require.NotContains(t, content, "UPDATE accounts")
			require.NotContains(t, content, "schedulable =")
			require.NotContains(t, content, "status =")
		})
	}

	require.True(t, strings.HasPrefix(string(postgres), "CREATE TABLE"), "PostgreSQL migration should start with SQL, not a BOM or hidden prefix")
}
