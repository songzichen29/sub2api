CREATE TABLE IF NOT EXISTS `account_performance_monitors` (
    `account_id` BIGINT NOT NULL,
    `config` JSON NOT NULL,
    `revision` BIGINT NOT NULL DEFAULT 0,
    `paused` BOOLEAN NOT NULL DEFAULT FALSE,
    `pause_generation` BIGINT NOT NULL DEFAULT 0,
    `paused_at` DATETIME(6) NULL,
    `reason` TEXT NOT NULL,
    `trial_started_at` DATETIME(6) NULL,
    `trial_requests` BIGINT NOT NULL DEFAULT 0,
    `next_trial_at` DATETIME(6) NULL,
    `metrics_since` DATETIME(6) NULL,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`account_id`),
    CONSTRAINT `fk_account_performance_monitors_account_id` FOREIGN KEY (`account_id`) REFERENCES `accounts` (`id`) ON DELETE CASCADE,
    CONSTRAINT `chk_account_performance_monitors_config_object` CHECK (JSON_TYPE(`config`) = 'OBJECT'),
    CONSTRAINT `chk_account_performance_monitors_counters` CHECK (`revision` >= 0 AND `pause_generation` >= 0 AND `trial_requests` >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `account_performance_events` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `account_id` BIGINT NOT NULL,
    `action` VARCHAR(16) NOT NULL,
    `actor_id` BIGINT NULL,
    `revision` BIGINT NOT NULL,
    `pause_generation` BIGINT NOT NULL,
    `evidence` JSON NOT NULL,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    KEY `idx_account_performance_events_account_id_id` (`account_id`, `id`),
    CONSTRAINT `fk_account_performance_events_account_id` FOREIGN KEY (`account_id`) REFERENCES `accounts` (`id`) ON DELETE CASCADE,
    CONSTRAINT `fk_account_performance_events_actor_id` FOREIGN KEY (`actor_id`) REFERENCES `users` (`id`) ON DELETE SET NULL,
    CONSTRAINT `chk_account_performance_events_action` CHECK (`action` IN ('configure', 'pause', 'trial', 'recover', 'release', 'retire')),
    CONSTRAINT `chk_account_performance_events_counters` CHECK (`revision` >= 0 AND `pause_generation` >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
