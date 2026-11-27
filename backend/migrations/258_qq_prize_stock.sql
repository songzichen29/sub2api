ALTER TABLE qqc_lottery_draws ADD COLUMN prize_id VARCHAR(32) NOT NULL DEFAULT '';
CREATE INDEX qqc_lottery_prize_stock ON qqc_lottery_draws(prize_id);
ALTER TABLE qqc_code_claims
 ALTER COLUMN redeem_code_id DROP NOT NULL,
 ADD COLUMN prize_id VARCHAR(32) NOT NULL DEFAULT '',
 ADD COLUMN outcome VARCHAR(16) NOT NULL DEFAULT 'prize',
 ADD COLUMN reward_snapshot TEXT,
 ADD COLUMN usage_tokens BIGINT NOT NULL DEFAULT 0,
 ADD COLUMN probability_bps INTEGER NOT NULL DEFAULT 10000,
 ADD COLUMN bonus_bps INTEGER NOT NULL DEFAULT 0;
CREATE INDEX qqc_campaign_prize_stock ON qqc_code_claims(campaign_id,prize_id);
