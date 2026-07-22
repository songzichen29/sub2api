SET @col_exists := (
    SELECT COUNT(*) FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'channel_model_pricing' AND column_name = 'image_input_price'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `channel_model_pricing` ADD COLUMN `image_input_price` DECIMAL(20,12) NULL',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
