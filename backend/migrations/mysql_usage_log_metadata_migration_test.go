package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMySQLUsageLogImageSizeMetadataMigrationExists(t *testing.T) {
	content, err := MySQLFS.ReadFile("015_usage_log_image_size_metadata.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "table_name = 'usage_logs'")
	require.Contains(t, sql, "column_name = 'image_input_size'")
	require.Contains(t, sql, "ADD COLUMN `image_input_size` varchar(32) NULL")
	require.Contains(t, sql, "column_name = 'image_output_size'")
	require.Contains(t, sql, "ADD COLUMN `image_output_size` varchar(32) NULL")
	require.Contains(t, sql, "column_name = 'image_size_source'")
	require.Contains(t, sql, "ADD COLUMN `image_size_source` varchar(16) NULL")
	require.Contains(t, sql, "column_name = 'image_size_breakdown'")
	require.Contains(t, sql, "ADD COLUMN `image_size_breakdown` json NULL")

	require.NotContains(t, strings.ToLower(sql), "payment_orders")
}
