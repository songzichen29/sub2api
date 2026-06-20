-- Add payment coupons and order discount fields.

CREATE TABLE IF NOT EXISTS `coupons` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `code` VARCHAR(32) NOT NULL,
    `type` VARCHAR(20) NOT NULL,
    `value` DECIMAL(20,8) NOT NULL,
    `min_amount` DECIMAL(20,2) NOT NULL DEFAULT 0,
    `max_discount` DECIMAL(20,2) NOT NULL DEFAULT 0,
    `scope` VARCHAR(20) NOT NULL DEFAULT 'all',
    `max_uses` INT NOT NULL DEFAULT 0,
    `used_count` INT NOT NULL DEFAULT 0,
    `per_user_limit` INT NOT NULL DEFAULT 1,
    `status` VARCHAR(20) NOT NULL DEFAULT 'active',
    `starts_at` DATETIME(6) NULL,
    `expires_at` DATETIME(6) NULL,
    `notes` LONGTEXT NULL,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    UNIQUE KEY `coupons_code_key` (`code`),
    KEY `idx_coupons_status` (`status`),
    KEY `idx_coupons_scope` (`scope`),
    KEY `idx_coupons_expires_at` (`expires_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `coupon_usages` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `coupon_id` BIGINT NOT NULL,
    `user_id` BIGINT NOT NULL,
    `order_id` BIGINT NOT NULL,
    `discount_amount` DECIMAL(20,2) NOT NULL,
    `used_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `status` VARCHAR(20) NOT NULL DEFAULT 'used',
    PRIMARY KEY (`id`),
    UNIQUE KEY `coupon_usages_order_id_key` (`order_id`),
    KEY `idx_coupon_usages_coupon_id` (`coupon_id`),
    KEY `idx_coupon_usages_user_id` (`user_id`),
    KEY `idx_coupon_usages_coupon_user` (`coupon_id`, `user_id`),
    KEY `idx_coupon_usages_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'payment_orders'
      AND column_name = 'discount_amount'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `payment_orders` ADD COLUMN `discount_amount` DECIMAL(20,2) NOT NULL DEFAULT 0 COMMENT ''Threshold discount amount'' AFTER `fee_rate`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'payment_orders'
      AND column_name = 'coupon_code'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `payment_orders` ADD COLUMN `coupon_code` VARCHAR(32) NOT NULL DEFAULT '''' COMMENT ''Payment coupon code'' AFTER `discount_amount`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'payment_orders'
      AND column_name = 'coupon_discount_amount'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `payment_orders` ADD COLUMN `coupon_discount_amount` DECIMAL(20,2) NOT NULL DEFAULT 0 COMMENT ''Coupon discount amount'' AFTER `coupon_code`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists := (
    SELECT COUNT(*)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'payment_orders'
      AND index_name = 'idx_payment_orders_coupon_code'
);
SET @ddl := IF(@idx_exists = 0,
    'CREATE INDEX `idx_payment_orders_coupon_code` ON `payment_orders` (`coupon_code`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
