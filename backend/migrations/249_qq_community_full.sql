CREATE TABLE IF NOT EXISTS qqc_groups (
    app_id TEXT NOT NULL,
    group_id TEXT NOT NULL,
    last_event TEXT NOT NULL DEFAULT '',
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (app_id, group_id)
);

CREATE TABLE IF NOT EXISTS qqc_bindings (
    app_id TEXT NOT NULL,
    member_id TEXT NOT NULL,
    user_id BIGINT NOT NULL REFERENCES users(id),
    nickname TEXT NOT NULL DEFAULT '',
    group_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (app_id, member_id),
    UNIQUE (app_id, user_id)
);

CREATE TABLE IF NOT EXISTS qqc_bind_requests (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    app_id TEXT NOT NULL,
    code_hash TEXT NOT NULL UNIQUE,
    member_id TEXT NOT NULL DEFAULT '',
    nickname TEXT NOT NULL DEFAULT '',
    group_id TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS qqc_members (
    app_id TEXT NOT NULL,
    group_id TEXT NOT NULL,
    member_id TEXT NOT NULL,
    inviter_id TEXT NOT NULL DEFAULT '',
    invited_at TIMESTAMPTZ,
    joined_at TIMESTAMPTZ,
    left_at TIMESTAMPTZ,
    last_membership_at TIMESTAMPTZ,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (app_id, group_id, member_id)
);

CREATE TABLE IF NOT EXISTS qqc_rewards (
    id BIGSERIAL PRIMARY KEY,
    reward_key TEXT NOT NULL UNIQUE,
    app_id TEXT NOT NULL,
    group_id TEXT NOT NULL,
    member_id TEXT NOT NULL,
    user_id BIGINT NOT NULL REFERENCES users(id),
    kind TEXT NOT NULL CHECK (kind IN ('checkin','invite','lottery','redemption','growth','streak','ranking')),
    source_user_id BIGINT REFERENCES users(id),
    source_member_id TEXT NOT NULL DEFAULT '',
    amount NUMERIC(20,8) NOT NULL CHECK (amount >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS qqc_jobs (
    id TEXT PRIMARY KEY,
    app_id TEXT NOT NULL,
    event_key TEXT NOT NULL,
    event_type TEXT NOT NULL,
    group_id TEXT NOT NULL,
    member_id TEXT NOT NULL DEFAULT '',
    message_id TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL,
    kind TEXT NOT NULL DEFAULT 'event',
    status TEXT NOT NULL DEFAULT 'queued',
    result_text TEXT NOT NULL DEFAULT '',
    result_image BYTEA,
    image_type TEXT NOT NULL DEFAULT '',
    error TEXT NOT NULL DEFAULT '',
    input_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    UNIQUE (app_id, event_key),
    reward_code_hash TEXT NOT NULL DEFAULT '',
    trigger_reason TEXT NOT NULL DEFAULT '',
    decision_reason TEXT NOT NULL DEFAULT '',
    model_reserved_at TIMESTAMPTZ,
    reply_reserved_at TIMESTAMPTZ,
    reply_sent_at TIMESTAMPTZ,
    member_nickname TEXT NOT NULL DEFAULT '',
    message_preview TEXT NOT NULL DEFAULT '',
    image_decision JSONB NOT NULL DEFAULT '{}'::jsonb,
    site_query JSONB NOT NULL DEFAULT '{}'::jsonb,
    chat_execution JSONB
);

CREATE TABLE IF NOT EXISTS qqc_daily_usage (
    app_id TEXT NOT NULL,
    scope TEXT NOT NULL,
    day DATE NOT NULL,
    kind TEXT NOT NULL,
    count INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (app_id, scope, day, kind)
);

CREATE TABLE IF NOT EXISTS qqc_conversations (
    app_id TEXT NOT NULL,
    group_id TEXT NOT NULL,
    member_id TEXT NOT NULL,
    messages JSONB NOT NULL DEFAULT '[]',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (app_id, group_id, member_id)
);

CREATE TABLE IF NOT EXISTS qqc_email_bind_requests (
    token_hash TEXT PRIMARY KEY,
    app_id TEXT NOT NULL,
    member_id TEXT NOT NULL,
    group_id TEXT NOT NULL,
    nickname TEXT NOT NULL DEFAULT '',
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    email TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (app_id, member_id)
);

CREATE TABLE IF NOT EXISTS qqc_email_limits (
    app_id TEXT NOT NULL,
    scope TEXT NOT NULL,
    day DATE NOT NULL,
    count INTEGER NOT NULL DEFAULT 0,
    last_sent_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (app_id, scope, day)
);

CREATE TABLE IF NOT EXISTS qqc_lottery_draws (
    id BIGSERIAL PRIMARY KEY,
    request_key TEXT NOT NULL UNIQUE,
    app_id TEXT NOT NULL,
    member_id TEXT NOT NULL,
    group_id TEXT NOT NULL,
    user_id BIGINT NOT NULL REFERENCES users(id),
    prize_name TEXT NOT NULL,
    amount NUMERIC(20,8) NOT NULL CHECK (amount >= 0),
    config_snapshot JSONB NOT NULL,
    recharge_total NUMERIC(20,8) NOT NULL CHECK (recharge_total >= 0),
    earned BIGINT NOT NULL CHECK (earned >= 0),
    remaining BIGINT NOT NULL CHECK (remaining >= 0),
    result_text TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS qqc_message_references (
    app_id TEXT NOT NULL,
    group_id TEXT NOT NULL,
    message_id TEXT NOT NULL,
    ref_idx TEXT NOT NULL DEFAULT '',
    content TEXT NOT NULL DEFAULT '',
    is_image BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (app_id, group_id, message_id),
    shareable BOOLEAN NOT NULL DEFAULT FALSE,
    image_job_id TEXT NOT NULL DEFAULT '',
    source_job_id TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS qqc_transfer_requests (
    token_hash     text             NOT NULL PRIMARY KEY,
    app_id         text             NOT NULL,
    from_member_id text             NOT NULL,
    from_user_id   bigint           NOT NULL,
    from_email     text             NOT NULL,
    to_user_id     bigint           NOT NULL,
    to_email       text             NOT NULL,
    amount         numeric(18,8)    NOT NULL,
    fee            numeric(18,8)    NOT NULL,
    net_amount     numeric(18,8)    NOT NULL,
    expires_at     timestamptz      NOT NULL,
    created_at     timestamptz      NOT NULL DEFAULT NOW(),
    UNIQUE (app_id, from_member_id)
);

CREATE TABLE IF NOT EXISTS qqc_group_messages (
    id BIGSERIAL PRIMARY KEY,
    app_id TEXT NOT NULL,
    group_id TEXT NOT NULL,
    message_key TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
    content TEXT NOT NULL,
    context_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    UNIQUE (app_id, group_id, message_key),
    member_id TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS qqc_reward_codes (
    id BIGSERIAL PRIMARY KEY,
    app_id TEXT NOT NULL,
    code_hash TEXT UNIQUE,
    code_hint TEXT NOT NULL,
    batch_name TEXT NOT NULL,
    reward_type TEXT NOT NULL CHECK (reward_type IN ('balance','chances','physical','other')),
    amount NUMERIC(20,8) NOT NULL DEFAULT 0,
    quantity INTEGER NOT NULL DEFAULT 0,
    prize_name TEXT NOT NULL DEFAULT '',
    claim_instructions TEXT NOT NULL DEFAULT '',
    target_user_id BIGINT REFERENCES users(id),
    expires_at TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT 'unused' CHECK (status IN ('unused','disabled','pending','fulfilled')),
    user_id BIGINT REFERENCES users(id),
    member_id TEXT NOT NULL DEFAULT '',
    group_id TEXT NOT NULL DEFAULT '',
    redeemed_at TIMESTAMPTZ,
    fulfilled_at TIMESTAMPTZ,
    fulfillment_note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK ((reward_type='balance' AND amount>0 AND amount<=1000 AND quantity=0)
        OR (reward_type<>'balance' AND amount=0 AND quantity BETWEEN 1 AND 1000)),
    CHECK ((status IN ('unused','disabled') AND user_id IS NULL AND redeemed_at IS NULL)
        OR (status IN ('pending','fulfilled') AND user_id IS NOT NULL AND redeemed_at IS NOT NULL)),
    source_kind TEXT NOT NULL DEFAULT 'code',
    source_key TEXT
);

CREATE TABLE IF NOT EXISTS qqc_reward_limits (
    scope TEXT PRIMARY KEY,
    last_attempt_at TIMESTAMPTZ NOT NULL DEFAULT 'epoch',
    blocked_until TIMESTAMPTZ NOT NULL DEFAULT 'epoch'
);

CREATE TABLE IF NOT EXISTS qqc_reward_attempts (
    request_key TEXT PRIMARY KEY,
    app_id TEXT NOT NULL,
    member_id TEXT NOT NULL,
    user_id BIGINT NOT NULL REFERENCES users(id),
    code_id BIGINT REFERENCES qqc_reward_codes(id),
    failed BOOLEAN NOT NULL,
    result_text TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS qqc_price_snapshots (
    app_id TEXT PRIMARY KEY,
    snapshot JSONB NOT NULL,
    next_check_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS qqc_autonomous_state (
    app_id TEXT NOT NULL,
    group_id TEXT NOT NULL,
    messages BIGINT NOT NULL DEFAULT 0,
    last_message_id TEXT NOT NULL DEFAULT '',
    last_message_at TIMESTAMPTZ,
    last_text_hash TEXT NOT NULL DEFAULT '',
    last_candidate_at TIMESTAMPTZ,
    last_reply_at TIMESTAMPTZ,
    last_reply_member TEXT NOT NULL DEFAULT '',
    last_reply_kind TEXT NOT NULL DEFAULT '',
    last_reply_trigger TEXT NOT NULL DEFAULT '',
    last_reply_counter BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY(app_id,group_id)
);

CREATE TABLE IF NOT EXISTS qqc_autonomous_seen (
    app_id TEXT NOT NULL,
    group_id TEXT NOT NULL,
    message_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY(app_id,group_id,message_id)
);

CREATE TABLE IF NOT EXISTS qqc_autonomous_decisions (
    app_id TEXT NOT NULL,
    group_id TEXT NOT NULL,
    day DATE NOT NULL,
    reason TEXT NOT NULL,
    count BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY(app_id,group_id,day,reason)
);

CREATE TABLE IF NOT EXISTS qqc_health_snapshots (
    app_id TEXT PRIMARY KEY,
    snapshot JSONB NOT NULL,
    next_check_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS qqc_ranking_config_versions (
    app_id TEXT NOT NULL,
    effective_day DATE NOT NULL,
    config JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY(app_id, effective_day)
);

CREATE TABLE IF NOT EXISTS qqc_ranking_settlements (
    app_id TEXT NOT NULL,
    ranking_day DATE NOT NULL,
    rules JSONB NOT NULL,
    reward_count INTEGER NOT NULL DEFAULT 0,
    settled_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY(app_id, ranking_day)
);

CREATE TABLE IF NOT EXISTS qqc_ranking_claims (
    id BIGSERIAL PRIMARY KEY,
    app_id TEXT NOT NULL,
    ranking_day DATE NOT NULL,
    ranking_kind TEXT NOT NULL CHECK (ranking_kind IN ('tokens','amount')),
    rank INTEGER NOT NULL CHECK (rank BETWEEN 1 AND 10),
    user_id BIGINT NOT NULL REFERENCES users(id),
    source_key TEXT NOT NULL UNIQUE,
    reward JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    claimed_at TIMESTAMPTZ,
    UNIQUE(app_id,ranking_day,ranking_kind,user_id)
);

CREATE TABLE IF NOT EXISTS qqc_periodic_growth_progress (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    task_id TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('usage_count','usage_tokens','usage_amount','recharge_total','checkin_total')),
    consumed NUMERIC NOT NULL DEFAULT 0 CHECK (consumed >= 0),
    cycles BIGINT NOT NULL DEFAULT 0 CHECK (cycles >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY(user_id, task_id, kind)
);

CREATE TABLE IF NOT EXISTS qqc_incoming_images (
    app_id TEXT NOT NULL,
    group_id TEXT NOT NULL,
    message_id TEXT NOT NULL,
    ref_idx TEXT NOT NULL DEFAULT '',
    attachments JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (app_id, group_id, message_id)
);

CREATE TABLE IF NOT EXISTS qqc_moderation_events (
    app_id TEXT NOT NULL,
    sandbox BOOLEAN NOT NULL,
    group_id TEXT NOT NULL,
    message_id TEXT NOT NULL,
    member_id TEXT NOT NULL,
    policy_hash TEXT NOT NULL,
    content_hash TEXT NOT NULL DEFAULT '',
    sent_at TIMESTAMPTZ NOT NULL,
    blocked BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY(app_id,sandbox,group_id,message_id)
);

CREATE TABLE IF NOT EXISTS qqc_moderation_members (
    app_id TEXT NOT NULL,
    sandbox BOOLEAN NOT NULL,
    group_id TEXT NOT NULL,
    member_id TEXT NOT NULL,
    cooldown_until TIMESTAMPTZ NOT NULL DEFAULT 'epoch',
    last_muted_at TIMESTAMPTZ NOT NULL DEFAULT 'epoch',
    active_incident_id TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY(app_id,sandbox,group_id,member_id)
);

CREATE TABLE IF NOT EXISTS qqc_moderation_incidents (
    id TEXT PRIMARY KEY,
    app_id TEXT NOT NULL,
    sandbox BOOLEAN NOT NULL,
    group_id TEXT NOT NULL,
    member_id TEXT NOT NULL,
    policy_hash TEXT NOT NULL,
    reason TEXT NOT NULL,
    mute_seconds INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS qqc_moderation_operations (
    id BIGSERIAL PRIMARY KEY,
    incident_id TEXT NOT NULL REFERENCES qqc_moderation_incidents(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('mute','recall')),
    message_id TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued',
    error TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    UNIQUE(incident_id,kind,message_id)
);

CREATE TABLE IF NOT EXISTS qqc_moderation_rates (
    app_id TEXT NOT NULL,
    sandbox BOOLEAN NOT NULL,
    kind TEXT NOT NULL,
    next_at TIMESTAMPTZ NOT NULL DEFAULT 'epoch',
    PRIMARY KEY(app_id,sandbox,kind)
);

CREATE TABLE IF NOT EXISTS qqc_transfer_history (
    id BIGSERIAL PRIMARY KEY,
    app_id TEXT NOT NULL,
    from_member_id TEXT NOT NULL,
    from_user_id BIGINT NOT NULL,
    to_user_id BIGINT NOT NULL,
    from_email TEXT NOT NULL,
    to_email TEXT NOT NULL,
    amount NUMERIC(18,8) NOT NULL CHECK(amount > 0),
    fee NUMERIC(18,8) NOT NULL CHECK(fee >= 0),
    net_amount NUMERIC(18,8) NOT NULL CHECK(net_amount > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK(amount = fee + net_amount)
);

CREATE TABLE IF NOT EXISTS qqc_pending_image_plans (
    app_id TEXT NOT NULL,
    group_id TEXT NOT NULL,
    member_id TEXT NOT NULL,
    job_id TEXT NOT NULL REFERENCES qqc_jobs(id) ON DELETE CASCADE,
    source_message_id TEXT NOT NULL,
    action TEXT NOT NULL CHECK(action IN ('generate_image','edit_image')),
    prompt TEXT NOT NULL CHECK(char_length(prompt) BETWEEN 1 AND 6000),
    request_quote TEXT NOT NULL CHECK(char_length(request_quote) BETWEEN 1 AND 300),
    expires_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY(app_id,group_id,member_id)
);

CREATE TABLE IF NOT EXISTS qqc_spam_report_sources (
    app_id TEXT NOT NULL,
    sandbox BOOLEAN NOT NULL,
    group_id TEXT NOT NULL,
    message_id TEXT NOT NULL,
    ref_idx TEXT NOT NULL DEFAULT '',
    member_id TEXT NOT NULL,
    member_role TEXT NOT NULL DEFAULT '',
    is_bot BOOLEAN NOT NULL DEFAULT FALSE,
    eligible BOOLEAN NOT NULL DEFAULT FALSE,
    content_encrypted TEXT NOT NULL DEFAULT '',
    content_hash TEXT NOT NULL DEFAULT '',
    policy_hash TEXT NOT NULL,
    sent_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY(app_id,sandbox,group_id,message_id)
);

CREATE TABLE IF NOT EXISTS qqc_spam_reports (
    id TEXT PRIMARY KEY,
    app_id TEXT NOT NULL,
    sandbox BOOLEAN NOT NULL,
    group_id TEXT NOT NULL,
    reporter_id TEXT NOT NULL,
    message_id TEXT NOT NULL,
    member_id TEXT NOT NULL,
    policy_hash TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'evaluating',
    category TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    incident_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    UNIQUE(app_id,sandbox,group_id,message_id)
);

CREATE INDEX IF NOT EXISTS qqc_rewards_user_time ON qqc_rewards(user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS qqc_jobs_queue ON qqc_jobs(created_at) WHERE status = 'queued';

CREATE INDEX IF NOT EXISTS qqc_jobs_member ON qqc_jobs(app_id, group_id, member_id, created_at DESC);

CREATE INDEX IF NOT EXISTS qqc_email_bind_requests_user ON qqc_email_bind_requests(user_id);

CREATE INDEX IF NOT EXISTS qqc_lottery_draws_user_time ON qqc_lottery_draws(user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS qqc_rewards_app_kind_time
    ON qqc_rewards(app_id, kind, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS qqc_rewards_invite_source_member
    ON qqc_rewards(app_id, source_member_id, id) WHERE kind='invite';

CREATE INDEX IF NOT EXISTS qqc_rewards_invite_source_user
    ON qqc_rewards(source_user_id, id) WHERE kind='invite';

CREATE INDEX IF NOT EXISTS qqc_lottery_draws_app_time
    ON qqc_lottery_draws(app_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS qqc_members_invite_time
    ON qqc_members(app_id, (COALESCE(invited_at,first_seen_at)) DESC, group_id, member_id)
    WHERE inviter_id<>'';

CREATE INDEX IF NOT EXISTS qqc_message_references_ref
    ON qqc_message_references(app_id, group_id, ref_idx) WHERE ref_idx <> '';

CREATE INDEX IF NOT EXISTS qqc_message_references_expiry
    ON qqc_message_references(created_at);

CREATE INDEX IF NOT EXISTS idx_qqc_group_messages_recent
    ON qqc_group_messages (app_id, group_id, context_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_qqc_group_messages_expiry
    ON qqc_group_messages (created_at);

CREATE INDEX IF NOT EXISTS qqc_reward_codes_user ON qqc_reward_codes(user_id, id DESC);

CREATE INDEX IF NOT EXISTS qqc_reward_attempts_member ON qqc_reward_attempts(app_id,member_id,created_at DESC) WHERE failed;

CREATE INDEX IF NOT EXISTS qqc_reward_attempts_user ON qqc_reward_attempts(user_id,created_at DESC) WHERE failed;

CREATE INDEX IF NOT EXISTS qqc_jobs_price_dispatch
    ON qqc_jobs(app_id, started_at DESC)
    WHERE event_type = 'PRICE_CHANGE_NOTIFICATION';

CREATE INDEX IF NOT EXISTS qqc_jobs_autonomous_limits ON qqc_jobs(app_id,group_id,reply_reserved_at DESC) WHERE kind='ambient_chat';

CREATE INDEX IF NOT EXISTS qqc_autonomous_seen_expiry ON qqc_autonomous_seen(created_at);

CREATE INDEX IF NOT EXISTS qqc_health_notification_queue ON qqc_jobs(app_id,created_at)
    WHERE event_type='HEALTH_CHANGE_NOTIFICATION' AND status IN ('queued','running');

CREATE UNIQUE INDEX IF NOT EXISTS qqc_reward_codes_source_key ON qqc_reward_codes(source_key);

CREATE INDEX IF NOT EXISTS idx_qqc_ranking_claims_user
    ON qqc_ranking_claims(app_id,user_id,ranking_day DESC,id DESC);

CREATE INDEX IF NOT EXISTS qqc_incoming_images_ref
    ON qqc_incoming_images(app_id, group_id, ref_idx) WHERE ref_idx <> '';

CREATE INDEX IF NOT EXISTS qqc_incoming_images_expiry
    ON qqc_incoming_images(created_at);

CREATE INDEX IF NOT EXISTS qqc_moderation_events_window ON qqc_moderation_events(app_id,sandbox,group_id,member_id,sent_at DESC);

CREATE INDEX IF NOT EXISTS qqc_moderation_events_expiry ON qqc_moderation_events(created_at);

CREATE INDEX IF NOT EXISTS qqc_moderation_incidents_recent ON qqc_moderation_incidents(app_id,sandbox,created_at DESC);

CREATE INDEX IF NOT EXISTS qqc_moderation_operations_pending ON qqc_moderation_operations(kind,id) WHERE status='queued';

CREATE INDEX IF NOT EXISTS qqc_jobs_recent_speakers
    ON qqc_jobs(app_id, created_at DESC, id DESC)
    WHERE event_type IN ('GROUP_AT_MESSAGE_CREATE','GROUP_MESSAGE_CREATE') AND member_id<>'';

CREATE INDEX IF NOT EXISTS qqc_transfer_history_app_time ON qqc_transfer_history(app_id,created_at DESC,id DESC);

CREATE INDEX IF NOT EXISTS qqc_pending_image_plans_expiry ON qqc_pending_image_plans(expires_at);

CREATE INDEX IF NOT EXISTS idx_qqc_group_messages_member
 ON qqc_group_messages (app_id,group_id,member_id,context_at DESC,id DESC);

CREATE INDEX IF NOT EXISTS qqc_spam_report_sources_reference ON qqc_spam_report_sources(app_id,sandbox,group_id,ref_idx) WHERE ref_idx<>'';

CREATE INDEX IF NOT EXISTS qqc_spam_report_sources_expiry ON qqc_spam_report_sources(sent_at);

CREATE INDEX IF NOT EXISTS qqc_spam_report_sources_member ON qqc_spam_report_sources(app_id,sandbox,group_id,member_id,sent_at DESC);

CREATE INDEX IF NOT EXISTS qqc_spam_reports_reporter ON qqc_spam_reports(app_id,sandbox,reporter_id,created_at DESC);

CREATE INDEX IF NOT EXISTS qqc_spam_reports_group ON qqc_spam_reports(app_id,sandbox,group_id,created_at DESC);

CREATE INDEX IF NOT EXISTS qqc_spam_reports_incident ON qqc_spam_reports(incident_id) WHERE incident_id<>'';

CREATE INDEX IF NOT EXISTS qqc_spam_reports_expiry ON qqc_spam_reports(created_at);

CREATE INDEX IF NOT EXISTS idx_qqc_jobs_site_query_member
 ON qqc_jobs(app_id,group_id,member_id,created_at DESC,id DESC)
 WHERE site_query->>'kind' IS NOT NULL;

CREATE TABLE IF NOT EXISTS qqc_transaction_lock (id INTEGER PRIMARY KEY);
INSERT INTO qqc_transaction_lock(id) VALUES(1) ON CONFLICT DO NOTHING;

CREATE UNIQUE INDEX IF NOT EXISTS qqc_checkin_account_day ON qqc_rewards(user_id, ((created_at AT TIME ZONE 'Asia/Shanghai')::date)) WHERE kind='checkin';

CREATE TABLE IF NOT EXISTS qqc_lottery_chance_grants (
 source_key TEXT PRIMARY KEY,
 user_id BIGINT NOT NULL REFERENCES users(id),
 chances INTEGER NOT NULL CHECK(chances BETWEEN 1 AND 1000),
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS qqc_lottery_chance_grants_user ON qqc_lottery_chance_grants(user_id);