SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'groups'
      AND column_name = 'web_search_price_per_call'
);

SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `groups` ADD COLUMN `web_search_price_per_call` DECIMAL(20,8) NULL COMMENT ''Codex web search price per call in USD, NULL uses default 0.01'' AFTER `batch_image_hold_multiplier`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
