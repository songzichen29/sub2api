CREATE TABLE IF NOT EXISTS qq_bot_bindings (
    user_id BIGINT NOT NULL PRIMARY KEY,
    qq_id VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL UNIQUE,
    group_id VARCHAR(32) NOT NULL,
    created_at DATETIME(6) NOT NULL,
    CONSTRAINT fk_qq_binding_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qq_bot_bind_requests (
    user_id BIGINT NOT NULL PRIMARY KEY,
    code_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL UNIQUE,
    qq_id VARCHAR(32) NOT NULL DEFAULT '',
    group_id VARCHAR(32) NOT NULL DEFAULT '',
    bot_id VARCHAR(32) NOT NULL DEFAULT '',
    expires_at DATETIME(6) NOT NULL,
    CONSTRAINT fk_qq_request_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qq_bot_rewards (
    id VARCHAR(36) NOT NULL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    qq_id VARCHAR(32) NOT NULL,
    group_id VARCHAR(32) NOT NULL,
    reward_day DATE NOT NULL,
    amount DECIMAL(20,8) NOT NULL,
    balance_after DECIMAL(20,8) NOT NULL,
    created_at DATETIME(6) NOT NULL,
    UNIQUE KEY uq_qq_reward_user_day (user_id, reward_day),
    INDEX idx_qq_reward_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qq_bot_events (
    id CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
    qq_id VARCHAR(32) NOT NULL,
    group_id VARCHAR(32) NOT NULL,
    command VARCHAR(32) NOT NULL,
    status VARCHAR(16) NOT NULL,
    result_text TEXT NOT NULL,
    created_at DATETIME(6) NOT NULL,
    finished_at DATETIME(6) NULL,
    INDEX idx_qq_event_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qq_bot_daily_usage (
    usage_day DATE NOT NULL,
    scope VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    request_count INT NOT NULL DEFAULT 0,
    last_request_at DATETIME(6) NOT NULL,
    PRIMARY KEY (usage_day, scope)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS qq_bot_conversations (
    qq_id VARCHAR(32) NOT NULL,
    group_id VARCHAR(32) NOT NULL,
    messages_json LONGTEXT NOT NULL,
    lease_token CHAR(64) NOT NULL DEFAULT '',
    lease_until DATETIME(6) NOT NULL,
    updated_at DATETIME(6) NOT NULL,
    PRIMARY KEY (qq_id, group_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
