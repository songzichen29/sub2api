-- 视频生成独立计费：分组级独立倍率开关、倍率与分档单价。
-- 与 ent schema 中 groups 表的 video_rate_independent / video_rate_multiplier /
-- video_price_480p / video_price_720p / video_price_1080p 字段对齐。
-- 列顺序位于 batch_image_hold_multiplier 之后（由 044 迁移保证已存在）。

SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'groups'
      AND column_name = 'video_rate_independent'
);

SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `groups` ADD COLUMN `video_rate_independent` BOOLEAN NOT NULL DEFAULT FALSE COMMENT ''是否启用视频生成独立倍率'' AFTER `batch_image_hold_multiplier`',
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
      AND column_name = 'video_rate_multiplier'
);

SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `groups` ADD COLUMN `video_rate_multiplier` DECIMAL(10,4) NOT NULL DEFAULT 1.0000 COMMENT ''视频生成倍率，仅 video_rate_independent=true 时生效'' AFTER `video_rate_independent`',
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
      AND column_name = 'video_price_480p'
);

SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `groups` ADD COLUMN `video_price_480p` DECIMAL(20,8) NULL COMMENT ''480p 视频单价，NULL 表示使用默认价'' AFTER `video_rate_multiplier`',
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
      AND column_name = 'video_price_720p'
);

SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `groups` ADD COLUMN `video_price_720p` DECIMAL(20,8) NULL COMMENT ''720p 视频单价，NULL 表示使用默认价'' AFTER `video_price_480p`',
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
      AND column_name = 'video_price_1080p'
);

SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `groups` ADD COLUMN `video_price_1080p` DECIMAL(20,8) NULL COMMENT ''1080p 视频单价，NULL 表示使用默认价'' AFTER `video_price_720p`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
