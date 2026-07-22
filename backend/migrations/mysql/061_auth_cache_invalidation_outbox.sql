CREATE TABLE IF NOT EXISTS `auth_cache_invalidation_outbox` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `cache_key` CHAR(64) NOT NULL,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `available_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `delivery_stage` SMALLINT NOT NULL DEFAULT 0,
    `attempts` INT NOT NULL DEFAULT 0,
    `last_error` LONGTEXT NULL,
    `claimed_at` DATETIME(6) NULL,
    `claimed_by` VARCHAR(255) NULL,
    PRIMARY KEY (`id`),
    CONSTRAINT `chk_auth_cache_key_hex` CHECK (`cache_key` REGEXP '^[0-9a-f]{64}$'),
    CONSTRAINT `chk_auth_cache_delivery_stage` CHECK (`delivery_stage` IN (0, 1)),
    CONSTRAINT `chk_auth_cache_attempts` CHECK (`attempts` >= 0),
    KEY `idx_auth_cache_invalidation_outbox_available` (`available_at`, `id`),
    KEY `idx_auth_cache_invalidation_outbox_lease` (`claimed_at`),
    KEY `idx_auth_cache_invalidation_outbox_cache_key` (`cache_key`),
    KEY `idx_auth_cache_invalidation_outbox_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

DROP TRIGGER IF EXISTS `trg_api_keys_auth_cache_invalidation_update`;
CREATE TRIGGER `trg_api_keys_auth_cache_invalidation_update` AFTER UPDATE ON `api_keys` FOR EACH ROW
INSERT INTO `auth_cache_invalidation_outbox` (`cache_key`)
SELECT SHA2(OLD.`key`, 256)
WHERE OLD.`key` <> '' AND (
    NOT (OLD.`key` <=> NEW.`key`) OR NOT (OLD.`status` <=> NEW.`status`) OR
    NOT (OLD.`deleted_at` <=> NEW.`deleted_at`) OR NOT (OLD.`user_id` <=> NEW.`user_id`) OR
    NOT (OLD.`group_id` <=> NEW.`group_id`) OR NOT (OLD.`ip_whitelist` <=> NEW.`ip_whitelist`) OR
    NOT (OLD.`ip_blacklist` <=> NEW.`ip_blacklist`) OR NOT (OLD.`expires_at` <=> NEW.`expires_at`)
)
UNION ALL
SELECT SHA2(NEW.`key`, 256)
WHERE NEW.`deleted_at` IS NULL AND NEW.`key` <> '' AND NOT (OLD.`key` <=> NEW.`key`);

DROP TRIGGER IF EXISTS `trg_api_keys_auth_cache_invalidation_delete`;
CREATE TRIGGER `trg_api_keys_auth_cache_invalidation_delete` AFTER DELETE ON `api_keys` FOR EACH ROW
INSERT INTO `auth_cache_invalidation_outbox` (`cache_key`)
SELECT SHA2(OLD.`key`, 256) WHERE OLD.`key` <> '';

DROP TRIGGER IF EXISTS `trg_users_auth_cache_invalidation_update`;
CREATE TRIGGER `trg_users_auth_cache_invalidation_update` AFTER UPDATE ON `users` FOR EACH ROW
INSERT INTO `auth_cache_invalidation_outbox` (`cache_key`)
SELECT SHA2(k.`key`, 256) FROM `api_keys` AS k
WHERE k.`user_id` = OLD.`id` AND k.`deleted_at` IS NULL AND k.`key` <> ''
  AND (NOT (OLD.`status` <=> NEW.`status`) OR NOT (OLD.`role` <=> NEW.`role`) OR NOT (OLD.`deleted_at` <=> NEW.`deleted_at`));

DROP TRIGGER IF EXISTS `trg_users_auth_cache_invalidation_delete`;
CREATE TRIGGER `trg_users_auth_cache_invalidation_delete` AFTER DELETE ON `users` FOR EACH ROW
INSERT INTO `auth_cache_invalidation_outbox` (`cache_key`)
SELECT SHA2(k.`key`, 256) FROM `api_keys` AS k
WHERE k.`user_id` = OLD.`id` AND k.`deleted_at` IS NULL AND k.`key` <> '';

