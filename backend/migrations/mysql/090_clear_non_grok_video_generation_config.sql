CREATE TABLE IF NOT EXISTS `groups_video_price_backup_220` AS
SELECT
    `id` AS `group_id`,
    `platform`,
    `video_price_480p`,
    `video_price_720p`,
    `video_price_1080p`,
    `video_model_prices`,
    CURRENT_TIMESTAMP(6) AS `backed_up_at`
FROM `groups`
WHERE (`platform` IS NULL OR `platform` NOT IN ('grok', 'composite'))
  AND (`video_price_480p` IS NOT NULL
    OR `video_price_720p` IS NOT NULL
    OR `video_price_1080p` IS NOT NULL
    OR `video_model_prices` IS NOT NULL);

UPDATE `groups`
SET `video_price_480p` = NULL,
    `video_price_720p` = NULL,
    `video_price_1080p` = NULL,
    `video_model_prices` = NULL
WHERE (`platform` IS NULL OR `platform` NOT IN ('grok', 'composite'))
  AND (`video_price_480p` IS NOT NULL
    OR `video_price_720p` IS NOT NULL
    OR `video_price_1080p` IS NOT NULL
    OR `video_model_prices` IS NOT NULL);
