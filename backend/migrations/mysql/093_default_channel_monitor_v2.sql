-- Make the passive channel monitor the default for existing installations.
-- Administrators can still switch back to V1 explicitly in Settings.
INSERT IGNORE INTO `settings` (`key`, `value`) VALUES ('channel_monitor_mode', 'v2');
UPDATE `settings`
SET `value` = 'v2'
WHERE `key` = 'channel_monitor_mode'
  AND `value` = 'v1';
