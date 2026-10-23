CREATE TABLE IF NOT EXISTS model_quality_plans (
    id VARCHAR(36) PRIMARY KEY,
    owner_id BIGINT NOT NULL,
    group_id BIGINT NOT NULL,
    name VARCHAR(160) NOT NULL,
    config_json TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    public_visible BOOLEAN NOT NULL DEFAULT FALSE,
    interval_minutes INT NOT NULL DEFAULT 60,
    next_run_at TIMESTAMPTZ NOT NULL,
    active_run_id VARCHAR(36),
    created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_quality_plan_due ON model_quality_plans(enabled, next_run_at);
CREATE INDEX IF NOT EXISTS idx_quality_plan_group ON model_quality_plans(group_id);

CREATE TABLE IF NOT EXISTS model_quality_runs (
    id VARCHAR(36) PRIMARY KEY,
    plan_id VARCHAR(36) NOT NULL REFERENCES model_quality_plans(id),
    target_json TEXT NOT NULL,
    status VARCHAR(24) NOT NULL,
    verdict VARCHAR(24) NOT NULL,
    phase VARCHAR(24) NOT NULL DEFAULT '',
    reason TEXT NOT NULL,
    result_json TEXT,
    remote_id VARCHAR(256) NOT NULL DEFAULT '',
    attempts INT NOT NULL DEFAULT 0,
    cancel_requested BOOLEAN NOT NULL DEFAULT FALSE,
    lease_token VARCHAR(36) NOT NULL DEFAULT '',
    lease_until TIMESTAMPTZ NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_quality_run_claim ON model_quality_runs(finished_at, lease_until);
CREATE INDEX IF NOT EXISTS idx_quality_run_history ON model_quality_runs(plan_id, started_at);
