CREATE TABLE IF NOT EXISTS qqc_code_campaigns (
 id VARCHAR(32) PRIMARY KEY,
 app_id VARCHAR(128) NOT NULL,
 config_json TEXT NOT NULL,
 version BIGINT NOT NULL DEFAULT 1,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS qqc_code_campaign_groups (
 app_id VARCHAR(128) NOT NULL,
 group_id VARCHAR(128) NOT NULL,
 campaign_id VARCHAR(32) NOT NULL,
 PRIMARY KEY(app_id,group_id)
);
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
 last_attempt_at TIMESTAMPTZ,
 sent_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 UNIQUE(campaign_id,user_id,period_key),
 UNIQUE(campaign_id,member_id,period_key)
);
CREATE INDEX IF NOT EXISTS qqc_code_claims_created_idx ON qqc_code_claims(created_at,id);
