DROP TRIGGER IF EXISTS `trg_groups_auth_cache_invalidation_update`;
CREATE TRIGGER `trg_groups_auth_cache_invalidation_update` AFTER UPDATE ON `groups` FOR EACH ROW
INSERT INTO `auth_cache_invalidation_outbox` (`cache_key`)
SELECT SHA2(k.`key`, 256) FROM `api_keys` AS k
WHERE k.`group_id` = OLD.`id` AND k.`deleted_at` IS NULL AND k.`key` <> ''
  AND (
      NOT (OLD.`status` <=> NEW.`status`) OR
      NOT (OLD.`is_exclusive` <=> NEW.`is_exclusive`) OR
      NOT (OLD.`allow_image_generation` <=> NEW.`allow_image_generation`) OR
      NOT (OLD.`platform` <=> NEW.`platform`) OR
      NOT (OLD.`subscription_type` <=> NEW.`subscription_type`) OR
      NOT (OLD.`rate_multiplier` <=> NEW.`rate_multiplier`) OR
      NOT (OLD.`peak_rate_enabled` <=> NEW.`peak_rate_enabled`) OR
      NOT (OLD.`peak_start` <=> NEW.`peak_start`) OR
      NOT (OLD.`peak_end` <=> NEW.`peak_end`) OR
      NOT (OLD.`peak_rate_multiplier` <=> NEW.`peak_rate_multiplier`) OR
      NOT (OLD.`profit_control_enabled` <=> NEW.`profit_control_enabled`) OR
      NOT (OLD.`profit_min_margin` <=> NEW.`profit_min_margin`) OR
      NOT (OLD.`profit_safety_buffer` <=> NEW.`profit_safety_buffer`) OR
      NOT (OLD.`deleted_at` <=> NEW.`deleted_at`)
  );
