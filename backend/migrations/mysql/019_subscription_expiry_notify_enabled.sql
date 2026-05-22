-- 019_subscription_expiry_notify_enabled.sql
-- MySQL counterpart for PostgreSQL migration 141_subscription_expiry_notify_enabled.sql.
-- Keep historical behavior by enabling subscription expiry reminder emails by default.

INSERT IGNORE INTO `settings` (`key`, `value`, `updated_at`)
VALUES ('subscription_expiry_notify_enabled', 'true', NOW());
