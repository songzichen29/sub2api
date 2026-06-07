-- 分组级自定义 /v1/models 展示列表配置。
-- 仅用于控制 GET /v1/models 的展示结果，不参与账号白名单、模型映射或网关调度。

SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'groups'
      AND column_name = 'models_list_config'
);

SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `groups` ADD COLUMN `models_list_config` json NOT NULL DEFAULT (JSON_OBJECT())',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
