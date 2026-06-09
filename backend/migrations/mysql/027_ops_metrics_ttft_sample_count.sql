-- ops_metrics: TTFT 样本数
--
-- first_token_ms 只存在于能观测到首 token 的请求，不能用 success_count
-- 作为 TTFT 聚合权重，否则非流式/未记录首 token 的成功请求会稀释展示值。
--
-- 防御性处理：只有目标表存在且列不存在时才执行 ALTER；列追加到末尾，避免
-- 异常旧表缺少 ttft_max_ms 时因为 AFTER 子句失败。

SET @table_exists := (
    SELECT COUNT(*)
    FROM information_schema.tables
    WHERE table_schema = DATABASE()
      AND table_name = 'ops_metrics_hourly'
);
SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'ops_metrics_hourly'
      AND column_name = 'ttft_sample_count'
);
SET @ddl := IF(@table_exists > 0 AND @col_exists = 0,
    'ALTER TABLE `ops_metrics_hourly` ADD COLUMN `ttft_sample_count` bigint NOT NULL DEFAULT 0',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @table_exists := (
    SELECT COUNT(*)
    FROM information_schema.tables
    WHERE table_schema = DATABASE()
      AND table_name = 'ops_metrics_daily'
);
SET @col_exists := (
    SELECT COUNT(*)
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'ops_metrics_daily'
      AND column_name = 'ttft_sample_count'
);
SET @ddl := IF(@table_exists > 0 AND @col_exists = 0,
    'ALTER TABLE `ops_metrics_daily` ADD COLUMN `ttft_sample_count` bigint NOT NULL DEFAULT 0',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
