SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'groups' AND column_name = 'force_openai_ultrafast');
SET @sql = IF(@col_exists = 0,
  'ALTER TABLE `groups` ADD COLUMN `force_openai_ultrafast` BOOLEAN NOT NULL DEFAULT FALSE', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'usage_logs' AND column_name = 'cache_creation_30m_tokens');
SET @sql = IF(@col_exists = 0,
  'ALTER TABLE `usage_logs` ADD COLUMN `cache_creation_30m_tokens` INT NOT NULL DEFAULT 0', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'usage_logs' AND column_name = 'cache_creation_12h_tokens');
SET @sql = IF(@col_exists = 0,
  'ALTER TABLE `usage_logs` ADD COLUMN `cache_creation_12h_tokens` INT NOT NULL DEFAULT 0', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'channel_model_pricing' AND column_name = 'cache_write_12h_price');
SET @sql = IF(@col_exists = 0,
  'ALTER TABLE `channel_model_pricing` ADD COLUMN `cache_write_12h_price` DECIMAL(20,12) NULL', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'channel_pricing_intervals' AND column_name = 'cache_write_12h_price');
SET @sql = IF(@col_exists = 0,
  'ALTER TABLE `channel_pricing_intervals` ADD COLUMN `cache_write_12h_price` DECIMAL(20,12) NULL', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'channel_account_stats_model_pricing' AND column_name = 'cache_write_12h_price');
SET @sql = IF(@col_exists = 0,
  'ALTER TABLE `channel_account_stats_model_pricing` ADD COLUMN `cache_write_12h_price` DECIMAL(20,12) NULL', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'channel_account_stats_pricing_intervals' AND column_name = 'cache_write_12h_price');
SET @sql = IF(@col_exists = 0,
  'ALTER TABLE `channel_account_stats_pricing_intervals` ADD COLUMN `cache_write_12h_price` DECIMAL(20,12) NULL', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'channel_model_pricing' AND column_name = 'ultrafast_multiplier');
SET @sql = IF(@col_exists = 0,
  'ALTER TABLE `channel_model_pricing` ADD COLUMN `ultrafast_multiplier` DECIMAL(12,6) NULL', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'usage_group_rollup_state' AND column_name = 'expired_before');
SET @sql = IF(@col_exists = 0,
  'ALTER TABLE `usage_group_rollup_state` ADD COLUMN `expired_before` DATETIME(6) NOT NULL DEFAULT ''1970-01-01 00:00:00.000000''', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'usage_group_rollup_state' AND column_name = 'building_until');
SET @sql = IF(@col_exists = 0,
  'ALTER TABLE `usage_group_rollup_state` ADD COLUMN `building_until` DATE NULL', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'usage_group_rollup_state' AND column_name = 'build_dirty_from');