DROP TRIGGER IF EXISTS `trg_groups_auth_cache_invalidation_update`;
CREATE TRIGGER `trg_groups_auth_cache_invalidation_update` AFTER UPDATE ON `groups` FOR EACH ROW
INSERT INTO `auth_cache_invalidation_outbox` (`cache_key`)
SELECT SHA2(k.`key`, 256) FROM `api_keys` AS k
WHERE k.`group_id` = OLD.`id` AND k.`deleted_at` IS NULL AND k.`key` <> ''
  AND (NOT (OLD.`status` <=> NEW.`status`) OR NOT (OLD.`is_exclusive` <=> NEW.`is_exclusive`) OR NOT (OLD.`deleted_at` <=> NEW.`deleted_at`));

DROP TRIGGER IF EXISTS `trg_groups_auth_cache_invalidation_delete`;
CREATE TRIGGER `trg_groups_auth_cache_invalidation_delete` AFTER DELETE ON `groups` FOR EACH ROW
INSERT INTO `auth_cache_invalidation_outbox` (`cache_key`)
SELECT SHA2(k.`key`, 256) FROM `api_keys` AS k
WHERE k.`group_id` = OLD.`id` AND k.`deleted_at` IS NULL AND k.`key` <> '';

DROP TRIGGER IF EXISTS `trg_user_allowed_groups_auth_cache_insert`;
CREATE TRIGGER `trg_user_allowed_groups_auth_cache_insert` AFTER INSERT ON `user_allowed_groups` FOR EACH ROW
INSERT INTO `auth_cache_invalidation_outbox` (`cache_key`)
SELECT SHA2(k.`key`, 256) FROM `api_keys` AS k
JOIN `groups` AS g ON g.`id` = NEW.`group_id` AND g.`is_exclusive` = TRUE
WHERE k.`user_id` = NEW.`user_id` AND k.`group_id` = NEW.`group_id` AND k.`deleted_at` IS NULL AND k.`key` <> '';

DROP TRIGGER IF EXISTS `trg_user_allowed_groups_auth_cache_update`;
CREATE TRIGGER `trg_user_allowed_groups_auth_cache_update` AFTER UPDATE ON `user_allowed_groups` FOR EACH ROW
INSERT INTO `auth_cache_invalidation_outbox` (`cache_key`)
SELECT SHA2(k.`key`, 256) FROM `api_keys` AS k
JOIN `groups` AS g ON g.`id` = OLD.`group_id` AND g.`is_exclusive` = TRUE
WHERE k.`user_id` = OLD.`user_id` AND k.`group_id` = OLD.`group_id` AND k.`deleted_at` IS NULL AND k.`key` <> ''
  AND (NOT (OLD.`user_id` <=> NEW.`user_id`) OR NOT (OLD.`group_id` <=> NEW.`group_id`))
UNION ALL
SELECT SHA2(k.`key`, 256) FROM `api_keys` AS k
JOIN `groups` AS g ON g.`id` = NEW.`group_id` AND g.`is_exclusive` = TRUE
WHERE k.`user_id` = NEW.`user_id` AND k.`group_id` = NEW.`group_id` AND k.`deleted_at` IS NULL AND k.`key` <> ''
  AND (NOT (OLD.`user_id` <=> NEW.`user_id`) OR NOT (OLD.`group_id` <=> NEW.`group_id`));

DROP TRIGGER IF EXISTS `trg_user_allowed_groups_auth_cache_delete`;
CREATE TRIGGER `trg_user_allowed_groups_auth_cache_delete` AFTER DELETE ON `user_allowed_groups` FOR EACH ROW
INSERT INTO `auth_cache_invalidation_outbox` (`cache_key`)
SELECT SHA2(k.`key`, 256) FROM `api_keys` AS k
JOIN `groups` AS g ON g.`id` = OLD.`group_id` AND g.`is_exclusive` = TRUE
WHERE k.`user_id` = OLD.`user_id` AND k.`group_id` = OLD.`group_id` AND k.`deleted_at` IS NULL AND k.`key` <> '';
