CREATE TABLE IF NOT EXISTS qqc_private_bind_sessions (
    app_id TEXT NOT NULL,
    member_id TEXT NOT NULL,
    request_id TEXT NOT NULL,
    group_id TEXT NOT NULL,
    nickname TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (app_id, member_id)
);

CREATE INDEX IF NOT EXISTS idx_qqc_private_bind_sessions_expiry ON qqc_private_bind_sessions(expires_at);
