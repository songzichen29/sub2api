-- MySQL runtime migration for batch image jobs and frozen balance.

CREATE TABLE IF NOT EXISTS `batch_image_jobs` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `batch_id` VARCHAR(64) NOT NULL,
    `user_id` BIGINT NOT NULL,
    `api_key_id` BIGINT NULL,
    `account_id` BIGINT NULL,
    `provider` VARCHAR(32) NOT NULL,
    `model` VARCHAR(128) NOT NULL,
    `task_name` VARCHAR(255) NOT NULL DEFAULT '',
    `parent_batch_id` VARCHAR(64) NULL,
    `status` VARCHAR(32) NOT NULL DEFAULT 'created',
    `provider_job_name` VARCHAR(512) NULL,
    `provider_input_ref` VARCHAR(1024) NULL,
    `provider_output_ref` VARCHAR(1024) NULL,
    `gcs_input_uri` VARCHAR(1024) NULL,
    `gcs_output_uri` VARCHAR(1024) NULL,
    `item_count` INT NOT NULL,
    `success_count` INT NOT NULL DEFAULT 0,
    `fail_count` INT NOT NULL DEFAULT 0,
    `cancelled_count` INT NOT NULL DEFAULT 0,
    `estimated_cost` DECIMAL(20,10) NOT NULL DEFAULT 0,
    `hold_amount` DECIMAL(20,10) NULL,
    `actual_cost` DECIMAL(20,10) NULL,
    `base_unit_price` DECIMAL(20,10) NOT NULL DEFAULT 0,
    `group_rate_multiplier` DECIMAL(10,4) NOT NULL DEFAULT 1.0,
    `account_rate_multiplier` DECIMAL(10,4) NOT NULL DEFAULT 1.0,
    `batch_discount_multiplier` DECIMAL(10,4) NOT NULL DEFAULT 0.5,
    `hold_multiplier` DECIMAL(10,4) NOT NULL DEFAULT 0.6,
    `billable_unit_price` DECIMAL(20,10) NOT NULL DEFAULT 0,
    `hold_unit_price` DECIMAL(20,10) NOT NULL DEFAULT 0,
    `pricing_snapshot_version` INT NOT NULL DEFAULT 0,
    `currency` VARCHAR(16) NOT NULL DEFAULT 'USD',
    `hold_id` VARCHAR(128) NULL,
    `idempotency_key` VARCHAR(255) NULL,
    `request_hash` VARCHAR(128) NULL,
    `manifest_hash` VARCHAR(128) NULL,
    `retry_count` INT NOT NULL DEFAULT 0,
    `version` INT NOT NULL DEFAULT 0,
    `output_expires_at` DATETIME(6) NULL,
    `input_deleted_at` DATETIME(6) NULL,
    `output_deleted_at` DATETIME(6) NULL,
    `downloaded_at` DATETIME(6) NULL,
    `user_deleted_at` DATETIME(6) NULL,
    `last_error_code` VARCHAR(128) NULL,
    `last_error_message` LONGTEXT NULL,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    `submitted_at` DATETIME(6) NULL,
    `started_at` DATETIME(6) NULL,
    `finished_at` DATETIME(6) NULL,
    `settled_at` DATETIME(6) NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `batch_image_jobs_batch_id_key` (`batch_id`),
    UNIQUE KEY `batch_image_jobs_manifest_hash_uq` (`manifest_hash`),
    KEY `batch_image_jobs_user_created_at_idx` (`user_id`, `created_at`),
    KEY `batch_image_jobs_status_idx` (`status`),
    KEY `batch_image_jobs_provider_status_idx` (`provider`, `status`),
    KEY `batch_image_jobs_idempotency_key_idx` (`idempotency_key`),
    KEY `batch_image_jobs_output_expires_at_idx` (`output_expires_at`),
    KEY `batch_image_jobs_downloaded_at_idx` (`downloaded_at`),
    KEY `batch_image_jobs_user_deleted_at_idx` (`user_deleted_at`),
    KEY `batch_image_jobs_task_name_idx` (`task_name`),
    KEY `batch_image_jobs_parent_batch_id_idx` (`parent_batch_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `batch_image_items` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `job_id` VARCHAR(64) NOT NULL,
    `custom_id` VARCHAR(255) NOT NULL,
    `status` VARCHAR(32) NOT NULL,
    `request_hash` VARCHAR(128) NULL,
    `prompt_preview` LONGTEXT NULL,
    `provider_source_object` VARCHAR(1024) NULL,
    `source_line_number` INT NULL,
    `source_byte_offset` BIGINT NULL,
    `source_byte_length` BIGINT NULL,
    `mime_type` VARCHAR(128) NULL,
    `file_extension` VARCHAR(32) NULL,
    `image_count` INT NOT NULL DEFAULT 0,
    `error_code` VARCHAR(128) NULL,
    `error_message` LONGTEXT NULL,
    `billed_amount` DECIMAL(20,10) NULL,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `indexed_at` DATETIME(6) NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `batch_image_items_job_custom_uq` (`job_id`, `custom_id`),
    KEY `batch_image_items_job_status_idx` (`job_id`, `status`),
    KEY `batch_image_items_provider_source_object_idx` (`provider_source_object`),
    CONSTRAINT `batch_image_items_job_fk` FOREIGN KEY (`job_id`) REFERENCES `batch_image_jobs` (`batch_id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `batch_image_events` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `job_id` VARCHAR(64) NOT NULL,
    `event_type` VARCHAR(64) NOT NULL,
    `payload` JSON NULL,
    `event_hash` VARCHAR(128) NULL,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    UNIQUE KEY `batch_image_events_job_event_hash_uq` (`job_id`, `event_hash`),
    KEY `batch_image_events_job_created_at_idx` (`job_id`, `created_at`),
    KEY `batch_image_events_event_type_idx` (`event_type`),
    CONSTRAINT `batch_image_events_job_fk` FOREIGN KEY (`job_id`) REFERENCES `batch_image_jobs` (`batch_id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'users'
      AND column_name = 'frozen_balance'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `users` ADD COLUMN `frozen_balance` DECIMAL(20,8) NOT NULL DEFAULT 0 AFTER `balance`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'groups'
      AND column_name = 'allow_batch_image_generation'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `groups` ADD COLUMN `allow_batch_image_generation` BOOLEAN NOT NULL DEFAULT FALSE AFTER `allow_image_generation`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'groups'
      AND column_name = 'batch_image_discount_multiplier'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `groups` ADD COLUMN `batch_image_discount_multiplier` DECIMAL(10,4) NOT NULL DEFAULT 0.5 AFTER `image_price_4k`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'groups'
      AND column_name = 'batch_image_hold_multiplier'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `groups` ADD COLUMN `batch_image_hold_multiplier` DECIMAL(10,4) NOT NULL DEFAULT 0.6 AFTER `batch_image_discount_multiplier`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

ALTER TABLE `groups`
    MODIFY COLUMN `batch_image_discount_multiplier` DECIMAL(10,4) NOT NULL DEFAULT 0.5,
    MODIFY COLUMN `batch_image_hold_multiplier` DECIMAL(10,4) NOT NULL DEFAULT 0.6;

UPDATE `groups`
SET `batch_image_discount_multiplier` = 0.5
WHERE `batch_image_discount_multiplier` = 1.0;

UPDATE `groups`
SET `batch_image_hold_multiplier` = 0.6
WHERE `batch_image_hold_multiplier` = 1.05;

UPDATE `batch_image_jobs`
SET `user_deleted_at` = COALESCE(`user_deleted_at`, `updated_at`, `created_at`, CURRENT_TIMESTAMP(6)),
    `updated_at` = CURRENT_TIMESTAMP(6)
WHERE `user_deleted_at` IS NULL
  AND `provider_job_name` IS NULL
  AND `status` = 'failed'
  AND `last_error_code` IN (
      'INSUFFICIENT_BALANCE',
      'PROVIDER_SUBMIT_FAILED',
      'BATCH_IMAGE_PROVIDER_SUBMIT_FAILED',
      'BATCH_IMAGE_VERTEX_GCS_BUCKET_MISSING',
      'VERTEX_MANAGED_GCS_BUCKET_MISSING',
      'BATCH_IMAGE_PROVIDER_MISSING_API_KEY',
      'BATCH_IMAGE_PROVIDER_MISSING_SERVICE_ACCOUNT',
      'BATCH_IMAGE_PROVIDER_UNSUPPORTED_ACCOUNT'
  );

UPDATE `batch_image_jobs`
SET `task_name` = DATE_FORMAT(DATE_ADD(`created_at`, INTERVAL 8 HOUR), '%Y-%m-%d %H:%i:%s')
WHERE `task_name` = '';
