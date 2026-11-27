ALTER TABLE qqc_lottery_draws
 ADD COLUMN prize_id VARCHAR(32) NOT NULL DEFAULT '',
 ADD INDEX qqc_lottery_prize_stock(prize_id);
ALTER TABLE qqc_code_claims
 MODIFY COLUMN redeem_code_id BIGINT NULL,
 ADD COLUMN prize_id VARCHAR(32) NOT NULL DEFAULT '',
 ADD COLUMN outcome VARCHAR(16) NOT NULL DEFAULT 'prize',
 ADD COLUMN reward_snapshot TEXT NULL,
 ADD COLUMN usage_tokens BIGINT NOT NULL DEFAULT 0,
 ADD COLUMN probability_bps INT NOT NULL DEFAULT 10000,
 ADD COLUMN bonus_bps INT NOT NULL DEFAULT 0,
 ADD INDEX qqc_campaign_prize_stock(campaign_id,prize_id);
