CREATE TABLE IF NOT EXISTS model_quality_plans (
    id VARCHAR(36) NOT NULL PRIMARY KEY,
    owner_id BIGINT NOT NULL,
    group_id BIGINT NOT NULL,
    name VARCHAR(160) NOT NULL,
    config_json LONGTEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    public_visible BOOLEAN NOT NULL DEFAULT FALSE,
    interval_minutes INT NOT NULL DEFAULT 60,
    next_run_at DATETIME(6) NOT NULL,
    active_run_id VARCHAR(36) NULL,
    created_at DATETIME(6) NOT NULL,
    INDEX idx_quality_plan_due (enabled, next_run_at),
    INDEX idx_quality_plan_group (group_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS model_quality_runs (
    id VARCHAR(36) NOT NULL PRIMARY KEY,
    plan_id VARCHAR(36) NOT NULL,
    target_json LONGTEXT NOT NULL,
    status VARCHAR(24) NOT NULL,
    verdict VARCHAR(24) NOT NULL,
    phase VARCHAR(24) NOT NULL DEFAULT '',
    reason TEXT NOT NULL,
    result_json LONGTEXT NULL,
    remote_id VARCHAR(256) NOT NULL DEFAULT '',
    attempts INT NOT NULL DEFAULT 0,
    cancel_requested BOOLEAN NOT NULL DEFAULT FALSE,
    lease_token VARCHAR(36) NOT NULL DEFAULT '',
    lease_until DATETIME(6) NOT NULL,
    started_at DATETIME(6) NOT NULL,
    finished_at DATETIME(6) NULL,
    INDEX idx_quality_run_claim (finished_at, lease_until),
    INDEX idx_quality_run_history (plan_id, started_at),
    CONSTRAINT fk_quality_run_plan FOREIGN KEY (plan_id) REFERENCES model_quality_plans(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
