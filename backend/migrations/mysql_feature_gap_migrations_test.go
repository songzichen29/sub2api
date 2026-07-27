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

func TestMySQLEnableOpenAIAdvancedSchedulerMigrationExists(t *testing.T) {
	content, err := MySQLFS.ReadFile("029_enable_openai_advanced_scheduler.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "openai_advanced_scheduler_enabled")
	require.Contains(t, sql, "ON DUPLICATE KEY UPDATE")
	requireNotPostgresOnlySQL(t, sql)
}

func TestMySQLUsageLogUpstreamFirstEventMsMigrationExists(t *testing.T) {
	content, err := MySQLFS.ReadFile("030_add_usage_log_upstream_first_event_ms.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "table_name = 'usage_logs'")
	require.Contains(t, sql, "column_name = 'upstream_first_event_ms'")
	require.Contains(t, sql, "ALTER TABLE `usage_logs` ADD COLUMN `upstream_first_event_ms` int NULL")
	requireNotPostgresOnlySQL(t, sql)
}

func TestMySQLAccountSparkShadowMigrationExists(t *testing.T) {
	content, err := MySQLFS.ReadFile("042_account_spark_shadow.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "table_name = 'accounts'")
	require.Contains(t, sql, "column_name = 'parent_account_id'")
	require.Contains(t, sql, "ADD COLUMN `parent_account_id` BIGINT NULL")
	require.Contains(t, sql, "column_name = 'quota_dimension'")
	require.Contains(t, sql, "ADD COLUMN `quota_dimension` VARCHAR(20) NOT NULL DEFAULT ''global''")
	require.Contains(t, sql, "chk_accounts_quota_dimension")
	require.Contains(t, sql, "CHECK (`quota_dimension` IN (''global'', ''spark''))")
	require.Contains(t, sql, "chk_accounts_parent_dimension")
	require.Contains(t, sql, "`parent_account_id` IS NOT NULL")
	require.Contains(t, sql, "`quota_dimension` <> ''global''")
	// [INFO] MySQL CHECK 不能引用自增列（Error 3818），042 不再创建 chk_accounts_parent_not_self。
	require.NotContains(t, sql, "chk_accounts_parent_not_self")
	require.Contains(t, sql, "column_name = 'spark_shadow_parent_key'")
	require.Contains(t, sql, "GENERATED ALWAYS AS")
	require.Contains(t, sql, "CASE WHEN `quota_dimension` = ''spark'' AND `deleted_at` IS NULL THEN `parent_account_id` ELSE NULL END")
	require.Contains(t, sql, "CREATE INDEX `idx_accounts_parent_account_id` ON `accounts` (`parent_account_id`)")
	require.Contains(t, sql, "CREATE UNIQUE INDEX `uq_accounts_spark_shadow_per_parent` ON `accounts` (`spark_shadow_parent_key`)")
	require.Contains(t, sql, "FOREIGN KEY (`parent_account_id`) REFERENCES `accounts` (`id`) ON DELETE RESTRICT")
	requireNotPostgresOnlySQL(t, sql)
}

func TestMySQLGroupPeakRateMultiplierMigrationExists(t *testing.T) {
	content, err := MySQLFS.ReadFile("043_add_group_peak_rate_multiplier.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "table_name = 'groups'")
	require.Contains(t, sql, "column_name = 'peak_rate_enabled'")
	require.Contains(t, sql, "ADD COLUMN `peak_rate_enabled` BOOLEAN NOT NULL DEFAULT FALSE")
	require.Contains(t, sql, "column_name = 'peak_start'")
	require.Contains(t, sql, "ADD COLUMN `peak_start` VARCHAR(5) NOT NULL DEFAULT ''''")
	require.Contains(t, sql, "column_name = 'peak_end'")
	require.Contains(t, sql, "ADD COLUMN `peak_end` VARCHAR(5) NOT NULL DEFAULT ''''")
	require.Contains(t, sql, "column_name = 'peak_rate_multiplier'")
	require.Contains(t, sql, "ADD COLUMN `peak_rate_multiplier` DECIMAL(10,4) NOT NULL DEFAULT 1.0")
	requireNotPostgresOnlySQL(t, sql)
}

func TestMySQLBatchImageAndFrozenBalanceMigrationExists(t *testing.T) {
	content, err := MySQLFS.ReadFile("044_batch_image_foundation_and_user_frozen_balance.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS `batch_image_jobs`")
	require.Contains(t, sql, "`id` BIGINT NOT NULL AUTO_INCREMENT")
	require.Contains(t, sql, "`payload` JSON NULL")
	require.Contains(t, sql, "`parent_batch_id` VARCHAR(64) NULL")
	require.Contains(t, sql, "`base_unit_price` DECIMAL(20,10) NOT NULL DEFAULT 0")
	require.Contains(t, sql, "`batch_image_discount_multiplier` DECIMAL(10,4) NOT NULL DEFAULT 0.5")
	require.Contains(t, sql, "ADD COLUMN `frozen_balance` DECIMAL(20,8) NOT NULL DEFAULT 0")
	require.Contains(t, sql, "ADD COLUMN `allow_batch_image_generation` BOOLEAN NOT NULL DEFAULT FALSE")
	require.Contains(t, sql, "DATE_FORMAT(DATE_ADD(`created_at`, INTERVAL 8 HOUR), '%Y-%m-%d %H:%i:%s')")
	requireNotPostgresOnlySQL(t, sql)
}

func TestMySQLGroupWebSearchPricePerCallMigrationExists(t *testing.T) {
	content, err := MySQLFS.ReadFile("045_group_web_search_price_per_call.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "table_name = 'groups'")
	require.Contains(t, sql, "column_name = 'web_search_price_per_call'")
	require.Contains(t, sql, "ADD COLUMN `web_search_price_per_call` DECIMAL(20,8) NULL")
	requireNotPostgresOnlySQL(t, sql)
}

func TestMySQLContentModerationAccountMigrationExists(t *testing.T) {
	content, err := MySQLFS.ReadFile("046_content_moderation_account.sql")
	require.NoError(t, err)
	sql := string(content)

	require.Contains(t, sql, "ADD COLUMN `account_id` BIGINT NULL")
	require.Contains(t, sql, "ADD COLUMN `account_name` VARCHAR(255) NOT NULL DEFAULT")
	require.Contains(t, sql, "idx_content_moderation_logs_request_api_key")
	require.Contains(t, sql, "content_moderation_logs_accounts_account")
}

func TestMySQLLatestAPIKeyIPIndexMigrationExists(t *testing.T) {
	content, err := MySQLFS.ReadFile("047_add_usage_logs_api_key_latest_ip_index.sql")
	require.NoError(t, err)
	sql := string(content)

	require.Contains(t, sql, "index_name = 'idx_usage_logs_api_key_latest_ip'")
	require.Contains(t, sql, "CREATE INDEX `idx_usage_logs_api_key_latest_ip` ON `usage_logs` (`api_key_id`, `created_at` DESC, `id` DESC, `ip_address`)")
	requireNotPostgresOnlySQL(t, sql)
}

func TestMySQLUsageLogLongContextBillingMigrationExists(t *testing.T) {
	content, err := MySQLFS.ReadFile("048_add_usage_log_long_context_billing.sql")
	require.NoError(t, err)
	sql := string(content)

	require.Contains(t, sql, "table_name = 'usage_logs'")
	require.Contains(t, sql, "column_name = 'long_context_billing_applied'")
	require.Contains(t, sql, "ADD COLUMN `long_context_billing_applied` BOOLEAN NOT NULL DEFAULT FALSE")
	requireNotPostgresOnlySQL(t, sql)
}

func TestMySQLOpsSystemLogsHostMigrationExists(t *testing.T) {
	content, err := MySQLFS.ReadFile("049_add_ops_system_logs_host.sql")
	require.NoError(t, err)
	sql := string(content)

	require.Contains(t, sql, "table_name = 'ops_system_logs'")
	require.Contains(t, sql, "column_name = 'host'")
	require.Contains(t, sql, "ADD COLUMN `host` VARCHAR(255) NULL")
	require.Contains(t, sql, "idx_ops_system_logs_host_created_at")
	requireNotPostgresOnlySQL(t, sql)
}

func TestMySQLDefaultOpenAILongContextBillingMigrationExists(t *testing.T) {
	content, err := MySQLFS.ReadFile("050_default_openai_long_context_billing.sql")
	require.NoError(t, err)
	sql := string(content)

	require.Contains(t, sql, "openai_long_context_billing_enabled")
	require.Contains(t, sql, "parent_account_id IS NULL")
	require.Contains(t, sql, "quota_dimension = 'spark'")
	require.Contains(t, sql, "JSON_SET")
	requireNotPostgresOnlySQL(t, sql)
}

func TestMySQLGroupVideoRateMigrationExists(t *testing.T) {
	content, err := MySQLFS.ReadFile("051_add_group_video_rate.sql")
	require.NoError(t, err)

	sql := string(content)
	require.Contains(t, sql, "table_name = 'groups'")
	require.Contains(t, sql, "column_name = 'video_rate_independent'")
	require.Contains(t, sql, "ADD COLUMN `video_rate_independent` BOOLEAN NOT NULL DEFAULT FALSE")
	require.Contains(t, sql, "column_name = 'video_rate_multiplier'")
	require.Contains(t, sql, "ADD COLUMN `video_rate_multiplier` DECIMAL(10,4) NOT NULL DEFAULT 1.0000")
	require.Contains(t, sql, "column_name = 'video_price_480p'")
	require.Contains(t, sql, "ADD COLUMN `video_price_480p` DECIMAL(20,8) NULL")
	require.Contains(t, sql, "column_name = 'video_price_720p'")
	require.Contains(t, sql, "ADD COLUMN `video_price_720p` DECIMAL(20,8) NULL")
	require.Contains(t, sql, "column_name = 'video_price_1080p'")
	require.Contains(t, sql, "ADD COLUMN `video_price_1080p` DECIMAL(20,8) NULL")
	requireNotPostgresOnlySQL(t, sql)
}

func TestMySQLUpstreamFeatureMigrationsExist(t *testing.T) {
	tests := []struct {
		name     string
		required []string
	}{
		{"053_add_subscription_plan_currency.sql", []string{"subscription_plans", "`currency` VARCHAR(3)"}},
		{"054_channel_image_input_price.sql", []string{"channel_model_pricing", "`image_input_price` DECIMAL(20,12)"}},
		{"055_usage_log_image_input_tokens.sql", []string{"`image_input_tokens` INT", "`image_input_cost` DECIMAL(20,10)"}},
		{"056_audit_logs.sql", []string{"CREATE TABLE IF NOT EXISTS `audit_logs`", "`extra` JSON", "idx_audit_logs_created_at_id"}},
		{"057_group_duplicate_operation_id.sql", []string{"`duplicate_operation_id` VARCHAR(64)", "GENERATED ALWAYS AS", "idx_groups_duplicate_operation_id_active"}},
		{"058_prompt_audit.sql", []string{"CREATE TABLE IF NOT EXISTS `prompt_audit_jobs`", "CREATE TABLE IF NOT EXISTS `prompt_audit_events`", "JSON_TYPE(`categories`) = 'ARRAY'"}},
		{"059_prompt_audit_full_prompt.sql", []string{"prompt_audit_events", "`full_prompt` LONGTEXT NOT NULL"}},
		{"060_ops_ingress_reject_aggregates.sql", []string{"CREATE TABLE IF NOT EXISTS `ops_ingress_reject_aggregates`", "`client_ip` VARCHAR(45)", "ops_ingress_reject_aggregates_dimensions_unique"}},
		{"061_auth_cache_invalidation_outbox.sql", []string{"CREATE TABLE IF NOT EXISTS `auth_cache_invalidation_outbox`", "SHA2(OLD.`key`, 256)", "trg_user_allowed_groups_auth_cache_delete"}},
		{"062_group_reasoning_effort_policy.sql", []string{"`max_reasoning_effort` VARCHAR(20)", "`reasoning_effort_mappings` JSON"}},
		{"063_add_usage_log_billing_mode.sql", []string{"table_name = 'usage_logs'", "column_name = 'billing_mode'", "ADD COLUMN `billing_mode` varchar(20) NULL"}},
		{"064_composite_model_routes.sql", []string{"CREATE TABLE IF NOT EXISTS `composite_model_routes`", "idx_composite_model_routes_unique_active", "fk_composite_model_routes_group"}},
		{"065_alipay_mobile_precreate_deep_link.sql", []string{"INSERT IGNORE INTO `settings`", "ALIPAY_MOBILE_PRECREATE_DEEP_LINK"}},
		{"066_group_auth_cache_image_generation.sql", []string{"trg_groups_auth_cache_invalidation_update", "allow_image_generation", "SHA2(k.`key`, 256)"}},
		{"067_add_usage_log_session_id.sql", []string{"table_name = 'usage_logs'", "table_name = 'batch_image_jobs'", "ADD COLUMN `session_id` VARCHAR(255)"}},
		{"068_allow_live_usage_request_type.sql", []string{"chk_usage_logs_request_type", "`request_type` >= 0", "`request_type` <= 5"}},
		{"069_add_group_allow_live.sql", []string{"table_name = 'groups'", "column_name = 'allow_live'", "ADD COLUMN `allow_live` BOOLEAN NOT NULL DEFAULT FALSE"}},
		{"070_add_users_email_alias_dedup_index.sql", []string{"email_dot_stripped", "GENERATED ALWAYS AS", "idx_users_email_dot_stripped"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := MySQLFS.ReadFile(tt.name)
			require.NoError(t, err)
			sql := string(content)
			for _, fragment := range tt.required {
				require.Contains(t, sql, fragment)
			}
			requireNotPostgresOnlySQL(t, sql)
		})
	}
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
