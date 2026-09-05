CREATE TABLE IF NOT EXISTS `channel_monitor_v2_config` (
    `id` SMALLINT NOT NULL,
    `version` INT NOT NULL DEFAULT 1,
    `enabled` BOOLEAN NOT NULL DEFAULT TRUE,
    `refresh_interval_seconds` INT NOT NULL DEFAULT 300,
    `platforms` JSON NOT NULL,
    `group_ids` JSON NOT NULL,
    `ignored_error_categories` JSON NOT NULL,
    `health_thresholds` JSON NOT NULL,
    `updated_by` BIGINT NULL,
    `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    CONSTRAINT `chk_channel_monitor_v2_config_id` CHECK (`id` = 1),
    CONSTRAINT `chk_channel_monitor_v2_refresh` CHECK (`refresh_interval_seconds` IN (60, 300))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

INSERT IGNORE INTO `channel_monitor_v2_config` (
    `id`, `version`, `enabled`, `refresh_interval_seconds`, `platforms`, `group_ids`,
    `ignored_error_categories`, `health_thresholds`, `updated_by`, `updated_at`
) VALUES (
    1,
    2,
    TRUE,
    300,
    JSON_ARRAY(
        JSON_OBJECT('platform', 'anthropic', 'enabled', TRUE, 'models', JSON_ARRAY(
            'claude-opus-5', 'claude-opus-4-8', 'claude-opus-4-7', 'claude-opus-4-6',
            'claude-opus-4.6', 'claude-opus-4-5', 'claude-opus-4-1', 'claude-opus-4',
            'claude-fable-5', 'claude-sonnet-5', 'claude-sonnet-4-6', 'claude-sonnet-4.6',
            'claude-sonnet-4-5', 'claude-sonnet-4.5', 'claude-sonnet-4',
            'claude-haiku-4-5', 'claude-haiku-4.5', 'claude-3-7-sonnet',
            'claude-3-5-sonnet', 'claude-3-5-haiku'
        )),
        JSON_OBJECT('platform', 'openai', 'enabled', TRUE, 'models', JSON_ARRAY(
            'gpt-5.6-sol', 'gpt-5.6-terra', 'gpt-5.6-luna', 'gpt-5.6', 'gpt-5.5',
            'gpt-5.4', 'gpt-5.4-mini', 'gpt-5.3-codex-spark', 'gpt-5.2', 'gpt-5.2-pro',
            'gpt-5', 'gpt-4.1', 'gpt-4.1-mini', 'gpt-4.1-nano', 'gpt-4o',
            'gpt-4o-mini', 'gpt-4-turbo', 'gpt-4', 'o3', 'o3-mini', 'o4-mini',
            'codex-auto-review', 'gpt-image-2', 'gpt-image-1'
        )),
        JSON_OBJECT('platform', 'gemini', 'enabled', TRUE, 'models', JSON_ARRAY(
            'gemini-2.5-pro', 'gemini-2.5-flash', 'gemini-2.5-flash-lite',
            'gemini-2.5-flash-image', 'gemini-2.0-flash', 'gemini-2.0-flash-lite',
            'gemini-3-flash', 'gemini-3-pro-image', 'gemini-3.1-pro-high',
            'gemini-3.1-pro-low', 'gemini-3.1-flash-lite', 'gemini-3.5-flash',
            'gemini-3.5-flash-lite'
        )),
        JSON_OBJECT('platform', 'kiro', 'enabled', TRUE, 'models', JSON_ARRAY(
            'claude-opus-4-8', 'claude-opus-4-6', 'claude-opus-4-5',
            'claude-sonnet-4-5', 'claude-sonnet-4-6', 'claude-haiku-4-5'
        )),
        JSON_OBJECT('platform', 'antigravity', 'enabled', TRUE, 'models', JSON_ARRAY(
            'claude-opus-4-6', 'claude-sonnet-4-5', 'claude-sonnet-4-6',
            'gemini-2.5-pro', 'gemini-2.5-flash'
        ))
    ),
    JSON_ARRAY(),
    JSON_ARRAY('authentication', 'client_cancelled', 'content_policy', 'context_limit',
        'group_access', 'model_unsupported', 'not_found', 'quota_or_balance'),
    JSON_OBJECT(
        'minimum_sample', 50,
        'warning_error_rate', 0.05,
        'critical_error_rate', 0.20,
        'target_ttft_ms', 3000,
        'warning_ttft_ms', 8000,
        'critical_ttft_ms', 20000,
        'warning_cache_rate', 0,
        'critical_cache_rate', 0,
        'error_weight', 0.60,
        'ttft_weight', 0.20,
        'cache_weight', 0.20
    ),
    NULL,
    NOW(6)
);

CREATE TABLE IF NOT EXISTS `channel_monitor_v2_metrics_1m` (
    `bucket_start` DATETIME(6) NOT NULL,
    `platform` VARCHAR(50) NOT NULL,
    `group_id` BIGINT NOT NULL DEFAULT 0,
    `model` VARCHAR(200) NOT NULL,
    `success_requests` BIGINT NOT NULL DEFAULT 0,
    `error_requests` BIGINT NOT NULL DEFAULT 0,
    `upstream_affected_requests` BIGINT NOT NULL DEFAULT 0,
    `upstream_attempt_count` BIGINT NOT NULL DEFAULT 0,
    `input_tokens` BIGINT NOT NULL DEFAULT 0,
    `output_tokens` BIGINT NOT NULL DEFAULT 0,
    `cache_creation_tokens` BIGINT NOT NULL DEFAULT 0,
    `cache_read_tokens` BIGINT NOT NULL DEFAULT 0,
    `ttft_sum_ms` BIGINT NOT NULL DEFAULT 0,
    `ttft_count` BIGINT NOT NULL DEFAULT 0,
    `duration_sum_ms` BIGINT NOT NULL DEFAULT 0,
    `duration_count` BIGINT NOT NULL DEFAULT 0,
    `computed_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`bucket_start`, `platform`, `group_id`, `model`),
    KEY `idx_channel_monitor_v2_metrics_platform_time` (`platform`, `bucket_start` DESC),
    KEY `idx_channel_monitor_v2_metrics_group_time` (`group_id`, `bucket_start` DESC),
    KEY `idx_channel_monitor_v2_metrics_model_time` (`model`, `bucket_start` DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `channel_monitor_v2_user_metrics_1m` (
    `bucket_start` DATETIME(6) NOT NULL,
    `platform` VARCHAR(50) NOT NULL,
    `group_id` BIGINT NOT NULL DEFAULT 0,
    `model` VARCHAR(200) NOT NULL,
    `user_id` BIGINT NOT NULL,
    `success_requests` BIGINT NOT NULL DEFAULT 0,
    `error_requests` BIGINT NOT NULL DEFAULT 0,
    `input_tokens` BIGINT NOT NULL DEFAULT 0,
    `output_tokens` BIGINT NOT NULL DEFAULT 0,
    `cache_creation_tokens` BIGINT NOT NULL DEFAULT 0,
    `cache_read_tokens` BIGINT NOT NULL DEFAULT 0,
    `ttft_sum_ms` BIGINT NOT NULL DEFAULT 0,
    `ttft_count` BIGINT NOT NULL DEFAULT 0,
    `duration_sum_ms` BIGINT NOT NULL DEFAULT 0,
    `duration_count` BIGINT NOT NULL DEFAULT 0,
    `computed_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`bucket_start`, `platform`, `group_id`, `model`, `user_id`),
    KEY `idx_channel_monitor_v2_user_metrics_user_time` (`user_id`, `bucket_start` DESC),
    KEY `idx_channel_monitor_v2_user_metrics_time` (`bucket_start` DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `channel_monitor_v2_error_metrics_1m` (
    `bucket_start` DATETIME(6) NOT NULL,
    `platform` VARCHAR(50) NOT NULL,
    `group_id` BIGINT NOT NULL DEFAULT 0,
    `model` VARCHAR(200) NOT NULL,
    `error_category` VARCHAR(64) NOT NULL,
    `taxonomy_version` SMALLINT NOT NULL,
    `error_requests` BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (`bucket_start`, `platform`, `group_id`, `model`, `error_category`, `taxonomy_version`),
    KEY `idx_channel_monitor_v2_errors_time` (`bucket_start` DESC),
    KEY `idx_channel_monitor_v2_errors_category_time` (`error_category`, `bucket_start` DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `channel_monitor_v2_latency_histograms_1m` (
    `bucket_start` DATETIME(6) NOT NULL,
    `platform` VARCHAR(50) NOT NULL,
    `group_id` BIGINT NOT NULL DEFAULT 0,
    `model` VARCHAR(200) NOT NULL,
    `user_id` BIGINT NOT NULL DEFAULT 0,
    `metric` VARCHAR(20) NOT NULL,
    `upper_bound_ms` INT NOT NULL,
    `sample_count` BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (`bucket_start`, `platform`, `group_id`, `model`, `user_id`, `metric`, `upper_bound_ms`),
    KEY `idx_channel_monitor_v2_histograms_time` (`bucket_start` DESC, `metric`),
    CONSTRAINT `chk_channel_monitor_v2_histogram_metric` CHECK (`metric` IN ('ttft', 'duration'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `channel_monitor_v2_watermarks` (
    `id` SMALLINT NOT NULL,
    `usage_coverage_start` DATETIME(6) NULL,
    `error_coverage_start` DATETIME(6) NULL,
    `data_through` DATETIME(6) NULL,
    `last_successful_at` DATETIME(6) NULL,
    `backfill_cursor` DATETIME(6) NULL,
    `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    CONSTRAINT `chk_channel_monitor_v2_watermarks_id` CHECK (`id` = 1)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

INSERT IGNORE INTO `channel_monitor_v2_watermarks` (`id`) VALUES (1);

CREATE TABLE IF NOT EXISTS `channel_monitor_v2_metrics_rollup` LIKE `channel_monitor_v2_metrics_1m`;
SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'channel_monitor_v2_metrics_rollup' AND column_name = 'bucket_seconds');
SET @sql = IF(@col_exists = 0, 'ALTER TABLE `channel_monitor_v2_metrics_rollup` ADD COLUMN `bucket_seconds` INT NOT NULL AFTER `bucket_start`', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
ALTER TABLE `channel_monitor_v2_metrics_rollup`
    DROP PRIMARY KEY,
    ADD PRIMARY KEY (`bucket_seconds`, `bucket_start`, `platform`, `group_id`, `model`);

CREATE TABLE IF NOT EXISTS `channel_monitor_v2_user_metrics_rollup` LIKE `channel_monitor_v2_user_metrics_1m`;
SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'channel_monitor_v2_user_metrics_rollup' AND column_name = 'bucket_seconds');
SET @sql = IF(@col_exists = 0, 'ALTER TABLE `channel_monitor_v2_user_metrics_rollup` ADD COLUMN `bucket_seconds` INT NOT NULL AFTER `bucket_start`', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
ALTER TABLE `channel_monitor_v2_user_metrics_rollup`
    DROP PRIMARY KEY,
    ADD PRIMARY KEY (`bucket_seconds`, `bucket_start`, `platform`, `group_id`, `model`, `user_id`);

CREATE TABLE IF NOT EXISTS `channel_monitor_v2_error_metrics_rollup` LIKE `channel_monitor_v2_error_metrics_1m`;
SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'channel_monitor_v2_error_metrics_rollup' AND column_name = 'bucket_seconds');
SET @sql = IF(@col_exists = 0, 'ALTER TABLE `channel_monitor_v2_error_metrics_rollup` ADD COLUMN `bucket_seconds` INT NOT NULL AFTER `bucket_start`', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
ALTER TABLE `channel_monitor_v2_error_metrics_rollup`
    DROP PRIMARY KEY,
    ADD PRIMARY KEY (`bucket_seconds`, `bucket_start`, `platform`, `group_id`, `model`, `error_category`, `taxonomy_version`);

CREATE TABLE IF NOT EXISTS `channel_monitor_v2_latency_histograms_rollup` LIKE `channel_monitor_v2_latency_histograms_1m`;
SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'channel_monitor_v2_latency_histograms_rollup' AND column_name = 'bucket_seconds');
SET @sql = IF(@col_exists = 0, 'ALTER TABLE `channel_monitor_v2_latency_histograms_rollup` ADD COLUMN `bucket_seconds` INT NOT NULL AFTER `bucket_start`', 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
ALTER TABLE `channel_monitor_v2_latency_histograms_rollup`
    DROP PRIMARY KEY,
    ADD PRIMARY KEY (`bucket_seconds`, `bucket_start`, `platform`, `group_id`, `model`, `user_id`, `metric`, `upper_bound_ms`);

INSERT IGNORE INTO `settings` (`key`, `value`) VALUES ('channel_monitor_mode', 'v1');
INSERT IGNORE INTO `settings` (`key`, `value`) VALUES ('channel_monitor_hide_throughput', 'true');
