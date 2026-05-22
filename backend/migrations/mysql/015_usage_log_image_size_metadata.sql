-- 015_usage_log_image_size_metadata.sql
-- MySQL counterpart for PostgreSQL migration 136_usage_log_image_size_metadata.sql.
--
-- The application now reads/writes generated-image billing size metadata from
-- usage_logs. Without these columns, /api/v1/usage list queries fail with
-- "Unknown column" after the OpenAI/Gateway merge.
--
-- PostgreSQL uses JSONB and CHECK constraints. MySQL stores the breakdown in a
-- JSON column and intentionally avoids CHECK constraints here to keep the
-- migration compatible with deployed MySQL versions.

-- usage_logs.image_input_size
SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'usage_logs'
      AND column_name = 'image_input_size'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `usage_logs` ADD COLUMN `image_input_size` varchar(32) NULL COMMENT ''Input image size used for generated-image billing metadata'' AFTER `image_size`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- usage_logs.image_output_size
SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'usage_logs'
      AND column_name = 'image_output_size'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `usage_logs` ADD COLUMN `image_output_size` varchar(32) NULL COMMENT ''Output image size used for generated-image billing metadata'' AFTER `image_input_size`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- usage_logs.image_size_source
SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'usage_logs'
      AND column_name = 'image_size_source'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `usage_logs` ADD COLUMN `image_size_source` varchar(16) NULL COMMENT ''Billing image size source: output/input/default/legacy'' AFTER `image_output_size`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- usage_logs.image_size_breakdown
SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'usage_logs'
      AND column_name = 'image_size_breakdown'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `usage_logs` ADD COLUMN `image_size_breakdown` json NULL COMMENT ''Generated-image billing size breakdown'' AFTER `image_size_source`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
