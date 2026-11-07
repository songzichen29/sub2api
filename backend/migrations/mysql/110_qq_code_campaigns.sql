CREATE TABLE IF NOT EXISTS qqc_code_campaigns (
 id VARCHAR(32) PRIMARY KEY,
 app_id VARCHAR(128) NOT NULL,
 config_json LONGTEXT NOT NULL,
 version BIGINT NOT NULL DEFAULT 1,
 created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE IF NOT EXISTS qqc_code_campaign_groups (
 app_id VARCHAR(128) NOT NULL,
 group_id VARCHAR(128) NOT NULL,
 campaign_id VARCHAR(32) NOT NULL,
 PRIMARY KEY(app_id,group_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
CREATE TABLE IF NOT EXISTS qqc_code_claims (
 id VARCHAR(32) PRIMARY KEY,
 campaign_id VARCHAR(32) NOT NULL,
 app_id VARCHAR(128) NOT NULL,
 group_id VARCHAR(128) NOT NULL,
 member_id VARCHAR(128) NOT NULL,
 user_id BIGINT NOT NULL,
 period_key VARCHAR(10) NOT NULL,
 redeem_code_id BIGINT NOT NULL UNIQUE,
 delivery_status VARCHAR(16) NOT NULL DEFAULT 'pending',
 delivery_token VARCHAR(32) NOT NULL DEFAULT '',
 attempts INTEGER NOT NULL DEFAULT 0,
 last_attempt_at DATETIME(6),
 sent_at DATETIME(6),
 created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 UNIQUE KEY qqc_code_claim_user(campaign_id,user_id,period_key),
 UNIQUE KEY qqc_code_claim_member(campaign_id,member_id,period_key),
 KEY qqc_code_claims_created_idx(created_at,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
