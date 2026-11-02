CREATE TABLE IF NOT EXISTS qqc_groups (
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    group_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    last_event LONGTEXT NOT NULL DEFAULT (''),
    last_seen_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (app_id, group_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_bindings (
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    member_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id),
    user_id BIGINT NOT NULL ,
    nickname LONGTEXT NOT NULL DEFAULT (''),
    group_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (app_id, member_id),
    UNIQUE (app_id, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_bind_requests (
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    user_id BIGINT PRIMARY KEY ,
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    code_hash VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL UNIQUE,
    member_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    nickname LONGTEXT NOT NULL DEFAULT (''),
    group_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    expires_at DATETIME(6) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_members (
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    group_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    member_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    inviter_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    invited_at DATETIME(6),
    joined_at DATETIME(6),
    left_at DATETIME(6),
    last_membership_at DATETIME(6),
    first_seen_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (app_id, group_id, member_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_rewards (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    reward_key VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL UNIQUE,
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    group_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    member_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id),
    user_id BIGINT NOT NULL ,
    kind VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL CHECK (kind IN ('checkin','invite','lottery','redemption','growth','streak','ranking')),
    FOREIGN KEY (source_user_id) REFERENCES users(id),
    source_user_id BIGINT ,
    source_member_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    amount NUMERIC(20,8) NOT NULL CHECK (amount >= 0),
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    checkin_day DATE GENERATED ALWAYS AS (CASE WHEN kind='checkin' THEN DATE(DATE_ADD(created_at, INTERVAL 8 HOUR)) ELSE NULL END) STORED,
    UNIQUE KEY qqc_checkin_account_day (user_id,checkin_day)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_jobs (
    id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    event_key VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    event_type LONGTEXT NOT NULL,
    group_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    member_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    message_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    payload JSON NOT NULL,
    kind VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'event',
    status VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'queued',
    result_text LONGTEXT NOT NULL DEFAULT (''),
    result_image MEDIUMBLOB,
    image_type LONGTEXT NOT NULL DEFAULT (''),
    error LONGTEXT NOT NULL DEFAULT (''),
    input_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    started_at DATETIME(6),
    finished_at DATETIME(6),
    UNIQUE (app_id, event_key),
    reward_code_hash LONGTEXT NOT NULL DEFAULT (''),
    trigger_reason LONGTEXT NOT NULL DEFAULT (''),
    decision_reason LONGTEXT NOT NULL DEFAULT (''),
    model_reserved_at DATETIME(6),
    reply_reserved_at DATETIME(6),
    reply_sent_at DATETIME(6),
    member_nickname LONGTEXT NOT NULL DEFAULT (''),
    message_preview LONGTEXT NOT NULL DEFAULT (''),
    image_decision JSON NOT NULL DEFAULT ('{}'),
    site_query JSON NOT NULL DEFAULT ('{}'),
    chat_execution JSON
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_daily_usage (
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    scope VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    day DATE NOT NULL,
    kind VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    count INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (app_id, scope, day, kind)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_conversations (
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    group_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    member_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    messages JSON NOT NULL DEFAULT ('[]'),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (app_id, group_id, member_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_email_bind_requests (
    token_hash VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    member_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    group_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    nickname LONGTEXT NOT NULL DEFAULT (''),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL ,
    email LONGTEXT NOT NULL,
    expires_at DATETIME(6) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    UNIQUE (app_id, member_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_email_limits (
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    scope VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    day DATE NOT NULL,
    count INTEGER NOT NULL DEFAULT 0,
    last_sent_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (app_id, scope, day)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_lottery_draws (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    request_key VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL UNIQUE,
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    member_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    group_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id),
    user_id BIGINT NOT NULL ,
    prize_name LONGTEXT NOT NULL,
    amount NUMERIC(20,8) NOT NULL CHECK (amount >= 0),
    config_snapshot JSON NOT NULL,
    recharge_total NUMERIC(20,8) NOT NULL CHECK (recharge_total >= 0),
    earned BIGINT NOT NULL CHECK (earned >= 0),
    remaining BIGINT NOT NULL CHECK (remaining >= 0),
    result_text LONGTEXT NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_message_references (
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    group_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    message_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    ref_idx VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    content LONGTEXT NOT NULL DEFAULT (''),
    is_image BOOLEAN NOT NULL DEFAULT FALSE,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (app_id, group_id, message_id),
    shareable BOOLEAN NOT NULL DEFAULT FALSE,
    image_job_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT (''),
    source_job_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT ('')
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_transfer_requests (
    token_hash     VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin             NOT NULL PRIMARY KEY,
    app_id         VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin             NOT NULL,
    from_member_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin             NOT NULL,
    from_user_id   bigint           NOT NULL,
    from_email     LONGTEXT             NOT NULL,
    to_user_id     bigint           NOT NULL,
    to_email       LONGTEXT             NOT NULL,
    amount         numeric(18,8)    NOT NULL,
    fee            numeric(18,8)    NOT NULL,
    net_amount     numeric(18,8)    NOT NULL,
    expires_at     DATETIME(6)      NOT NULL,
    created_at     DATETIME(6)      NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    UNIQUE (app_id, from_member_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_group_messages (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    group_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    message_key VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    role LONGTEXT NOT NULL CHECK (role IN ('user', 'assistant')),
    content LONGTEXT NOT NULL,
    context_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    UNIQUE (app_id, group_id, message_key),
    member_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT ''
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_reward_codes (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    code_hash VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin UNIQUE,
    code_hint LONGTEXT NOT NULL,
    batch_name LONGTEXT NOT NULL,
    reward_type LONGTEXT NOT NULL CHECK (reward_type IN ('balance','chances','physical','other')),
    amount NUMERIC(20,8) NOT NULL DEFAULT 0,
    quantity INTEGER NOT NULL DEFAULT 0,
    prize_name LONGTEXT NOT NULL DEFAULT (''),
    claim_instructions LONGTEXT NOT NULL DEFAULT (''),
    FOREIGN KEY (target_user_id) REFERENCES users(id),
    target_user_id BIGINT ,
    expires_at DATETIME(6),
    status VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'unused' CHECK (status IN ('unused','disabled','pending','fulfilled')),
    FOREIGN KEY (user_id) REFERENCES users(id),
    user_id BIGINT ,
    member_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    group_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    redeemed_at DATETIME(6),
    fulfilled_at DATETIME(6),
    fulfillment_note LONGTEXT NOT NULL DEFAULT (''),
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    CHECK ((reward_type='balance' AND amount>0 AND amount<=1000 AND quantity=0)
        OR (reward_type<>'balance' AND amount=0 AND quantity BETWEEN 1 AND 1000)),
    CHECK ((status IN ('unused','disabled') AND user_id IS NULL AND redeemed_at IS NULL)
        OR (status IN ('pending','fulfilled') AND user_id IS NOT NULL AND redeemed_at IS NOT NULL)),
    source_kind VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'code',
    source_key VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_reward_limits (
    scope VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
    last_attempt_at DATETIME(6) NOT NULL DEFAULT '1970-01-01 00:00:00',
    blocked_until DATETIME(6) NOT NULL DEFAULT '1970-01-01 00:00:00'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_reward_attempts (
    request_key VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    member_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id),
    user_id BIGINT NOT NULL ,
    FOREIGN KEY (code_id) REFERENCES qqc_reward_codes(id),
    code_id BIGINT ,
    failed BOOLEAN NOT NULL,
    result_text LONGTEXT NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_price_snapshots (
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
    snapshot JSON NOT NULL,
    next_check_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_autonomous_state (
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    group_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    messages BIGINT NOT NULL DEFAULT 0,
    last_message_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    last_message_at DATETIME(6),
    last_text_hash VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    last_candidate_at DATETIME(6),
    last_reply_at DATETIME(6),
    last_reply_member VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    last_reply_kind VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    last_reply_trigger VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    last_reply_counter BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY(app_id,group_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_autonomous_seen (
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    group_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    message_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY(app_id,group_id,message_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_autonomous_decisions (
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    group_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    day DATE NOT NULL,
    reason VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    count BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY(app_id,group_id,day,reason)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_health_snapshots (
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
    snapshot JSON NOT NULL,
    next_check_at DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_ranking_config_versions (
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    effective_day DATE NOT NULL,
    config JSON NOT NULL,
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY(app_id, effective_day)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_ranking_settlements (
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    ranking_day DATE NOT NULL,
    rules JSON NOT NULL,
    reward_count INTEGER NOT NULL DEFAULT 0,
    settled_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY(app_id, ranking_day)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_ranking_claims (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    ranking_day DATE NOT NULL,
    ranking_kind VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL CHECK (ranking_kind IN ('tokens','amount')),
    `rank` INTEGER NOT NULL CHECK (`rank` BETWEEN 1 AND 10),
    FOREIGN KEY (user_id) REFERENCES users(id),
    user_id BIGINT NOT NULL ,
    source_key VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL UNIQUE,
    reward JSON NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    claimed_at DATETIME(6),
    UNIQUE(app_id,ranking_day,ranking_kind,user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_periodic_growth_progress (
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL ,
    task_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    kind VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL CHECK (kind IN ('usage_count','usage_tokens','usage_amount','recharge_total','checkin_total')),
    consumed DECIMAL(30,8) NOT NULL DEFAULT 0 CHECK (consumed >= 0),
    cycles BIGINT NOT NULL DEFAULT 0 CHECK (cycles >= 0),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY(user_id, task_id, kind)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_incoming_images (
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    group_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    message_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    ref_idx VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    attachments JSON NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (app_id, group_id, message_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_moderation_events (
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    sandbox BOOLEAN NOT NULL,
    group_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    message_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    member_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    policy_hash VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    content_hash VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    sent_at DATETIME(6) NOT NULL,
    blocked BOOLEAN NOT NULL DEFAULT FALSE,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY(app_id,sandbox,group_id,message_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_moderation_members (
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    sandbox BOOLEAN NOT NULL,
    group_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    member_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    cooldown_until DATETIME(6) NOT NULL DEFAULT '1970-01-01 00:00:00',
    last_muted_at DATETIME(6) NOT NULL DEFAULT '1970-01-01 00:00:00',
    active_incident_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY(app_id,sandbox,group_id,member_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_moderation_incidents (
    id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    sandbox BOOLEAN NOT NULL,
    group_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    member_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    policy_hash VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    reason VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    mute_seconds INTEGER NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_moderation_operations (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    FOREIGN KEY (incident_id) REFERENCES qqc_moderation_incidents(id) ON DELETE CASCADE,
    incident_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL ,
    kind VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL CHECK (kind IN ('mute','recall')),
    message_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    expires_at DATETIME(6) NOT NULL,
    status VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'queued',
    error LONGTEXT NOT NULL DEFAULT (''),
    started_at DATETIME(6),
    finished_at DATETIME(6),
    UNIQUE(incident_id,kind,message_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_moderation_rates (
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    sandbox BOOLEAN NOT NULL,
    kind VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    next_at DATETIME(6) NOT NULL DEFAULT '1970-01-01 00:00:00',
    PRIMARY KEY(app_id,sandbox,kind)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_transfer_history (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    from_member_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    from_user_id BIGINT NOT NULL,
    to_user_id BIGINT NOT NULL,
    from_email LONGTEXT NOT NULL,
    to_email LONGTEXT NOT NULL,
    amount NUMERIC(18,8) NOT NULL CHECK(amount > 0),
    fee NUMERIC(18,8) NOT NULL CHECK(fee >= 0),
    net_amount NUMERIC(18,8) NOT NULL CHECK(net_amount > 0),
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    CHECK(amount = fee + net_amount)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_pending_image_plans (
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    group_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    member_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    FOREIGN KEY (job_id) REFERENCES qqc_jobs(id) ON DELETE CASCADE,
    job_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL ,
    source_message_id LONGTEXT NOT NULL,
    action VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL CHECK(action IN ('generate_image','edit_image')),
    prompt LONGTEXT NOT NULL CHECK(char_length(prompt) BETWEEN 1 AND 6000),
    request_quote LONGTEXT NOT NULL CHECK(char_length(request_quote) BETWEEN 1 AND 300),
    expires_at DATETIME(6) NOT NULL,
    PRIMARY KEY(app_id,group_id,member_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_spam_report_sources (
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    sandbox BOOLEAN NOT NULL,
    group_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    message_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    ref_idx VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    member_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    member_role VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    is_bot BOOLEAN NOT NULL DEFAULT FALSE,
    eligible BOOLEAN NOT NULL DEFAULT FALSE,
    content_encrypted LONGTEXT NOT NULL DEFAULT (''),
    content_hash VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    policy_hash VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    sent_at DATETIME(6) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY(app_id,sandbox,group_id,message_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qqc_spam_reports (
    id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    sandbox BOOLEAN NOT NULL,
    group_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    reporter_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    message_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    member_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    policy_hash VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    status VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'evaluating',
    category LONGTEXT NOT NULL DEFAULT (''),
    reason TEXT NOT NULL,
    incident_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    finished_at DATETIME(6),
    UNIQUE(app_id,sandbox,group_id,message_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE INDEX qqc_rewards_user_time ON qqc_rewards(user_id, created_at DESC);

CREATE INDEX qqc_jobs_queue ON qqc_jobs(created_at);

CREATE INDEX qqc_jobs_member ON qqc_jobs(app_id, group_id, member_id, created_at DESC);

CREATE INDEX qqc_email_bind_requests_user ON qqc_email_bind_requests(user_id);

CREATE INDEX qqc_lottery_draws_user_time ON qqc_lottery_draws(user_id, created_at DESC);

CREATE INDEX qqc_rewards_app_kind_time
    ON qqc_rewards(app_id, kind, created_at DESC, id DESC);

CREATE INDEX qqc_rewards_invite_source_member
    ON qqc_rewards(app_id, source_member_id, id);

CREATE INDEX qqc_rewards_invite_source_user
    ON qqc_rewards(source_user_id, id);

CREATE INDEX qqc_lottery_draws_app_time
    ON qqc_lottery_draws(app_id, created_at DESC, id DESC);

CREATE INDEX qqc_members_invite_time
    ON qqc_members(app_id, invited_at DESC, group_id, member_id)
   ;

CREATE INDEX qqc_message_references_ref
    ON qqc_message_references(app_id, group_id, ref_idx);

CREATE INDEX qqc_message_references_expiry
    ON qqc_message_references(created_at);

CREATE INDEX idx_qqc_group_messages_recent
    ON qqc_group_messages (app_id, group_id, context_at DESC, id DESC);

CREATE INDEX idx_qqc_group_messages_expiry
    ON qqc_group_messages (created_at);

CREATE INDEX qqc_reward_codes_user ON qqc_reward_codes(user_id, id DESC);

CREATE INDEX qqc_reward_attempts_member ON qqc_reward_attempts(app_id,member_id,created_at DESC);

CREATE INDEX qqc_reward_attempts_user ON qqc_reward_attempts(user_id,created_at DESC);

CREATE INDEX qqc_jobs_price_dispatch
    ON qqc_jobs(app_id, started_at DESC)
   ;

CREATE INDEX qqc_jobs_autonomous_limits ON qqc_jobs(app_id,group_id,reply_reserved_at DESC);

CREATE INDEX qqc_autonomous_seen_expiry ON qqc_autonomous_seen(created_at);

CREATE INDEX qqc_health_notification_queue ON qqc_jobs(app_id,created_at)
   ;

CREATE UNIQUE INDEX qqc_reward_codes_source_key ON qqc_reward_codes(source_key);

CREATE INDEX idx_qqc_ranking_claims_user
    ON qqc_ranking_claims(app_id,user_id,ranking_day DESC,id DESC);

CREATE INDEX qqc_incoming_images_ref
    ON qqc_incoming_images(app_id, group_id, ref_idx);

CREATE INDEX qqc_incoming_images_expiry
    ON qqc_incoming_images(created_at);

CREATE INDEX qqc_moderation_events_window ON qqc_moderation_events(app_id,sandbox,group_id,member_id,sent_at DESC);

CREATE INDEX qqc_moderation_events_expiry ON qqc_moderation_events(created_at);

CREATE INDEX qqc_moderation_incidents_recent ON qqc_moderation_incidents(app_id,sandbox,created_at DESC);

CREATE INDEX qqc_moderation_operations_pending ON qqc_moderation_operations(kind,id);

CREATE INDEX qqc_jobs_recent_speakers
    ON qqc_jobs(app_id, created_at DESC, id DESC)
   ;

CREATE INDEX qqc_transfer_history_app_time ON qqc_transfer_history(app_id,created_at DESC,id DESC);

CREATE INDEX qqc_pending_image_plans_expiry ON qqc_pending_image_plans(expires_at);

CREATE INDEX idx_qqc_group_messages_member
 ON qqc_group_messages (app_id,group_id,member_id,context_at DESC,id DESC);

CREATE INDEX qqc_spam_report_sources_reference ON qqc_spam_report_sources(app_id,sandbox,group_id,ref_idx);

CREATE INDEX qqc_spam_report_sources_expiry ON qqc_spam_report_sources(sent_at);

CREATE INDEX qqc_spam_report_sources_member ON qqc_spam_report_sources(app_id,sandbox,group_id,member_id,sent_at DESC);

CREATE INDEX qqc_spam_reports_reporter ON qqc_spam_reports(app_id,sandbox,reporter_id,created_at DESC);

CREATE INDEX qqc_spam_reports_group ON qqc_spam_reports(app_id,sandbox,group_id,created_at DESC);

CREATE INDEX qqc_spam_reports_incident ON qqc_spam_reports(incident_id);

CREATE INDEX qqc_spam_reports_expiry ON qqc_spam_reports(created_at);

CREATE INDEX idx_qqc_jobs_site_query_member
 ON qqc_jobs(app_id,group_id,member_id,created_at DESC,id DESC)
;

CREATE TABLE IF NOT EXISTS qqc_transaction_lock (id INTEGER PRIMARY KEY) ENGINE=InnoDB;
INSERT IGNORE INTO qqc_transaction_lock(id) VALUES(1);

CREATE TABLE IF NOT EXISTS qqc_lottery_chance_grants (
 source_key VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 user_id BIGINT NOT NULL,
 chances INTEGER NOT NULL CHECK(chances BETWEEN 1 AND 1000),
 created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 FOREIGN KEY(user_id) REFERENCES users(id),
 INDEX qqc_lottery_chance_grants_user(user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
