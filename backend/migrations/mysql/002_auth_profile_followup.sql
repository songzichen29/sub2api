-- Follow-up parity migration for manual auth/profile tables that are not part of the Ent-generated baseline.

CREATE TABLE IF NOT EXISTS `user_provider_default_grants` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `user_id` bigint NOT NULL,
    `provider_type` varchar(20) NOT NULL,
    `grant_reason` varchar(20) NOT NULL DEFAULT 'first_bind',
    `granted_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    UNIQUE INDEX `user_provider_default_grants_user_provider_reason_key` (`user_id`, `provider_type`, `grant_reason`),
    INDEX `user_provider_default_grants_user_id_idx` (`user_id`),
    CONSTRAINT `user_provider_default_grants_users_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) CHARSET utf8mb4 COLLATE utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `user_avatars` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `user_id` bigint NOT NULL,
    `storage_provider` varchar(20) NOT NULL DEFAULT 'database',
    `storage_key` longtext NOT NULL,
    `url` longtext NOT NULL,
    `content_type` varchar(100) NOT NULL DEFAULT '',
    `byte_size` int NOT NULL DEFAULT 0,
    `sha256` varchar(64) NOT NULL DEFAULT '',
    `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    UNIQUE INDEX `user_avatars_user_id_key` (`user_id`),
    CONSTRAINT `user_avatars_users_avatar` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) CHARSET utf8mb4 COLLATE utf8mb4_bin;

INSERT IGNORE INTO `settings` (`key`, `value`, `updated_at`) VALUES
    ('auth_source_default_email_balance', '0', CURRENT_TIMESTAMP(6)),
    ('auth_source_default_email_concurrency', '5', CURRENT_TIMESTAMP(6)),
    ('auth_source_default_email_subscriptions', '[]', CURRENT_TIMESTAMP(6)),
    ('auth_source_default_email_grant_on_signup', 'false', CURRENT_TIMESTAMP(6)),
    ('auth_source_default_email_grant_on_first_bind', 'false', CURRENT_TIMESTAMP(6)),
    ('auth_source_default_linuxdo_balance', '0', CURRENT_TIMESTAMP(6)),
    ('auth_source_default_linuxdo_concurrency', '5', CURRENT_TIMESTAMP(6)),
    ('auth_source_default_linuxdo_subscriptions', '[]', CURRENT_TIMESTAMP(6)),
    ('auth_source_default_linuxdo_grant_on_signup', 'false', CURRENT_TIMESTAMP(6)),
    ('auth_source_default_linuxdo_grant_on_first_bind', 'false', CURRENT_TIMESTAMP(6)),
    ('auth_source_default_oidc_balance', '0', CURRENT_TIMESTAMP(6)),
    ('auth_source_default_oidc_concurrency', '5', CURRENT_TIMESTAMP(6)),
    ('auth_source_default_oidc_subscriptions', '[]', CURRENT_TIMESTAMP(6)),
    ('auth_source_default_oidc_grant_on_signup', 'false', CURRENT_TIMESTAMP(6)),
    ('auth_source_default_oidc_grant_on_first_bind', 'false', CURRENT_TIMESTAMP(6)),
    ('auth_source_default_wechat_balance', '0', CURRENT_TIMESTAMP(6)),
    ('auth_source_default_wechat_concurrency', '5', CURRENT_TIMESTAMP(6)),
    ('auth_source_default_wechat_subscriptions', '[]', CURRENT_TIMESTAMP(6)),
    ('auth_source_default_wechat_grant_on_signup', 'false', CURRENT_TIMESTAMP(6)),
    ('auth_source_default_wechat_grant_on_first_bind', 'false', CURRENT_TIMESTAMP(6)),
    ('force_email_on_third_party_signup', 'false', CURRENT_TIMESTAMP(6));
