CREATE TABLE IF NOT EXISTS qqc_binding_commands (
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    member_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    request_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    source_job_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL UNIQUE,
    group_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    message_id VARCHAR(1024) NOT NULL,
    nickname VARCHAR(100) NOT NULL DEFAULT '',
    command_text TEXT NOT NULL,
    user_id BIGINT NOT NULL DEFAULT 0,
    delivered BOOLEAN NOT NULL DEFAULT FALSE,
    expires_at DATETIME(6) NOT NULL,
    PRIMARY KEY (app_id, member_id),
    INDEX idx_qqc_binding_commands_expiry (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
