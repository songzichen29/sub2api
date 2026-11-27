ALTER TABLE qqc_rewards
    MODIFY COLUMN checkin_day DATE GENERATED ALWAYS AS (
        CASE WHEN kind='checkin' THEN DATE(created_at) ELSE NULL END
    ) STORED;
