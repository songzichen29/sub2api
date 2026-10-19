-- MySQL schema parity for the upstream v0.2.17 feature set.
-- This migration is intentionally idempotent because production startup runs
-- only the MySQL migration filesystem.

-- Content moderation audit provenance.
SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'content_moderation_logs' AND column_name = 'engine_meta');
SET @sql = IF(@col_exists = 0,
  'ALTER TABLE `content_moderation_logs` ADD COLUMN `engine_meta` JSON NULL', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- Reasoning-effort pricing maps replace the legacy single max multiplier.
SET @reasoning_multipliers_exists = (SELECT COUNT(1) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'channel_model_pricing' AND column_name = 'reasoning_effort_multipliers');
SET @sql = IF(@reasoning_multipliers_exists = 0,
  'ALTER TABLE `channel_model_pricing` ADD COLUMN `reasoning_effort_multipliers` JSON NOT NULL DEFAULT (JSON_OBJECT())', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- Match the upstream one-time backfill while keeping this migration replay-safe.
-- A later replay must preserve an intentionally cleared or customized map.
SET @sql = IF(@reasoning_multipliers_exists = 0,
  'UPDATE `channel_model_pricing` SET `reasoning_effort_multipliers` = JSON_OBJECT(''max'', `max_reasoning_effort_multiplier`) WHERE `max_reasoning_effort_multiplier` IS NOT NULL',
  'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'channel_account_stats_model_pricing' AND column_name = 'reasoning_effort_multipliers');
SET @sql = IF(@col_exists = 0,
  'ALTER TABLE `channel_account_stats_model_pricing` ADD COLUMN `reasoning_effort_multipliers` JSON NOT NULL DEFAULT (JSON_OBJECT())', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- Preserve configured legacy max multipliers in the new JSON map. The
-- application keeps the old field for compatibility, so this update is safe
-- to re-run and only fills entries without an explicit map.
UPDATE `groups`
SET `model_pricing` = (
  SELECT JSON_ARRAYAGG(x.entry)
  FROM (
    SELECT j.ordinal,
      CASE
      WHEN JSON_TYPE(j.entry) = 'OBJECT'
       AND JSON_CONTAINS_PATH(j.entry, 'one', '$.max_reasoning_effort_multiplier')
       AND NOT JSON_CONTAINS_PATH(j.entry, 'one', '$.reasoning_effort_multipliers')
      THEN JSON_REMOVE(
        JSON_SET(j.entry, '$.reasoning_effort_multipliers',
          JSON_OBJECT('max', JSON_EXTRACT(j.entry, '$.max_reasoning_effort_multiplier'))),
        '$.max_reasoning_effort_multiplier')
      ELSE j.entry
    END AS entry
    FROM JSON_TABLE(
      CASE WHEN JSON_TYPE(`groups`.`model_pricing`) = 'ARRAY'
        THEN `groups`.`model_pricing` ELSE JSON_ARRAY() END,
      '$[*]' COLUMNS (ordinal FOR ORDINALITY, entry JSON PATH '$')
    ) AS j
    ORDER BY j.ordinal
  ) AS x
)
WHERE JSON_TYPE(`model_pricing`) = 'ARRAY'
  AND JSON_CONTAINS_PATH(`model_pricing`, 'one', '$[*].max_reasoning_effort_multiplier');

-- Affiliate withdrawal idempotency.
SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'user_affiliate_ledger' AND column_name = 'operation_id');
SET @sql = IF(@col_exists = 0,
  'ALTER TABLE `user_affiliate_ledger` ADD COLUMN `operation_id` VARCHAR(64) NULL', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
SET @idx_exists = (SELECT COUNT(1) FROM information_schema.statistics
  WHERE table_schema = DATABASE() AND table_name = 'user_affiliate_ledger' AND index_name = 'idx_user_affiliate_ledger_operation_id');
SET @sql = IF(@idx_exists = 0,
  'CREATE UNIQUE INDEX `idx_user_affiliate_ledger_operation_id` ON `user_affiliate_ledger` (`operation_id`)', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- Recharge bonus amount snapshot.
SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'payment_orders' AND column_name = 'bonus_amount');
SET @sql = IF(@col_exists = 0,
  'ALTER TABLE `payment_orders` ADD COLUMN `bonus_amount` DECIMAL(20,2) NOT NULL DEFAULT 0 AFTER `coupon_discount_amount`', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- Group localized display text.
SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'groups' AND column_name = 'i18n');
SET @sql = IF(@col_exists = 0,
  'ALTER TABLE `groups` ADD COLUMN `i18n` JSON NOT NULL DEFAULT (JSON_OBJECT())', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- Platform checks are application-owned now; dropping them allows the
-- registered command_code, cline, opencode_go and typesafe platforms.
SET @constraint_exists = (SELECT COUNT(1) FROM information_schema.table_constraints
  WHERE constraint_schema = DATABASE() AND table_name = 'user_platform_quotas'
    AND constraint_name = 'user_platform_quotas_platform_check' AND constraint_type = 'CHECK');
SET @sql = IF(@constraint_exists > 0,
  'ALTER TABLE `user_platform_quotas` DROP CHECK `user_platform_quotas_platform_check`', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @constraint_exists = (SELECT COUNT(1) FROM information_schema.table_constraints
  WHERE constraint_schema = DATABASE() AND table_name = 'composite_model_routes'
    AND constraint_name = 'chk_composite_model_routes_target_platform' AND constraint_type = 'CHECK');
SET @sql = IF(@constraint_exists > 0,
  'ALTER TABLE `composite_model_routes` DROP CHECK `chk_composite_model_routes_target_platform`', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- OpenCode is supported by channel monitoring; TypeSafe remains a gateway
-- platform and therefore is intentionally absent from these provider enums.
ALTER TABLE `channel_monitors`
  MODIFY COLUMN `provider` ENUM('openai','anthropic','gemini','grok','antigravity','kimi','zhipu','deepseek','minimax','opencode_go') NOT NULL;
ALTER TABLE `channel_monitor_request_templates`
  MODIFY COLUMN `provider` ENUM('openai','anthropic','gemini','grok','antigravity','kimi','zhipu','deepseek','minimax','opencode_go') NOT NULL;

-- Gemini cachedContents persistence and mutation leases.
CREATE TABLE IF NOT EXISTS `gemini_cached_contents` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `public_id` VARCHAR(64) NOT NULL,
  `user_id` BIGINT NOT NULL,
  `api_key_id` BIGINT NOT NULL,
  `group_id` BIGINT NOT NULL,
  `account_id` BIGINT NOT NULL,
  `upstream_name` VARCHAR(512) NOT NULL,
  `model` VARCHAR(255) NOT NULL,
  `upstream_model` VARCHAR(512) NOT NULL,
  `display_name` VARCHAR(512) NOT NULL DEFAULT '',
  `total_token_count` BIGINT NOT NULL DEFAULT 0,
  `expire_time` DATETIME(6) NOT NULL,
  `channel_usage` JSON NOT NULL DEFAULT (JSON_OBJECT()),
  `mutation_token` VARCHAR(64) NULL,
  `mutation_lease_until` DATETIME(6) NULL,
  `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `deleted_at` DATETIME(6) NULL,
  PRIMARY KEY (`id`),
  UNIQUE INDEX `idx_gemini_cached_contents_public_id` (`public_id`),
  INDEX `idx_gemini_cached_contents_api_key_active` (`api_key_id`, `id`),
  INDEX `idx_gemini_cached_contents_account` (`account_id`),
  INDEX `idx_gemini_cached_contents_pending_mutation` (`mutation_lease_until`)
) CHARSET utf8mb4 COLLATE utf8mb4_bin;
