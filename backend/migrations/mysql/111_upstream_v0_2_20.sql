UPDATE `groups` SET allow_image_generation = TRUE
WHERE platform IN ('gemini', 'antigravity') AND allow_image_generation = FALSE;

SET @col_exists = (SELECT COUNT(1) FROM information_schema.columns
  WHERE table_schema = DATABASE() AND table_name = 'groups' AND column_name = 'image_count_billing');
SET @sql = IF(@col_exists = 0,
  'ALTER TABLE `groups` ADD COLUMN `image_count_billing` BOOLEAN NOT NULL DEFAULT FALSE', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

CREATE TABLE IF NOT EXISTS scoped_admin_api_keys (
    id VARCHAR(24) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT NOT NULL,
    key_prefix VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    key_hash VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    expires_at DATETIME(6) NULL,
    allowed_ips JSON NOT NULL DEFAULT (JSON_ARRAY()),
    policy JSON NOT NULL DEFAULT (JSON_OBJECT()),
    created_by BIGINT NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    last_used_at DATETIME(6) NULL,
    last_used_ip VARCHAR(64) NOT NULL DEFAULT '',
    revoked_at DATETIME(6) NULL,
    deleted_at DATETIME(6) NULL,
    CONSTRAINT fk_scoped_admin_key_creator FOREIGN KEY (created_by) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS mcp_oauth_grants (
    id VARCHAR(43) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
    client_id TEXT NOT NULL,
    client_name VARCHAR(100) NOT NULL,
    user_id BIGINT NOT NULL,
    token_version BIGINT NOT NULL,
    resource TEXT NOT NULL,
    scope VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    read_only BOOLEAN NOT NULL DEFAULT TRUE,
    created_at DATETIME(6) NOT NULL,
    expires_at DATETIME(6) NOT NULL,
    last_used_at DATETIME(6) NULL,
    revoked_at DATETIME(6) NULL,
    CONSTRAINT chk_mcp_grant_scope CHECK (scope IN ('user', 'admin')),
    CONSTRAINT fk_mcp_grant_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    INDEX idx_mcp_oauth_grants_user_created (user_id, created_at DESC),
    INDEX idx_mcp_oauth_grants_expiry (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS mcp_oauth_codes (
    code_hash VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
    grant_id VARCHAR(43) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    client_id TEXT NOT NULL,
    redirect_uri TEXT NOT NULL,
    resource TEXT NOT NULL,
    code_challenge VARCHAR(43) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    expires_at DATETIME(6) NOT NULL,
    consumed_at DATETIME(6) NULL,
    CONSTRAINT fk_mcp_code_grant FOREIGN KEY (grant_id) REFERENCES mcp_oauth_grants(id) ON DELETE CASCADE,
    INDEX idx_mcp_oauth_codes_grant (grant_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS mcp_oauth_credentials (
    key_hash VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
    grant_id VARCHAR(43) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    kind VARCHAR(8) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    expires_at DATETIME(6) NOT NULL,
    used_at DATETIME(6) NULL,
    CONSTRAINT chk_mcp_credential_kind CHECK (kind IN ('access', 'refresh')),
    CONSTRAINT fk_mcp_credential_grant FOREIGN KEY (grant_id) REFERENCES mcp_oauth_grants(id) ON DELETE CASCADE,
    INDEX idx_mcp_oauth_credentials_grant (grant_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS account_proxy_selection_lock (
    id INT PRIMARY KEY
) ENGINE=InnoDB;
INSERT IGNORE INTO account_proxy_selection_lock (id) VALUES (1);
