SET @col_exists = (SELECT COUNT(*) FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'api_keys' AND column_name = 'subscription_id');
SET @ddl = IF(@col_exists = 0,
    'ALTER TABLE `api_keys` ADD COLUMN `subscription_id` BIGINT NULL', 'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists = (SELECT COUNT(*) FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'user_subscriptions' AND column_name = 'purchase_order_id');
SET @ddl = IF(@col_exists = 0,
    'ALTER TABLE `user_subscriptions` ADD COLUMN `purchase_order_id` BIGINT NULL', 'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @index_exists = (SELECT COUNT(*) FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'api_keys' AND index_name = 'apikey_subscription_id');
SET @ddl = IF(@index_exists = 0,
    'CREATE INDEX `apikey_subscription_id` ON `api_keys` (`subscription_id`)', 'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @index_exists = (SELECT COUNT(*) FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'user_subscriptions' AND index_name = 'user_subscriptions_purchase_order_unique');
SET @ddl = IF(@index_exists = 0,
    'CREATE UNIQUE INDEX `user_subscriptions_purchase_order_unique` ON `user_subscriptions` (`purchase_order_id`)', 'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- MySQL 基线的用户与分组组合已是普通索引，无需删除。
-- 只绑定唯一的旧订阅，不拆分或重置历史权益。
UPDATE api_keys AS k
JOIN (
    SELECT user_id, group_id, MIN(id) AS id
    FROM user_subscriptions
    WHERE deleted_at IS NULL
    GROUP BY user_id, group_id
    HAVING COUNT(*) = 1
) AS existing ON existing.user_id = k.user_id AND existing.group_id = k.group_id
JOIN `groups` AS g ON g.id = k.group_id AND g.subscription_type = 'subscription'
SET k.subscription_id = existing.id
WHERE k.deleted_at IS NULL AND k.subscription_id IS NULL;
