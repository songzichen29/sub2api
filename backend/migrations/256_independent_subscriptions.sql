ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS subscription_id BIGINT;
ALTER TABLE user_subscriptions ADD COLUMN IF NOT EXISTS purchase_order_id BIGINT;

CREATE INDEX IF NOT EXISTS apikey_subscription_id ON api_keys (subscription_id);
CREATE UNIQUE INDEX IF NOT EXISTS user_subscriptions_purchase_order_unique
    ON user_subscriptions (purchase_order_id);

-- 旧密钥绑定迁移前唯一的订阅；不改订阅额度、用量或有效期。
UPDATE api_keys AS k
SET subscription_id = existing.id
FROM (
    SELECT user_id, group_id, MIN(id) AS id
    FROM user_subscriptions
    WHERE deleted_at IS NULL
    GROUP BY user_id, group_id
    HAVING COUNT(*) = 1
) AS existing
WHERE k.user_id = existing.user_id
  AND k.group_id = existing.group_id
  AND k.deleted_at IS NULL
  AND k.subscription_id IS NULL
  AND EXISTS (
      SELECT 1 FROM groups AS g
      WHERE g.id = k.group_id AND g.subscription_type = 'subscription'
  );

DROP INDEX IF EXISTS user_subscriptions_user_group_unique_active;
CREATE INDEX IF NOT EXISTS usersubscription_user_id_group_id
    ON user_subscriptions (user_id, group_id);
CREATE UNIQUE INDEX IF NOT EXISTS user_subscriptions_legacy_user_group_unique
    ON user_subscriptions (user_id, group_id)
    WHERE deleted_at IS NULL AND purchase_order_id IS NULL;
