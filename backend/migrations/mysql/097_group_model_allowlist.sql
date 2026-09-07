SET @old_col_exists = (
    SELECT COUNT(1)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'groups'
      AND column_name = 'models_list_config'
);
SET @new_col_exists = (
    SELECT COUNT(1)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'groups'
      AND column_name = 'model_allowlist'
);
SET @sql = CASE
    WHEN @old_col_exists > 0 AND @new_col_exists = 0
        THEN 'ALTER TABLE `groups` RENAME COLUMN `models_list_config` TO `model_allowlist`'
    WHEN @old_col_exists = 0 AND @new_col_exists = 0
        THEN 'ALTER TABLE `groups` ADD COLUMN `model_allowlist` JSON NOT NULL DEFAULT (JSON_OBJECT())'
    ELSE 'SELECT 1'
END;
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @sql = IF(
    @old_col_exists > 0 AND @new_col_exists > 0,
    'UPDATE `groups` SET `model_allowlist` = `models_list_config` WHERE JSON_LENGTH(`model_allowlist`) = 0 AND JSON_LENGTH(`models_list_config`) > 0',
    'SELECT 1'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
