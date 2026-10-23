ALTER TABLE model_quality_runs
    ADD COLUMN config_id VARCHAR(36) NOT NULL DEFAULT '',
    ADD COLUMN batch_id VARCHAR(36) NOT NULL DEFAULT '',
    ADD COLUMN model VARCHAR(200) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT '',
    ADD COLUMN benchmark VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN execution_started_at DATETIME(6) NULL;

UPDATE model_quality_runs SET
    batch_id = id,
    model = COALESCE(JSON_UNQUOTE(JSON_EXTRACT(target_json, '$.model')), ''),
    benchmark = COALESCE(JSON_UNQUOTE(JSON_EXTRACT(target_json, '$.benchmark')), ''),
    execution_started_at = CASE WHEN status = 'queued' THEN NULL ELSE started_at END;

CREATE INDEX idx_quality_run_batch ON model_quality_runs(plan_id, batch_id, started_at);
CREATE INDEX idx_quality_run_dimensions ON model_quality_runs(plan_id, model, benchmark, started_at);
