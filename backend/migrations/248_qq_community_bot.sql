CREATE TABLE IF NOT EXISTS qq_bot_bindings (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    qq_id VARCHAR(32) NOT NULL UNIQUE,
    group_id VARCHAR(32) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE IF NOT EXISTS qq_bot_bind_requests (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    code_hash CHAR(64) NOT NULL UNIQUE,
    qq_id VARCHAR(32) NOT NULL DEFAULT '',
    group_id VARCHAR(32) NOT NULL DEFAULT '',
    bot_id VARCHAR(32) NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE IF NOT EXISTS qq_bot_rewards (
    id VARCHAR(36) PRIMARY KEY,
    user_id BIGINT NOT NULL,
    qq_id VARCHAR(32) NOT NULL,
    group_id VARCHAR(32) NOT NULL,
    reward_day DATE NOT NULL,
    amount NUMERIC(20,8) NOT NULL,
    balance_after NUMERIC(20,8) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (user_id, reward_day)
);
CREATE INDEX IF NOT EXISTS idx_qq_reward_created ON qq_bot_rewards(created_at);
CREATE TABLE IF NOT EXISTS qq_bot_events (
    id CHAR(64) PRIMARY KEY,
    qq_id VARCHAR(32) NOT NULL,
    group_id VARCHAR(32) NOT NULL,
    command VARCHAR(32) NOT NULL,
    status VARCHAR(16) NOT NULL,
    result_text TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_qq_event_created ON qq_bot_events(created_at);
CREATE TABLE IF NOT EXISTS qq_bot_daily_usage (
    usage_day DATE NOT NULL,
    scope VARCHAR(64) NOT NULL,
    request_count INTEGER NOT NULL DEFAULT 0,
    last_request_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (usage_day, scope)
);
CREATE TABLE IF NOT EXISTS qq_bot_conversations (
    qq_id VARCHAR(32) NOT NULL,
    group_id VARCHAR(32) NOT NULL,
    messages_json TEXT NOT NULL,
    lease_token CHAR(64) NOT NULL DEFAULT '',
    lease_until TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (qq_id, group_id)
);
