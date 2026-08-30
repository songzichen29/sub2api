SET @allow_image_generation_exists = (
    SELECT COUNT(1) FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'groups'
      AND column_name = 'allow_image_generation'
);
SET @sql = IF(@allow_image_generation_exists = 1,
    'UPDATE `groups` SET `allow_image_generation` = TRUE WHERE `platform` = ''grok'' AND `allow_image_generation` = FALSE',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
