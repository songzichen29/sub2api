-- 添加 user_subscriptions.source 字段，用于标识订阅来源并约束重置规则。
-- 取值：
--   admin   = 管理员手动分配（按窗口层级规则可部分重置）
--   redeem  = 用户用兑换码兑换（同上）
--   payment = 用户付费购买（永久禁止重置）
ALTER TABLE `user_subscriptions`
    ADD COLUMN IF NOT EXISTS `source` varchar(20) NOT NULL DEFAULT 'admin';

-- 历史数据回填：
--   assigned_by 非空 → 视为 admin（保持原值，无需更新）
--   assigned_by 为空 → 历史上无法精确区分 redeem 与 payment，
--                     保守地标记为 redeem（保留可重置能力，避免误锁旧支付订单时
--                     管理员需要救场）。新订阅由各业务入口显式写入正确 source。
UPDATE `user_subscriptions`
   SET `source` = 'redeem'
 WHERE `assigned_by` IS NULL
   AND `source` = 'admin';
