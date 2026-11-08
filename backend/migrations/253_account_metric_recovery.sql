CREATE TABLE IF NOT EXISTS account_performance_monitors (
    account_id BIGINT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    config JSONB NOT NULL CHECK (jsonb_typeof(config) = 'object'),
    revision BIGINT NOT NULL DEFAULT 0 CHECK (revision >= 0),
    paused BOOLEAN NOT NULL DEFAULT FALSE,
    pause_generation BIGINT NOT NULL DEFAULT 0 CHECK (pause_generation >= 0),
    paused_at TIMESTAMPTZ,
    reason TEXT NOT NULL DEFAULT '',
    trial_started_at TIMESTAMPTZ,
    trial_requests BIGINT NOT NULL DEFAULT 0 CHECK (trial_requests >= 0),
    next_trial_at TIMESTAMPTZ,
    metrics_since TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS account_performance_events (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    action VARCHAR(16) NOT NULL CHECK (action IN ('configure', 'pause', 'trial', 'recover', 'release', 'retire')),
    actor_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    revision BIGINT NOT NULL,
    pause_generation BIGINT NOT NULL,
    evidence JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_account_performance_events_account_id_id
    ON account_performance_events (account_id, id DESC);
