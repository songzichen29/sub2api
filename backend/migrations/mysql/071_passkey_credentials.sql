CREATE TABLE IF NOT EXISTS `passkey_user_handles` (
    `user_id` BIGINT NOT NULL,
    `user_handle` VARBINARY(64) NOT NULL,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`user_id`),
    UNIQUE KEY `uq_passkey_user_handles_handle` (`user_handle`),
    CONSTRAINT `fk_passkey_user_handles_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `passkey_credentials` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT NOT NULL,
    `credential_id` VARBINARY(1024) NOT NULL,
    `name` VARCHAR(100) NOT NULL DEFAULT 'Passkey',
    `credential_data` JSON NOT NULL,
    `last_used_at` DATETIME(6) NULL,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    UNIQUE KEY `uq_passkey_credentials_credential_id` (`credential_id`),
    KEY `idx_passkey_credentials_user_id` (`user_id`),
    KEY `idx_passkey_credentials_last_used_at` (`last_used_at`),
    CONSTRAINT `fk_passkey_credentials_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
