package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMySQLContentModerationMigrationExists(t *testing.T) {
	content, err := MySQLFS.ReadFile("016_content_moderation.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "INSERT IGNORE INTO `settings`")
	require.Contains(t, sql, "'risk_control_enabled', 'false'")
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS `content_moderation_logs`")
	require.Contains(t, sql, "`category_scores` json NOT NULL")
	require.Contains(t, sql, "`threshold_snapshot` json NOT NULL")
	require.Contains(t, sql, "`violation_count` int NOT NULL DEFAULT 0")
	require.Contains(t, sql, "`auto_banned` bool NOT NULL DEFAULT FALSE")
	require.Contains(t, sql, "`email_sent` bool NOT NULL DEFAULT FALSE")
	require.Contains(t, sql, "`queue_delay_ms` int NULL")
	require.Contains(t, sql, "`idx_content_moderation_logs_group_created_at`")
	requireNotPostgresOnlySQL(t, sql)
}

func TestMySQLChannelMonitorAPIModeMigrationExists(t *testing.T) {
	content, err := MySQLFS.ReadFile("017_channel_monitor_openai_api_mode.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "table_name = 'channel_monitors'")
	require.Contains(t, sql, "column_name = 'api_mode'")
	require.Contains(t, sql, "ADD COLUMN `api_mode` varchar(32) NOT NULL DEFAULT ''chat_completions''")
	require.Contains(t, sql, "table_name = 'channel_monitor_request_templates'")
	require.Contains(t, sql, "`idx_channel_monitors_provider_api_mode`")
	require.Contains(t, sql, "`idx_channel_monitor_templates_provider_api_mode`")
	requireNotPostgresOnlySQL(t, sql)
}

func TestMySQLSeedOpenAIMonitorTemplatesMigrationExists(t *testing.T) {
	content, err := MySQLFS.ReadFile("018_seed_openai_monitor_templates.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "INSERT IGNORE INTO `channel_monitor_request_templates`")
	require.Contains(t, sql, "`created_at`, `updated_at`, `name`, `provider`, `api_mode`")
	require.Contains(t, sql, "'OpenAI Compatible 默认检测'")
	require.Contains(t, sql, "'OpenAI Responses / 本站自检'")
	require.Contains(t, sql, "'responses'")
	require.Contains(t, sql, `'{"max_output_tokens": 20}'`)
	requireNotPostgresOnlySQL(t, sql)
}

func TestMySQLSubscriptionExpiryNotifyEnabledMigrationExists(t *testing.T) {
	content, err := MySQLFS.ReadFile("019_subscription_expiry_notify_enabled.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "INSERT IGNORE INTO `settings`")
	require.Contains(t, sql, "'subscription_expiry_notify_enabled', 'true'")
	requireNotPostgresOnlySQL(t, sql)
}

func TestMySQLProxyExpiryFallbackMigrationExists(t *testing.T) {
	content, err := MySQLFS.ReadFile("026_proxy_expiry_fallback.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "table_name = 'proxies'")
	require.Contains(t, sql, "column_name = 'expires_at'")
	require.Contains(t, sql, "ADD COLUMN `expires_at` datetime(6) NULL")
	require.Contains(t, sql, "ADD COLUMN `fallback_mode` varchar(20) NOT NULL DEFAULT ''none''")
	require.Contains(t, sql, "ADD COLUMN `backup_proxy_id` bigint NULL")
	require.Contains(t, sql, "ADD COLUMN `expiry_warn_days` int NOT NULL DEFAULT 7")
	require.Contains(t, sql, "CREATE INDEX `proxy_expires_at` ON `proxies` (`expires_at`)")
	require.Contains(t, sql, "CREATE INDEX `proxy_backup_proxy_id` ON `proxies` (`backup_proxy_id`)")
	require.Contains(t, sql, "FOREIGN KEY (`backup_proxy_id`) REFERENCES `proxies` (`id`) ON DELETE SET NULL")
	require.Contains(t, sql, "table_name = 'accounts'")
	require.Contains(t, sql, "ADD COLUMN `proxy_fallback_origin_id` bigint NULL")
	requireNotPostgresOnlySQL(t, sql)
}

func TestMySQLAccountGroupSchedulerIndexesMigrationExists(t *testing.T) {
	content, err := MySQLFS.ReadFile("028_account_group_scheduler_indexes.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "table_name = 'account_groups'")
	require.Contains(t, sql, "index_name = 'idx_account_groups_group_priority_account'")
	require.Contains(t, sql, "CREATE INDEX `idx_account_groups_group_priority_account` ON `account_groups` (`group_id`, `priority`, `account_id`)")
	require.Contains(t, sql, "index_name = 'idx_account_groups_account_priority_group'")
	require.Contains(t, sql, "CREATE INDEX `idx_account_groups_account_priority_group` ON `account_groups` (`account_id`, `priority`, `group_id`)")
	requireNotPostgresOnlySQL(t, sql)
}

func requireNotPostgresOnlySQL(t *testing.T, sql string) {
	t.Helper()

	lower := strings.ToLower(sql)
	require.NotContains(t, lower, "on conflict")
	require.NotContains(t, lower, "concurrently")
	require.NotContains(t, lower, "::jsonb")
	require.NotContains(t, lower, "bigserial")
	require.NotContains(t, lower, "timestamptz")
	require.NotContains(t, lower, " ilike ")
	require.NotContains(t, lower, " returning ")
	require.NotContains(t, sql, "$1")
}
