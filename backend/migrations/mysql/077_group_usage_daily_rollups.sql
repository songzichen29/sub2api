CREATE TABLE IF NOT EXISTS `usage_group_daily_rollups` (
    `bucket_date` DATE NOT NULL,
    `group_id` BIGINT NOT NULL,
    `actual_cost` DECIMAL(20, 10) NOT NULL DEFAULT 0,
    `computed_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`bucket_date`, `group_id`),
    KEY `idx_usage_group_daily_rollups_group_date` (`group_id`, `bucket_date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `usage_group_rollup_state` (
    `id` SMALLINT NOT NULL,
    `closed_before` DATE NOT NULL DEFAULT '1970-01-01',
    `retained_from` DATETIME(6) NOT NULL DEFAULT '1970-01-01 00:00:00.000000',
    `timezone_name` VARCHAR(255) NOT NULL DEFAULT 'Asia/Shanghai',
    `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    CONSTRAINT `chk_usage_group_rollup_state_id` CHECK (`id` = 1)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

INSERT IGNORE INTO `usage_group_rollup_state` (
    `id`, `closed_before`, `retained_from`, `timezone_name`, `updated_at`
) VALUES (1, '1970-01-01', '1970-01-01 00:00:00.000000', 'Asia/Shanghai', NOW(6));

DROP TRIGGER IF EXISTS `trg_usage_logs_group_rollup_invalidate_insert`;
CREATE TRIGGER `trg_usage_logs_group_rollup_invalidate_insert` AFTER INSERT ON `usage_logs` FOR EACH ROW
UPDATE `usage_group_rollup_state`
SET `closed_before` = LEAST(
        `closed_before`,
        COALESCE(
            DATE(CONVERT_TZ(NEW.`created_at`, '+00:00', `timezone_name`)),
            DATE_SUB(DATE(NEW.`created_at`), INTERVAL 1 DAY)
        )
    ),
    `updated_at` = NOW(6)
WHERE `id` = 1
  AND NEW.`group_id` IS NOT NULL
  AND `closed_before` > COALESCE(
      DATE(CONVERT_TZ(NEW.`created_at`, '+00:00', `timezone_name`)),
      DATE_SUB(DATE(NEW.`created_at`), INTERVAL 1 DAY)
  );

DROP TRIGGER IF EXISTS `trg_usage_logs_group_rollup_invalidate_delete`;
CREATE TRIGGER `trg_usage_logs_group_rollup_invalidate_delete` AFTER DELETE ON `usage_logs` FOR EACH ROW
UPDATE `usage_group_rollup_state`
SET `closed_before` = LEAST(
        `closed_before`,
        COALESCE(
            DATE(CONVERT_TZ(OLD.`created_at`, '+00:00', `timezone_name`)),
            DATE_SUB(DATE(OLD.`created_at`), INTERVAL 1 DAY)
        )
    ),
    `updated_at` = NOW(6)
WHERE `id` = 1
  AND OLD.`group_id` IS NOT NULL
  AND `closed_before` > COALESCE(
      DATE(CONVERT_TZ(OLD.`created_at`, '+00:00', `timezone_name`)),
      DATE_SUB(DATE(OLD.`created_at`), INTERVAL 1 DAY)
  );

DROP TRIGGER IF EXISTS `trg_usage_logs_group_rollup_invalidate_update`;
CREATE TRIGGER `trg_usage_logs_group_rollup_invalidate_update` AFTER UPDATE ON `usage_logs` FOR EACH ROW
UPDATE `usage_group_rollup_state`
SET `closed_before` = LEAST(
        `closed_before`,
        CASE
            WHEN OLD.`group_id` IS NULL THEN COALESCE(
                DATE(CONVERT_TZ(NEW.`created_at`, '+00:00', `timezone_name`)),
                DATE_SUB(DATE(NEW.`created_at`), INTERVAL 1 DAY)
            )
            WHEN NEW.`group_id` IS NULL THEN COALESCE(
                DATE(CONVERT_TZ(OLD.`created_at`, '+00:00', `timezone_name`)),
                DATE_SUB(DATE(OLD.`created_at`), INTERVAL 1 DAY)
            )
            ELSE LEAST(
                COALESCE(
                    DATE(CONVERT_TZ(OLD.`created_at`, '+00:00', `timezone_name`)),
                    DATE_SUB(DATE(OLD.`created_at`), INTERVAL 1 DAY)
                ),
                COALESCE(
                    DATE(CONVERT_TZ(NEW.`created_at`, '+00:00', `timezone_name`)),
                    DATE_SUB(DATE(NEW.`created_at`), INTERVAL 1 DAY)
                )
            )
        END
    ),
    `updated_at` = NOW(6)
WHERE `id` = 1
  AND (OLD.`group_id` IS NOT NULL OR NEW.`group_id` IS NOT NULL)
  AND (
      NOT (OLD.`created_at` <=> NEW.`created_at`)
      OR NOT (OLD.`group_id` <=> NEW.`group_id`)
      OR NOT (OLD.`actual_cost` <=> NEW.`actual_cost`)
  );
