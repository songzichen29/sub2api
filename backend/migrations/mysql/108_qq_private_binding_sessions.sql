CREATE TABLE IF NOT EXISTS qqc_private_bind_sessions (
    app_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    member_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    request_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    group_id VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    nickname VARCHAR(100) NOT NULL DEFAULT '',
    expires_at DATETIME(6) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (app_id, member_id),
    INDEX idx_qqc_private_bind_sessions_expiry (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
