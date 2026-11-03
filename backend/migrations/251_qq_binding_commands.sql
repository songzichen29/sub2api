CREATE TABLE IF NOT EXISTS qqc_binding_commands (
    app_id TEXT NOT NULL,
    member_id TEXT NOT NULL,
    request_id TEXT NOT NULL,
    source_job_id TEXT NOT NULL UNIQUE,
    group_id TEXT NOT NULL,
    message_id TEXT NOT NULL,
    nickname TEXT NOT NULL DEFAULT '',
    command_text TEXT NOT NULL,
    user_id BIGINT NOT NULL DEFAULT 0,
    delivered BOOLEAN NOT NULL DEFAULT FALSE,
    expires_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (app_id, member_id)
);
CREATE INDEX IF NOT EXISTS idx_qqc_binding_commands_expiry ON qqc_binding_commands(expires_at);