SET @sql = IF(@col_exists = 0,
  'ALTER TABLE `usage_group_rollup_state` ADD COLUMN `build_dirty_from` DATE NULL', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 按 Key 查询费用时使用覆盖索引。
SET @index_exists = (SELECT COUNT(1) FROM information_schema.statistics
  WHERE table_schema = DATABASE() AND table_name = 'usage_logs' AND index_name = 'idx_usage_logs_api_key_created_cost');
SET @sql = IF(@index_exists = 0,
  'CREATE INDEX `idx_usage_logs_api_key_created_cost` ON `usage_logs` (`api_key_id`, `created_at`, `actual_cost`)', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @constraint_exists = (SELECT COUNT(1) FROM information_schema.table_constraints
  WHERE constraint_schema = DATABASE() AND table_name = 'channel_model_pricing'
    AND constraint_name = 'channel_model_pricing_ultrafast_multiplier_positive' AND constraint_type = 'CHECK');
SET @sql = IF(@constraint_exists = 0,
  'ALTER TABLE `channel_model_pricing` ADD CONSTRAINT `channel_model_pricing_ultrafast_multiplier_positive` CHECK (`ultrafast_multiplier` IS NULL OR `ultrafast_multiplier` > 0)', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 计算期间记录脏日期，登记和发布阶段仅短暂持有状态行锁。
DROP TRIGGER IF EXISTS `trg_usage_logs_group_rollup_invalidate_insert`;
CREATE TRIGGER `trg_usage_logs_group_rollup_invalidate_insert` AFTER INSERT ON `usage_logs` FOR EACH ROW
UPDATE `usage_group_rollup_state`
SET `build_dirty_from` = CASE
        WHEN `building_until` > (COALESCE(DATE(CONVERT_TZ(NEW.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(NEW.`created_at`), INTERVAL 1 DAY)))
        THEN LEAST(COALESCE(`build_dirty_from`, (COALESCE(DATE(CONVERT_TZ(NEW.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(NEW.`created_at`), INTERVAL 1 DAY)))), (COALESCE(DATE(CONVERT_TZ(NEW.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(NEW.`created_at`), INTERVAL 1 DAY))))
        ELSE `build_dirty_from`
    END,
    `closed_before` = LEAST(`closed_before`, (COALESCE(DATE(CONVERT_TZ(NEW.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(NEW.`created_at`), INTERVAL 1 DAY)))),
    `updated_at` = NOW(6)
WHERE `id` = 1
  AND NEW.`group_id` IS NOT NULL AND NEW.`created_at` >= `expired_before`
  AND (`closed_before` > (COALESCE(DATE(CONVERT_TZ(NEW.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(NEW.`created_at`), INTERVAL 1 DAY)))
       OR (`building_until` > (COALESCE(DATE(CONVERT_TZ(NEW.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(NEW.`created_at`), INTERVAL 1 DAY)))
           AND (`build_dirty_from` IS NULL OR `build_dirty_from` > (COALESCE(DATE(CONVERT_TZ(NEW.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(NEW.`created_at`), INTERVAL 1 DAY))))));

-- 计算期间记录脏日期，登记和发布阶段仅短暂持有状态行锁。
DROP TRIGGER IF EXISTS `trg_usage_logs_group_rollup_invalidate_delete`;
CREATE TRIGGER `trg_usage_logs_group_rollup_invalidate_delete` AFTER DELETE ON `usage_logs` FOR EACH ROW
UPDATE `usage_group_rollup_state`
SET `build_dirty_from` = CASE
        WHEN `building_until` > (COALESCE(DATE(CONVERT_TZ(OLD.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(OLD.`created_at`), INTERVAL 1 DAY)))
        THEN LEAST(COALESCE(`build_dirty_from`, (COALESCE(DATE(CONVERT_TZ(OLD.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(OLD.`created_at`), INTERVAL 1 DAY)))), (COALESCE(DATE(CONVERT_TZ(OLD.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(OLD.`created_at`), INTERVAL 1 DAY))))
        ELSE `build_dirty_from`
    END,
    `closed_before` = LEAST(`closed_before`, (COALESCE(DATE(CONVERT_TZ(OLD.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(OLD.`created_at`), INTERVAL 1 DAY)))),
    `updated_at` = NOW(6)
WHERE `id` = 1
  AND OLD.`group_id` IS NOT NULL AND OLD.`created_at` >= `expired_before`
  AND (`closed_before` > (COALESCE(DATE(CONVERT_TZ(OLD.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(OLD.`created_at`), INTERVAL 1 DAY)))
       OR (`building_until` > (COALESCE(DATE(CONVERT_TZ(OLD.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(OLD.`created_at`), INTERVAL 1 DAY)))
           AND (`build_dirty_from` IS NULL OR `build_dirty_from` > (COALESCE(DATE(CONVERT_TZ(OLD.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(OLD.`created_at`), INTERVAL 1 DAY))))));

-- 计算期间记录脏日期，登记和发布阶段仅短暂持有状态行锁。
DROP TRIGGER IF EXISTS `trg_usage_logs_group_rollup_invalidate_update`;
CREATE TRIGGER `trg_usage_logs_group_rollup_invalidate_update` AFTER UPDATE ON `usage_logs` FOR EACH ROW
UPDATE `usage_group_rollup_state`
SET `build_dirty_from` = CASE
        WHEN `building_until` > (CASE
        WHEN OLD.`group_id` IS NULL OR OLD.`created_at` < `expired_before` THEN COALESCE(DATE(CONVERT_TZ(NEW.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(NEW.`created_at`), INTERVAL 1 DAY))
        WHEN NEW.`group_id` IS NULL OR NEW.`created_at` < `expired_before` THEN COALESCE(DATE(CONVERT_TZ(OLD.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(OLD.`created_at`), INTERVAL 1 DAY))
        ELSE LEAST(COALESCE(DATE(CONVERT_TZ(OLD.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(OLD.`created_at`), INTERVAL 1 DAY)), COALESCE(DATE(CONVERT_TZ(NEW.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(NEW.`created_at`), INTERVAL 1 DAY)))
    END)
        THEN LEAST(COALESCE(`build_dirty_from`, (CASE
        WHEN OLD.`group_id` IS NULL OR OLD.`created_at` < `expired_before` THEN COALESCE(DATE(CONVERT_TZ(NEW.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(NEW.`created_at`), INTERVAL 1 DAY))
        WHEN NEW.`group_id` IS NULL OR NEW.`created_at` < `expired_before` THEN COALESCE(DATE(CONVERT_TZ(OLD.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(OLD.`created_at`), INTERVAL 1 DAY))
        ELSE LEAST(COALESCE(DATE(CONVERT_TZ(OLD.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(OLD.`created_at`), INTERVAL 1 DAY)), COALESCE(DATE(CONVERT_TZ(NEW.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(NEW.`created_at`), INTERVAL 1 DAY)))
    END)), (CASE
        WHEN OLD.`group_id` IS NULL OR OLD.`created_at` < `expired_before` THEN COALESCE(DATE(CONVERT_TZ(NEW.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(NEW.`created_at`), INTERVAL 1 DAY))
        WHEN NEW.`group_id` IS NULL OR NEW.`created_at` < `expired_before` THEN COALESCE(DATE(CONVERT_TZ(OLD.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(OLD.`created_at`), INTERVAL 1 DAY))
        ELSE LEAST(COALESCE(DATE(CONVERT_TZ(OLD.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(OLD.`created_at`), INTERVAL 1 DAY)), COALESCE(DATE(CONVERT_TZ(NEW.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(NEW.`created_at`), INTERVAL 1 DAY)))
    END))
        ELSE `build_dirty_from`
    END,
    `closed_before` = LEAST(`closed_before`, (CASE
        WHEN OLD.`group_id` IS NULL OR OLD.`created_at` < `expired_before` THEN COALESCE(DATE(CONVERT_TZ(NEW.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(NEW.`created_at`), INTERVAL 1 DAY))
        WHEN NEW.`group_id` IS NULL OR NEW.`created_at` < `expired_before` THEN COALESCE(DATE(CONVERT_TZ(OLD.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(OLD.`created_at`), INTERVAL 1 DAY))
        ELSE LEAST(COALESCE(DATE(CONVERT_TZ(OLD.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(OLD.`created_at`), INTERVAL 1 DAY)), COALESCE(DATE(CONVERT_TZ(NEW.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(NEW.`created_at`), INTERVAL 1 DAY)))
    END)),
    `updated_at` = NOW(6)
WHERE `id` = 1
  AND ((OLD.`group_id` IS NOT NULL AND OLD.`created_at` >= `expired_before`)
       OR (NEW.`group_id` IS NOT NULL AND NEW.`created_at` >= `expired_before`))
  AND (NOT (OLD.`created_at` <=> NEW.`created_at`)
       OR NOT (OLD.`group_id` <=> NEW.`group_id`)
       OR NOT (OLD.`actual_cost` <=> NEW.`actual_cost`))
  AND (`closed_before` > (CASE
        WHEN OLD.`group_id` IS NULL OR OLD.`created_at` < `expired_before` THEN COALESCE(DATE(CONVERT_TZ(NEW.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(NEW.`created_at`), INTERVAL 1 DAY))
        WHEN NEW.`group_id` IS NULL OR NEW.`created_at` < `expired_before` THEN COALESCE(DATE(CONVERT_TZ(OLD.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(OLD.`created_at`), INTERVAL 1 DAY))
        ELSE LEAST(COALESCE(DATE(CONVERT_TZ(OLD.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(OLD.`created_at`), INTERVAL 1 DAY)), COALESCE(DATE(CONVERT_TZ(NEW.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(NEW.`created_at`), INTERVAL 1 DAY)))
    END)
       OR (`building_until` > (CASE
        WHEN OLD.`group_id` IS NULL OR OLD.`created_at` < `expired_before` THEN COALESCE(DATE(CONVERT_TZ(NEW.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(NEW.`created_at`), INTERVAL 1 DAY))
        WHEN NEW.`group_id` IS NULL OR NEW.`created_at` < `expired_before` THEN COALESCE(DATE(CONVERT_TZ(OLD.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(OLD.`created_at`), INTERVAL 1 DAY))
        ELSE LEAST(COALESCE(DATE(CONVERT_TZ(OLD.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(OLD.`created_at`), INTERVAL 1 DAY)), COALESCE(DATE(CONVERT_TZ(NEW.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(NEW.`created_at`), INTERVAL 1 DAY)))
    END)
           AND (`build_dirty_from` IS NULL OR `build_dirty_from` > (CASE
        WHEN OLD.`group_id` IS NULL OR OLD.`created_at` < `expired_before` THEN COALESCE(DATE(CONVERT_TZ(NEW.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(NEW.`created_at`), INTERVAL 1 DAY))
        WHEN NEW.`group_id` IS NULL OR NEW.`created_at` < `expired_before` THEN COALESCE(DATE(CONVERT_TZ(OLD.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(OLD.`created_at`), INTERVAL 1 DAY))
        ELSE LEAST(COALESCE(DATE(CONVERT_TZ(OLD.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(OLD.`created_at`), INTERVAL 1 DAY)), COALESCE(DATE(CONVERT_TZ(NEW.`created_at`, '+00:00', `timezone_name`)), DATE_SUB(DATE(NEW.`created_at`), INTERVAL 1 DAY)))
    END))));
