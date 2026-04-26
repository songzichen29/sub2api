-- MySQL baseline generated from current Ent schema.
CREATE TABLE `security_secrets` (`id` bigint NOT NULL AUTO_INCREMENT, `created_at` datetime(6) NOT NULL, `updated_at` datetime(6) NOT NULL, `key` varchar(100) NOT NULL, `value` longtext NOT NULL, PRIMARY KEY (`id`), UNIQUE INDEX `key` (`key`)) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `payment_audit_logs` (`id` bigint NOT NULL AUTO_INCREMENT, `order_id` varchar(64) NOT NULL, `action` varchar(50) NOT NULL, `detail` longtext NOT NULL, `operator` varchar(100) NOT NULL DEFAULT 'system', `created_at` datetime(6) NOT NULL, PRIMARY KEY (`id`), INDEX `paymentauditlog_order_id` (`order_id`)) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `payment_provider_instances` (`id` bigint NOT NULL AUTO_INCREMENT, `provider_key` varchar(30) NOT NULL, `name` varchar(100) NOT NULL DEFAULT '', `config` longtext NOT NULL, `supported_types` varchar(200) NOT NULL DEFAULT '', `enabled` bool NOT NULL DEFAULT true, `payment_mode` varchar(20) NOT NULL DEFAULT '', `sort_order` bigint NOT NULL DEFAULT 0, `limits` longtext NOT NULL, `refund_enabled` bool NOT NULL DEFAULT false, `allow_user_refund` bool NOT NULL DEFAULT false, `created_at` datetime(6) NOT NULL, `updated_at` datetime(6) NOT NULL, PRIMARY KEY (`id`), INDEX `paymentproviderinstance_provider_key` (`provider_key`), INDEX `paymentproviderinstance_enabled` (`enabled`)) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `proxies` (`id` bigint NOT NULL AUTO_INCREMENT, `created_at` datetime(6) NOT NULL, `updated_at` datetime(6) NOT NULL, `deleted_at` datetime(6) NULL, `name` varchar(100) NOT NULL, `protocol` varchar(20) NOT NULL, `host` varchar(255) NOT NULL, `port` bigint NOT NULL, `username` varchar(100) NULL, `password` varchar(100) NULL, `status` varchar(20) NOT NULL DEFAULT 'active', PRIMARY KEY (`id`), INDEX `proxy_status` (`status`), INDEX `proxy_deleted_at` (`deleted_at`)) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `idempotency_records` (`id` bigint NOT NULL AUTO_INCREMENT, `created_at` datetime(6) NOT NULL, `updated_at` datetime(6) NOT NULL, `scope` varchar(128) NOT NULL, `idempotency_key_hash` varchar(64) NOT NULL, `request_fingerprint` varchar(64) NOT NULL, `status` varchar(32) NOT NULL, `response_status` bigint NULL, `response_body` varchar(255) NULL, `error_reason` varchar(128) NULL, `locked_until` timestamp NULL, `expires_at` timestamp NOT NULL, PRIMARY KEY (`id`), UNIQUE INDEX `idempotencyrecord_scope_idempotency_key_hash` (`scope`, `idempotency_key_hash`), INDEX `idempotencyrecord_expires_at` (`expires_at`), INDEX `idempotencyrecord_status_locked_until` (`status`, `locked_until`)) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `settings` (`id` bigint NOT NULL AUTO_INCREMENT, `key` varchar(100) NOT NULL, `value` longtext NOT NULL, `updated_at` datetime(6) NOT NULL, PRIMARY KEY (`id`), UNIQUE INDEX `key` (`key`)) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `tls_fingerprint_profiles` (`id` bigint NOT NULL AUTO_INCREMENT, `created_at` datetime(6) NOT NULL, `updated_at` datetime(6) NOT NULL, `name` varchar(100) NOT NULL, `description` longtext NULL, `enable_grease` bool NOT NULL DEFAULT false, `cipher_suites` json NULL, `curves` json NULL, `point_formats` json NULL, `signature_algorithms` json NULL, `alpn_protocols` json NULL, `supported_versions` json NULL, `key_share_groups` json NULL, `psk_modes` json NULL, `extensions` json NULL, PRIMARY KEY (`id`), UNIQUE INDEX `name` (`name`)) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `usage_cleanup_tasks` (`id` bigint NOT NULL AUTO_INCREMENT, `created_at` datetime(6) NOT NULL, `updated_at` datetime(6) NOT NULL, `status` varchar(20) NOT NULL, `filters` json NOT NULL, `created_by` bigint NOT NULL, `deleted_rows` bigint NOT NULL DEFAULT 0, `error_message` varchar(255) NULL, `canceled_by` bigint NULL, `canceled_at` timestamp NULL, `started_at` timestamp NULL, `finished_at` timestamp NULL, PRIMARY KEY (`id`), INDEX `usagecleanuptask_status_created_at` (`status`, `created_at`), INDEX `usagecleanuptask_created_at` (`created_at`), INDEX `usagecleanuptask_canceled_at` (`canceled_at`)) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `subscription_plans` (`id` bigint NOT NULL AUTO_INCREMENT, `group_id` bigint NOT NULL, `name` varchar(100) NOT NULL, `description` longtext NOT NULL, `price` decimal(20,2) NOT NULL, `original_price` decimal(20,2) NULL, `validity_days` bigint NOT NULL DEFAULT 30, `validity_unit` varchar(10) NOT NULL DEFAULT 'day', `features` longtext NOT NULL, `product_name` varchar(100) NOT NULL DEFAULT '', `for_sale` bool NOT NULL DEFAULT true, `sort_order` bigint NOT NULL DEFAULT 0, `created_at` datetime(6) NOT NULL, `updated_at` datetime(6) NOT NULL, PRIMARY KEY (`id`), INDEX `subscriptionplan_group_id` (`group_id`), INDEX `subscriptionplan_for_sale` (`for_sale`)) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `error_passthrough_rules` (`id` bigint NOT NULL AUTO_INCREMENT, `created_at` datetime(6) NOT NULL, `updated_at` datetime(6) NOT NULL, `name` varchar(100) NOT NULL, `enabled` bool NOT NULL DEFAULT true, `priority` bigint NOT NULL DEFAULT 0, `error_codes` json NULL, `keywords` json NULL, `match_mode` varchar(10) NOT NULL DEFAULT 'any', `platforms` json NULL, `passthrough_code` bool NOT NULL DEFAULT true, `response_code` bigint NULL, `passthrough_body` bool NOT NULL DEFAULT true, `custom_message` longtext NULL, `skip_monitoring` bool NOT NULL DEFAULT false, `description` longtext NULL, PRIMARY KEY (`id`), INDEX `errorpassthroughrule_enabled` (`enabled`), INDEX `errorpassthroughrule_priority` (`priority`)) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `accounts` (`id` bigint NOT NULL AUTO_INCREMENT, `created_at` datetime(6) NOT NULL, `updated_at` datetime(6) NOT NULL, `deleted_at` datetime(6) NULL, `name` varchar(100) NOT NULL, `notes` longtext NULL, `platform` varchar(50) NOT NULL, `type` varchar(20) NOT NULL, `credentials` json NOT NULL, `extra` json NOT NULL, `concurrency` bigint NOT NULL DEFAULT 3, `load_factor` bigint NULL, `priority` bigint NOT NULL DEFAULT 50, `rate_multiplier` decimal(10,4) NOT NULL DEFAULT 1, `status` varchar(20) NOT NULL DEFAULT 'active', `error_message` longtext NULL, `last_used_at` datetime(6) NULL, `expires_at` datetime(6) NULL, `auto_pause_on_expired` bool NOT NULL DEFAULT true, `schedulable` bool NOT NULL DEFAULT true, `rate_limited_at` datetime(6) NULL, `rate_limit_reset_at` datetime(6) NULL, `overload_until` datetime(6) NULL, `temp_unschedulable_until` datetime(6) NULL, `temp_unschedulable_reason` longtext NULL, `session_window_start` datetime(6) NULL, `session_window_end` datetime(6) NULL, `session_window_status` varchar(20) NULL, `proxy_id` bigint NULL, PRIMARY KEY (`id`), INDEX `account_platform` (`platform`), INDEX `account_type` (`type`), INDEX `account_status` (`status`), INDEX `account_proxy_id` (`proxy_id`), INDEX `account_priority` (`priority`), INDEX `account_last_used_at` (`last_used_at`), INDEX `account_schedulable` (`schedulable`), INDEX `account_rate_limited_at` (`rate_limited_at`), INDEX `account_rate_limit_reset_at` (`rate_limit_reset_at`), INDEX `account_overload_until` (`overload_until`), INDEX `account_platform_priority` (`platform`, `priority`), INDEX `account_priority_status` (`priority`, `status`), INDEX `account_deleted_at` (`deleted_at`), CONSTRAINT `accounts_proxies_proxy` FOREIGN KEY (`proxy_id`) REFERENCES `proxies` (`id`) ON DELETE SET NULL) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `groups` (`id` bigint NOT NULL AUTO_INCREMENT, `created_at` datetime(6) NOT NULL, `updated_at` datetime(6) NOT NULL, `deleted_at` datetime(6) NULL, `name` varchar(100) NOT NULL, `description` longtext NULL, `rate_multiplier` decimal(10,4) NOT NULL DEFAULT 1, `is_exclusive` bool NOT NULL DEFAULT false, `status` varchar(20) NOT NULL DEFAULT 'active', `platform` varchar(50) NOT NULL DEFAULT 'anthropic', `subscription_type` varchar(20) NOT NULL DEFAULT 'standard', `daily_limit_usd` decimal(20,8) NULL, `weekly_limit_usd` decimal(20,8) NULL, `monthly_limit_usd` decimal(20,8) NULL, `default_validity_days` bigint NOT NULL DEFAULT 30, `image_price_1k` decimal(20,8) NULL, `image_price_2k` decimal(20,8) NULL, `image_price_4k` decimal(20,8) NULL, `claude_code_only` bool NOT NULL DEFAULT false, `fallback_group_id` bigint NULL, `fallback_group_id_on_invalid_request` bigint NULL, `model_routing` json NULL, `model_routing_enabled` bool NOT NULL DEFAULT false, `mcp_xml_inject` bool NOT NULL DEFAULT true, `supported_model_scopes` json NOT NULL, `sort_order` bigint NOT NULL DEFAULT 0, `allow_messages_dispatch` bool NOT NULL DEFAULT false, `require_oauth_only` bool NOT NULL DEFAULT false, `require_privacy_set` bool NOT NULL DEFAULT false, `default_mapped_model` varchar(100) NOT NULL DEFAULT '', `messages_dispatch_model_config` json NOT NULL, `rpm_limit` bigint NOT NULL DEFAULT 0, PRIMARY KEY (`id`), INDEX `group_status` (`status`), INDEX `group_platform` (`platform`), INDEX `group_subscription_type` (`subscription_type`), INDEX `group_is_exclusive` (`is_exclusive`), INDEX `group_deleted_at` (`deleted_at`), INDEX `group_sort_order` (`sort_order`)) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `account_groups` (`priority` bigint NOT NULL DEFAULT 50, `created_at` datetime(6) NOT NULL, `account_id` bigint NOT NULL, `group_id` bigint NOT NULL, PRIMARY KEY (`account_id`, `group_id`), INDEX `accountgroup_group_id` (`group_id`), INDEX `accountgroup_priority` (`priority`), CONSTRAINT `account_groups_accounts_account` FOREIGN KEY (`account_id`) REFERENCES `accounts` (`id`) ON DELETE NO ACTION, CONSTRAINT `account_groups_groups_group` FOREIGN KEY (`group_id`) REFERENCES `groups` (`id`) ON DELETE NO ACTION) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `announcements` (`id` bigint NOT NULL AUTO_INCREMENT, `title` varchar(200) NOT NULL, `content` longtext NOT NULL, `status` varchar(20) NOT NULL DEFAULT 'draft', `notify_mode` varchar(20) NOT NULL DEFAULT 'silent', `targeting` json NULL, `starts_at` datetime(6) NULL, `ends_at` datetime(6) NULL, `created_by` bigint NULL, `updated_by` bigint NULL, `created_at` datetime(6) NOT NULL, `updated_at` datetime(6) NOT NULL, PRIMARY KEY (`id`), INDEX `announcement_status` (`status`), INDEX `announcement_created_at` (`created_at`), INDEX `announcement_starts_at` (`starts_at`), INDEX `announcement_ends_at` (`ends_at`)) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `users` (`id` bigint NOT NULL AUTO_INCREMENT, `created_at` datetime(6) NOT NULL, `updated_at` datetime(6) NOT NULL, `deleted_at` datetime(6) NULL, `email` varchar(255) NOT NULL, `password_hash` varchar(255) NOT NULL, `role` varchar(20) NOT NULL DEFAULT 'user', `balance` decimal(20,8) NOT NULL DEFAULT 0, `concurrency` bigint NOT NULL DEFAULT 5, `status` varchar(20) NOT NULL DEFAULT 'active', `username` varchar(100) NOT NULL DEFAULT '', `notes` longtext NOT NULL, `totp_secret_encrypted` longtext NULL, `totp_enabled` bool NOT NULL DEFAULT false, `totp_enabled_at` timestamp NULL, `signup_source` varchar(255) NOT NULL DEFAULT 'email', `last_login_at` datetime(6) NULL, `last_active_at` datetime(6) NULL, `balance_notify_enabled` bool NOT NULL DEFAULT true, `balance_notify_threshold_type` varchar(255) NOT NULL DEFAULT 'fixed', `balance_notify_threshold` decimal(20,8) NULL, `balance_notify_extra_emails` longtext NOT NULL, `total_recharged` decimal(20,8) NOT NULL DEFAULT 0, `rpm_limit` bigint NOT NULL DEFAULT 0, PRIMARY KEY (`id`), INDEX `user_status` (`status`), INDEX `user_deleted_at` (`deleted_at`)) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `announcement_reads` (`id` bigint NOT NULL AUTO_INCREMENT, `read_at` datetime(6) NOT NULL, `created_at` datetime(6) NOT NULL, `announcement_id` bigint NOT NULL, `user_id` bigint NOT NULL, PRIMARY KEY (`id`), INDEX `announcementread_announcement_id` (`announcement_id`), INDEX `announcementread_user_id` (`user_id`), INDEX `announcementread_read_at` (`read_at`), UNIQUE INDEX `announcementread_announcement_id_user_id` (`announcement_id`, `user_id`), CONSTRAINT `announcement_reads_announcements_reads` FOREIGN KEY (`announcement_id`) REFERENCES `announcements` (`id`) ON DELETE NO ACTION, CONSTRAINT `announcement_reads_users_announcement_reads` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE NO ACTION) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `api_keys` (`id` bigint NOT NULL AUTO_INCREMENT, `created_at` datetime(6) NOT NULL, `updated_at` datetime(6) NOT NULL, `deleted_at` datetime(6) NULL, `key` varchar(128) NOT NULL, `name` varchar(100) NOT NULL, `status` varchar(20) NOT NULL DEFAULT 'active', `last_used_at` timestamp NULL, `ip_whitelist` json NULL, `ip_blacklist` json NULL, `quota` decimal(20,8) NOT NULL DEFAULT 0, `quota_used` decimal(20,8) NOT NULL DEFAULT 0, `expires_at` timestamp NULL, `rate_limit_5h` decimal(20,8) NOT NULL DEFAULT 0, `rate_limit_1d` decimal(20,8) NOT NULL DEFAULT 0, `rate_limit_7d` decimal(20,8) NOT NULL DEFAULT 0, `usage_5h` decimal(20,8) NOT NULL DEFAULT 0, `usage_1d` decimal(20,8) NOT NULL DEFAULT 0, `usage_7d` decimal(20,8) NOT NULL DEFAULT 0, `window_5h_start` timestamp NULL, `window_1d_start` timestamp NULL, `window_7d_start` timestamp NULL, `group_id` bigint NULL, `user_id` bigint NOT NULL, PRIMARY KEY (`id`), UNIQUE INDEX `key` (`key`), INDEX `apikey_user_id` (`user_id`), INDEX `apikey_group_id` (`group_id`), INDEX `apikey_status` (`status`), INDEX `apikey_deleted_at` (`deleted_at`), INDEX `apikey_last_used_at` (`last_used_at`), INDEX `apikey_quota_quota_used` (`quota`, `quota_used`), INDEX `apikey_expires_at` (`expires_at`), CONSTRAINT `api_keys_groups_api_keys` FOREIGN KEY (`group_id`) REFERENCES `groups` (`id`) ON DELETE SET NULL, CONSTRAINT `api_keys_users_api_keys` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE NO ACTION) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `auth_identities` (`id` bigint NOT NULL AUTO_INCREMENT, `created_at` datetime(6) NOT NULL, `updated_at` datetime(6) NOT NULL, `provider_type` varchar(20) NOT NULL, `provider_key` varchar(255) NOT NULL, `provider_subject` varchar(255) NOT NULL, `verified_at` datetime(6) NULL, `issuer` longtext NULL, `metadata` json NOT NULL, `user_id` bigint NOT NULL, PRIMARY KEY (`id`), UNIQUE INDEX `authidentity_provider_type_provider_key_provider_subject` (`provider_type`, `provider_key`, `provider_subject`), INDEX `authidentity_user_id` (`user_id`), INDEX `authidentity_user_id_provider_type` (`user_id`, `provider_type`), CONSTRAINT `auth_identities_users_auth_identities` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `auth_identity_channels` (`id` bigint NOT NULL AUTO_INCREMENT, `created_at` datetime(6) NOT NULL, `updated_at` datetime(6) NOT NULL, `provider_type` varchar(20) NOT NULL, `provider_key` varchar(255) NOT NULL, `channel` varchar(20) NOT NULL, `channel_app_id` varchar(128) NOT NULL, `channel_subject` varchar(255) NOT NULL, `metadata` json NOT NULL, `identity_id` bigint NOT NULL, PRIMARY KEY (`id`), UNIQUE INDEX `authidentitychannel_provider_ty_9e5e83a6073c8096ae3b1e15cac03ec6` (`provider_type`, `provider_key`, `channel`, `channel_app_id`, `channel_subject`), INDEX `authidentitychannel_identity_id` (`identity_id`), CONSTRAINT `auth_identity_channels_auth_identities_channels` FOREIGN KEY (`identity_id`) REFERENCES `auth_identities` (`id`) ON DELETE CASCADE) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `channel_monitor_request_templates` (`id` bigint NOT NULL AUTO_INCREMENT, `created_at` datetime(6) NOT NULL, `updated_at` datetime(6) NOT NULL, `name` varchar(100) NOT NULL, `provider` enum('openai','anthropic','gemini') NOT NULL, `description` varchar(500) NULL DEFAULT '', `extra_headers` json NOT NULL, `body_override_mode` varchar(10) NOT NULL DEFAULT 'off', `body_override` json NULL, PRIMARY KEY (`id`), UNIQUE INDEX `channelmonitorrequesttemplate_provider_name` (`provider`, `name`)) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `channel_monitors` (`id` bigint NOT NULL AUTO_INCREMENT, `created_at` datetime(6) NOT NULL, `updated_at` datetime(6) NOT NULL, `name` varchar(100) NOT NULL, `provider` enum('openai','anthropic','gemini') NOT NULL, `endpoint` varchar(500) NOT NULL, `api_key_encrypted` varchar(255) NOT NULL, `primary_model` varchar(200) NOT NULL, `extra_models` json NOT NULL, `group_name` varchar(100) NULL DEFAULT '', `enabled` bool NOT NULL DEFAULT true, `interval_seconds` bigint NOT NULL, `last_checked_at` timestamp NULL, `created_by` bigint NOT NULL, `extra_headers` json NOT NULL, `body_override_mode` varchar(10) NOT NULL DEFAULT 'off', `body_override` json NULL, `template_id` bigint NULL, PRIMARY KEY (`id`), INDEX `channelmonitor_enabled_last_checked_at` (`enabled`, `last_checked_at`), INDEX `channelmonitor_provider` (`provider`), INDEX `channelmonitor_group_name` (`group_name`), INDEX `channelmonitor_template_id` (`template_id`), CONSTRAINT `channel_monitors_channel_monito_a5daa16611505caf79f06364bb25a7f7` FOREIGN KEY (`template_id`) REFERENCES `channel_monitor_request_templates` (`id`) ON DELETE SET NULL) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `channel_monitor_daily_rollups` (`id` bigint NOT NULL AUTO_INCREMENT, `model` varchar(200) NOT NULL, `bucket_date` date NOT NULL, `total_checks` bigint NOT NULL DEFAULT 0, `ok_count` bigint NOT NULL DEFAULT 0, `operational_count` bigint NOT NULL DEFAULT 0, `degraded_count` bigint NOT NULL DEFAULT 0, `failed_count` bigint NOT NULL DEFAULT 0, `error_count` bigint NOT NULL DEFAULT 0, `sum_latency_ms` bigint NOT NULL DEFAULT 0, `count_latency` bigint NOT NULL DEFAULT 0, `sum_ping_latency_ms` bigint NOT NULL DEFAULT 0, `count_ping_latency` bigint NOT NULL DEFAULT 0, `computed_at` timestamp NOT NULL, `monitor_id` bigint NOT NULL, PRIMARY KEY (`id`), UNIQUE INDEX `channelmonitordailyrollup_monitor_id_model_bucket_date` (`monitor_id`, `model`, `bucket_date`), INDEX `channelmonitordailyrollup_bucket_date` (`bucket_date`), CONSTRAINT `channel_monitor_daily_rollups_channel_monitors_daily_rollups` FOREIGN KEY (`monitor_id`) REFERENCES `channel_monitors` (`id`) ON DELETE CASCADE) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `channel_monitor_histories` (`id` bigint NOT NULL AUTO_INCREMENT, `model` varchar(200) NOT NULL, `status` enum('operational','degraded','failed','error') NOT NULL, `latency_ms` bigint NULL, `ping_latency_ms` bigint NULL, `message` varchar(500) NULL DEFAULT '', `checked_at` timestamp NOT NULL, `monitor_id` bigint NOT NULL, PRIMARY KEY (`id`), INDEX `channelmonitorhistory_monitor_id_model_checked_at` (`monitor_id`, `model`, `checked_at`), INDEX `channelmonitorhistory_checked_at` (`checked_at`), CONSTRAINT `channel_monitor_histories_channel_monitors_history` FOREIGN KEY (`monitor_id`) REFERENCES `channel_monitors` (`id`) ON DELETE CASCADE) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `pending_auth_sessions` (`id` bigint NOT NULL AUTO_INCREMENT, `created_at` datetime(6) NOT NULL, `updated_at` datetime(6) NOT NULL, `session_token` varchar(255) NOT NULL, `intent` varchar(40) NOT NULL, `provider_type` varchar(20) NOT NULL, `provider_key` varchar(255) NOT NULL, `provider_subject` varchar(255) NOT NULL, `redirect_to` longtext NOT NULL, `resolved_email` longtext NOT NULL, `registration_password_hash` longtext NOT NULL, `upstream_identity_claims` json NOT NULL, `local_flow_state` json NOT NULL, `browser_session_key` longtext NOT NULL, `completion_code_hash` varchar(255) NOT NULL, `completion_code_expires_at` datetime(6) NULL, `email_verified_at` datetime(6) NULL, `password_verified_at` datetime(6) NULL, `totp_verified_at` datetime(6) NULL, `expires_at` datetime(6) NOT NULL, `consumed_at` datetime(6) NULL, `target_user_id` bigint NULL, PRIMARY KEY (`id`), UNIQUE INDEX `pendingauthsession_session_token` (`session_token`), INDEX `pendingauthsession_target_user_id` (`target_user_id`), INDEX `pendingauthsession_expires_at` (`expires_at`), INDEX `pendingauthsession_provider_type_provider_key_provider_subject` (`provider_type`, `provider_key`, `provider_subject`), INDEX `pendingauthsession_completion_code_hash` (`completion_code_hash`), CONSTRAINT `pending_auth_sessions_users_pending_auth_sessions` FOREIGN KEY (`target_user_id`) REFERENCES `users` (`id`) ON DELETE SET NULL) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `identity_adoption_decisions` (`id` bigint NOT NULL AUTO_INCREMENT, `created_at` datetime(6) NOT NULL, `updated_at` datetime(6) NOT NULL, `adopt_display_name` bool NOT NULL DEFAULT false, `adopt_avatar` bool NOT NULL DEFAULT false, `decided_at` datetime(6) NOT NULL, `identity_id` bigint NULL, `pending_auth_session_id` bigint NOT NULL, PRIMARY KEY (`id`), UNIQUE INDEX `pending_auth_session_id` (`pending_auth_session_id`), UNIQUE INDEX `identityadoptiondecision_pending_auth_session_id` (`pending_auth_session_id`), INDEX `identityadoptiondecision_identity_id` (`identity_id`), CONSTRAINT `identity_adoption_decisions_auth_identities_adoption_decisions` FOREIGN KEY (`identity_id`) REFERENCES `auth_identities` (`id`) ON DELETE SET NULL, CONSTRAINT `identity_adoption_decisions_pen_207f864dd5b2369ca1c49b4476527b1b` FOREIGN KEY (`pending_auth_session_id`) REFERENCES `pending_auth_sessions` (`id`) ON DELETE CASCADE) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `payment_orders` (`id` bigint NOT NULL AUTO_INCREMENT, `user_email` varchar(255) NOT NULL, `user_name` varchar(100) NOT NULL, `user_notes` longtext NULL, `amount` decimal(20,2) NOT NULL, `pay_amount` decimal(20,2) NOT NULL, `fee_rate` decimal(10,4) NOT NULL DEFAULT 0, `recharge_code` varchar(64) NOT NULL, `out_trade_no` varchar(64) NOT NULL DEFAULT '', `payment_type` varchar(30) NOT NULL, `payment_trade_no` varchar(128) NOT NULL, `pay_url` longtext NULL, `qr_code` longtext NULL, `qr_code_img` longtext NULL, `order_type` varchar(20) NOT NULL DEFAULT 'balance', `plan_id` bigint NULL, `subscription_group_id` bigint NULL, `subscription_days` bigint NULL, `provider_instance_id` varchar(64) NULL, `provider_key` varchar(30) NULL, `provider_snapshot` json NULL, `status` varchar(30) NOT NULL DEFAULT 'PENDING', `refund_amount` decimal(20,2) NOT NULL DEFAULT 0, `refund_reason` longtext NULL, `refund_at` datetime(6) NULL, `force_refund` bool NOT NULL DEFAULT false, `refund_requested_at` datetime(6) NULL, `refund_request_reason` longtext NULL, `refund_requested_by` varchar(20) NULL, `expires_at` datetime(6) NOT NULL, `paid_at` datetime(6) NULL, `completed_at` datetime(6) NULL, `failed_at` datetime(6) NULL, `failed_reason` longtext NULL, `client_ip` varchar(50) NOT NULL, `src_host` varchar(255) NOT NULL, `src_url` longtext NULL, `created_at` datetime(6) NOT NULL, `updated_at` datetime(6) NOT NULL, `user_id` bigint NOT NULL, PRIMARY KEY (`id`), UNIQUE INDEX `paymentorder_out_trade_no` (`out_trade_no`), INDEX `paymentorder_user_id` (`user_id`), INDEX `paymentorder_status` (`status`), INDEX `paymentorder_expires_at` (`expires_at`), INDEX `paymentorder_created_at` (`created_at`), INDEX `paymentorder_paid_at` (`paid_at`), INDEX `paymentorder_payment_type_paid_at` (`payment_type`, `paid_at`), INDEX `paymentorder_order_type` (`order_type`), CONSTRAINT `payment_orders_users_payment_orders` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE NO ACTION) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `promo_codes` (`id` bigint NOT NULL AUTO_INCREMENT, `code` varchar(32) NOT NULL, `bonus_amount` decimal(20,8) NOT NULL DEFAULT 0, `max_uses` bigint NOT NULL DEFAULT 0, `used_count` bigint NOT NULL DEFAULT 0, `status` varchar(20) NOT NULL DEFAULT 'active', `expires_at` datetime(6) NULL, `notes` longtext NULL, `created_at` datetime(6) NOT NULL, `updated_at` datetime(6) NOT NULL, PRIMARY KEY (`id`), UNIQUE INDEX `code` (`code`), INDEX `promocode_status` (`status`), INDEX `promocode_expires_at` (`expires_at`)) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `promo_code_usages` (`id` bigint NOT NULL AUTO_INCREMENT, `bonus_amount` decimal(20,8) NOT NULL, `used_at` datetime(6) NOT NULL, `promo_code_id` bigint NOT NULL, `user_id` bigint NOT NULL, PRIMARY KEY (`id`), INDEX `promocodeusage_promo_code_id` (`promo_code_id`), INDEX `promocodeusage_user_id` (`user_id`), UNIQUE INDEX `promocodeusage_promo_code_id_user_id` (`promo_code_id`, `user_id`), CONSTRAINT `promo_code_usages_promo_codes_usage_records` FOREIGN KEY (`promo_code_id`) REFERENCES `promo_codes` (`id`) ON DELETE NO ACTION, CONSTRAINT `promo_code_usages_users_promo_code_usages` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE NO ACTION) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `redeem_codes` (`id` bigint NOT NULL AUTO_INCREMENT, `code` varchar(32) NOT NULL, `type` varchar(20) NOT NULL DEFAULT 'balance', `value` decimal(20,8) NOT NULL DEFAULT 0, `status` varchar(20) NOT NULL DEFAULT 'unused', `used_at` datetime(6) NULL, `notes` longtext NULL, `created_at` datetime(6) NOT NULL, `validity_days` bigint NOT NULL DEFAULT 30, `group_id` bigint NULL, `used_by` bigint NULL, PRIMARY KEY (`id`), UNIQUE INDEX `code` (`code`), INDEX `redeemcode_status` (`status`), INDEX `redeemcode_used_by` (`used_by`), INDEX `redeemcode_group_id` (`group_id`), CONSTRAINT `redeem_codes_groups_redeem_codes` FOREIGN KEY (`group_id`) REFERENCES `groups` (`id`) ON DELETE SET NULL, CONSTRAINT `redeem_codes_users_redeem_codes` FOREIGN KEY (`used_by`) REFERENCES `users` (`id`) ON DELETE SET NULL) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `user_subscriptions` (`id` bigint NOT NULL AUTO_INCREMENT, `created_at` datetime(6) NOT NULL, `updated_at` datetime(6) NOT NULL, `deleted_at` datetime(6) NULL, `starts_at` datetime(6) NOT NULL, `expires_at` datetime(6) NOT NULL, `status` varchar(20) NOT NULL DEFAULT 'active', `daily_window_start` datetime(6) NULL, `weekly_window_start` datetime(6) NULL, `monthly_window_start` datetime(6) NULL, `daily_usage_usd` decimal(20,10) NOT NULL DEFAULT 0, `weekly_usage_usd` decimal(20,10) NOT NULL DEFAULT 0, `monthly_usage_usd` decimal(20,10) NOT NULL DEFAULT 0, `assigned_at` datetime(6) NOT NULL, `notes` longtext NULL, `group_id` bigint NOT NULL, `user_id` bigint NOT NULL, `assigned_by` bigint NULL, PRIMARY KEY (`id`), INDEX `usersubscription_user_id` (`user_id`), INDEX `usersubscription_group_id` (`group_id`), INDEX `usersubscription_status` (`status`), INDEX `usersubscription_expires_at` (`expires_at`), INDEX `usersubscription_user_id_status_expires_at` (`user_id`, `status`, `expires_at`), INDEX `usersubscription_assigned_by` (`assigned_by`), INDEX `usersubscription_user_id_group_id` (`user_id`, `group_id`), INDEX `usersubscription_deleted_at` (`deleted_at`), CONSTRAINT `user_subscriptions_groups_subscriptions` FOREIGN KEY (`group_id`) REFERENCES `groups` (`id`) ON DELETE NO ACTION, CONSTRAINT `user_subscriptions_users_subscriptions` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE NO ACTION, CONSTRAINT `user_subscriptions_users_assigned_subscriptions` FOREIGN KEY (`assigned_by`) REFERENCES `users` (`id`) ON DELETE SET NULL) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `usage_logs` (`id` bigint NOT NULL AUTO_INCREMENT, `request_id` varchar(64) NOT NULL, `model` varchar(100) NOT NULL, `requested_model` varchar(100) NULL, `upstream_model` varchar(100) NULL, `channel_id` bigint NULL, `model_mapping_chain` varchar(500) NULL, `billing_tier` varchar(50) NULL, `billing_mode` varchar(20) NULL, `input_tokens` bigint NOT NULL DEFAULT 0, `output_tokens` bigint NOT NULL DEFAULT 0, `cache_creation_tokens` bigint NOT NULL DEFAULT 0, `cache_read_tokens` bigint NOT NULL DEFAULT 0, `cache_creation_5m_tokens` bigint NOT NULL DEFAULT 0, `cache_creation_1h_tokens` bigint NOT NULL DEFAULT 0, `input_cost` decimal(20,10) NOT NULL DEFAULT 0, `output_cost` decimal(20,10) NOT NULL DEFAULT 0, `cache_creation_cost` decimal(20,10) NOT NULL DEFAULT 0, `cache_read_cost` decimal(20,10) NOT NULL DEFAULT 0, `total_cost` decimal(20,10) NOT NULL DEFAULT 0, `actual_cost` decimal(20,10) NOT NULL DEFAULT 0, `rate_multiplier` decimal(10,4) NOT NULL DEFAULT 1, `account_rate_multiplier` decimal(10,4) NULL, `billing_type` tinyint NOT NULL DEFAULT 0, `stream` bool NOT NULL DEFAULT false, `duration_ms` bigint NULL, `first_token_ms` bigint NULL, `user_agent` varchar(512) NULL, `ip_address` varchar(45) NULL, `image_count` bigint NOT NULL DEFAULT 0, `image_size` varchar(10) NULL, `cache_ttl_overridden` bool NOT NULL DEFAULT false, `created_at` datetime(6) NOT NULL, `api_key_id` bigint NOT NULL, `account_id` bigint NOT NULL, `group_id` bigint NULL, `user_id` bigint NOT NULL, `subscription_id` bigint NULL, PRIMARY KEY (`id`), INDEX `usagelog_user_id` (`user_id`), INDEX `usagelog_api_key_id` (`api_key_id`), INDEX `usagelog_account_id` (`account_id`), INDEX `usagelog_group_id` (`group_id`), INDEX `usagelog_subscription_id` (`subscription_id`), INDEX `usagelog_created_at` (`created_at`), INDEX `usagelog_model` (`model`), INDEX `usagelog_requested_model` (`requested_model`), INDEX `usagelog_request_id` (`request_id`), INDEX `usagelog_user_id_created_at` (`user_id`, `created_at`), INDEX `usagelog_api_key_id_created_at` (`api_key_id`, `created_at`), INDEX `usagelog_group_id_created_at` (`group_id`, `created_at`), CONSTRAINT `usage_logs_api_keys_usage_logs` FOREIGN KEY (`api_key_id`) REFERENCES `api_keys` (`id`) ON DELETE NO ACTION, CONSTRAINT `usage_logs_accounts_usage_logs` FOREIGN KEY (`account_id`) REFERENCES `accounts` (`id`) ON DELETE NO ACTION, CONSTRAINT `usage_logs_groups_usage_logs` FOREIGN KEY (`group_id`) REFERENCES `groups` (`id`) ON DELETE SET NULL, CONSTRAINT `usage_logs_users_usage_logs` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE NO ACTION, CONSTRAINT `usage_logs_user_subscriptions_usage_logs` FOREIGN KEY (`subscription_id`) REFERENCES `user_subscriptions` (`id`) ON DELETE SET NULL) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `user_allowed_groups` (`created_at` datetime(6) NOT NULL, `user_id` bigint NOT NULL, `group_id` bigint NOT NULL, PRIMARY KEY (`user_id`, `group_id`), INDEX `userallowedgroup_group_id` (`group_id`), CONSTRAINT `user_allowed_groups_users_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE NO ACTION, CONSTRAINT `user_allowed_groups_groups_group` FOREIGN KEY (`group_id`) REFERENCES `groups` (`id`) ON DELETE NO ACTION) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `user_attribute_definitions` (`id` bigint NOT NULL AUTO_INCREMENT, `created_at` datetime(6) NOT NULL, `updated_at` datetime(6) NOT NULL, `deleted_at` datetime(6) NULL, `key` varchar(100) NOT NULL, `name` varchar(255) NOT NULL, `description` longtext NOT NULL, `type` varchar(20) NOT NULL, `options` json NOT NULL, `required` bool NOT NULL DEFAULT false, `validation` json NOT NULL, `placeholder` varchar(255) NOT NULL DEFAULT '', `display_order` bigint NOT NULL DEFAULT 0, `enabled` bool NOT NULL DEFAULT true, PRIMARY KEY (`id`), INDEX `userattributedefinition_key` (`key`), INDEX `userattributedefinition_enabled` (`enabled`), INDEX `userattributedefinition_display_order` (`display_order`), INDEX `userattributedefinition_deleted_at` (`deleted_at`)) CHARSET utf8mb4 COLLATE utf8mb4_bin;
CREATE TABLE `user_attribute_values` (`id` bigint NOT NULL AUTO_INCREMENT, `created_at` datetime(6) NOT NULL, `updated_at` datetime(6) NOT NULL, `value` longtext NOT NULL, `user_id` bigint NOT NULL, `attribute_id` bigint NOT NULL, PRIMARY KEY (`id`), UNIQUE INDEX `userattributevalue_user_id_attribute_id` (`user_id`, `attribute_id`), INDEX `userattributevalue_attribute_id` (`attribute_id`), CONSTRAINT `user_attribute_values_users_attribute_values` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE NO ACTION, CONSTRAINT `user_attribute_values_user_attribute_definitions_values` FOREIGN KEY (`attribute_id`) REFERENCES `user_attribute_definitions` (`id`) ON DELETE NO ACTION) CHARSET utf8mb4 COLLATE utf8mb4_bin;

-- ============================================================
-- ops_error_logs: ops 错误日志核心表 (源: 033 + 034/036/038/079)
-- ============================================================
CREATE TABLE `ops_error_logs` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `request_id` varchar(64) NULL,
    `client_request_id` varchar(64) NULL,
    `user_id` bigint NULL,
    `api_key_id` bigint NULL,
    `account_id` bigint NULL,
    `group_id` bigint NULL,
    `client_ip` varchar(45) NULL,
    `platform` varchar(32) NULL,
    `model` varchar(100) NULL,
    `request_path` varchar(256) NULL,
    `stream` bool NOT NULL DEFAULT false,
    `user_agent` longtext NULL,
    `error_phase` varchar(32) NOT NULL,
    `error_type` varchar(64) NOT NULL,
    `severity` varchar(8) NOT NULL DEFAULT 'P2',
    `status_code` int NULL,
    `is_business_limited` bool NOT NULL DEFAULT false,
    `error_message` longtext NULL,
    `error_body` longtext NULL,
    `error_source` varchar(64) NULL,
    `error_owner` varchar(32) NULL,
    `account_status` varchar(50) NULL,
    `upstream_status_code` int NULL,
    `upstream_error_message` longtext NULL,
    `upstream_error_detail` longtext NULL,
    `provider_error_code` varchar(64) NULL,
    `provider_error_type` varchar(64) NULL,
    `network_error_type` varchar(50) NULL,
    `retry_after_seconds` int NULL,
    `duration_ms` int NULL,
    `time_to_first_token_ms` bigint NULL,
    `auth_latency_ms` bigint NULL,
    `routing_latency_ms` bigint NULL,
    `upstream_latency_ms` bigint NULL,
    `response_latency_ms` bigint NULL,
    `request_body` json NULL,
    `request_headers` json NULL,
    `request_body_truncated` bool NOT NULL DEFAULT false,
    `request_body_bytes` int NULL,
    `is_retryable` bool NOT NULL DEFAULT false,
    `retry_count` int NOT NULL DEFAULT 0,
    `upstream_errors` json NULL,
    `is_count_tokens` bool NOT NULL DEFAULT false,
    `resolved` bool NOT NULL DEFAULT false,
    `resolved_at` datetime(6) NULL,
    `resolved_by_user_id` bigint NULL,
    `resolved_retry_id` bigint NULL,
    `inbound_endpoint` varchar(256) NULL,
    `upstream_endpoint` varchar(256) NULL,
    `requested_model` varchar(100) NULL,
    `upstream_model` varchar(100) NULL,
    `request_type` smallint NULL,
    `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    INDEX `idx_ops_error_logs_created_at` (`created_at` DESC),
    INDEX `idx_ops_error_logs_platform_time` (`platform`, `created_at` DESC),
    INDEX `idx_ops_error_logs_group_time` (`group_id`, `created_at` DESC),
    INDEX `idx_ops_error_logs_account_time` (`account_id`, `created_at` DESC),
    INDEX `idx_ops_error_logs_status_time` (`status_code`, `created_at` DESC),
    INDEX `idx_ops_error_logs_phase_time` (`error_phase`, `created_at` DESC),
    INDEX `idx_ops_error_logs_type_time` (`error_type`, `created_at` DESC),
    INDEX `idx_ops_error_logs_request_id` (`request_id`),
    INDEX `idx_ops_error_logs_client_request_id` (`client_request_id`),
    INDEX `idx_ops_error_logs_resolved_time` (`resolved`, `created_at` DESC),
    INDEX `idx_ops_error_logs_is_count_tokens` (`is_count_tokens`)
) CHARSET utf8mb4 COLLATE utf8mb4_bin;

-- ============================================================
-- ops_retry_attempts: 重试审计表 (源: 033 + 038)
-- ============================================================
CREATE TABLE `ops_retry_attempts` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `requested_by_user_id` bigint NULL,
    `source_error_id` bigint NULL,
    `mode` varchar(16) NOT NULL,
    `pinned_account_id` bigint NULL,
    `status` varchar(16) NOT NULL DEFAULT 'queued',
    `started_at` datetime(6) NULL,
    `finished_at` datetime(6) NULL,
    `duration_ms` bigint NULL,
    `result_request_id` varchar(64) NULL,
    `result_error_id` bigint NULL,
    `result_usage_request_id` varchar(64) NULL,
    `error_message` longtext NULL,
    `success` bool NULL,
    `http_status_code` int NULL,
    `upstream_request_id` varchar(128) NULL,
    `used_account_id` bigint NULL,
    `response_preview` longtext NULL,
    `response_truncated` bool NOT NULL DEFAULT false,
    PRIMARY KEY (`id`),
    INDEX `idx_ops_retry_attempts_created_at` (`created_at` DESC),
    INDEX `idx_ops_retry_attempts_source_error` (`source_error_id`, `created_at` DESC),
    INDEX `idx_ops_retry_attempts_success_time` (`success`, `created_at` DESC)
) CHARSET utf8mb4 COLLATE utf8mb4_bin;

-- ============================================================
-- ops_system_metrics: 系统指标快照 (源: 033 + 033扩展 + 042b)
-- ============================================================
CREATE TABLE `ops_system_metrics` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `window_minutes` int NOT NULL DEFAULT 1,
    `platform` varchar(32) NULL,
    `group_id` bigint NULL,
    `success_count` bigint NOT NULL DEFAULT 0,
    `error_count_total` bigint NOT NULL DEFAULT 0,
    `business_limited_count` bigint NOT NULL DEFAULT 0,
    `error_count_sla` bigint NOT NULL DEFAULT 0,
    `upstream_error_count_excl_429_529` bigint NOT NULL DEFAULT 0,
    `upstream_429_count` bigint NOT NULL DEFAULT 0,
    `upstream_529_count` bigint NOT NULL DEFAULT 0,
    `token_consumed` bigint NOT NULL DEFAULT 0,
    `qps` double NULL,
    `tps` double NULL,
    `duration_p50_ms` int NULL,
    `duration_p90_ms` int NULL,
    `duration_p95_ms` int NULL,
    `duration_p99_ms` int NULL,
    `duration_avg_ms` double NULL,
    `duration_max_ms` int NULL,
    `ttft_p50_ms` int NULL,
    `ttft_p90_ms` int NULL,
    `ttft_p95_ms` int NULL,
    `ttft_p99_ms` int NULL,
    `ttft_avg_ms` double NULL,
    `ttft_max_ms` int NULL,
    `cpu_usage_percent` double NULL,
    `memory_used_mb` bigint NULL,
    `memory_total_mb` bigint NULL,
    `memory_usage_percent` double NULL,
    `db_ok` bool NULL,
    `redis_ok` bool NULL,
    `db_conn_active` int NULL,
    `db_conn_idle` int NULL,
    `db_conn_waiting` int NULL,
    `goroutine_count` int NULL,
    `concurrency_queue_depth` int NULL,
    `redis_conn_total` int NULL,
    `redis_conn_idle` int NULL,
    `account_switch_count` bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (`id`),
    INDEX `idx_ops_system_metrics_created_at` (`created_at` DESC),
    INDEX `idx_ops_system_metrics_window_time` (`window_minutes`, `created_at` DESC),
    INDEX `idx_ops_system_metrics_platform_time` (`platform`, `created_at` DESC),
    INDEX `idx_ops_system_metrics_group_time` (`group_id`, `created_at` DESC)
) CHARSET utf8mb4 COLLATE utf8mb4_bin;

-- ============================================================
-- ops_job_heartbeats: 后台任务心跳 (源: 033 + 039)
-- ============================================================
CREATE TABLE `ops_job_heartbeats` (
    `job_name` varchar(64) NOT NULL,
    `last_run_at` datetime(6) NULL,
    `last_success_at` datetime(6) NULL,
    `last_error_at` datetime(6) NULL,
    `last_error` longtext NULL,
    `last_duration_ms` bigint NULL,
    `last_result` longtext NULL,
    `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`job_name`)
) CHARSET utf8mb4 COLLATE utf8mb4_bin;

-- ============================================================
-- ops_alert_rules: 告警规则 (源: 033 + 035)
-- ============================================================
CREATE TABLE `ops_alert_rules` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `name` varchar(128) NOT NULL,
    `description` longtext NULL,
    `enabled` bool NOT NULL DEFAULT true,
    `severity` varchar(16) NOT NULL DEFAULT 'warning',
    `metric_type` varchar(64) NOT NULL,
    `operator` varchar(8) NOT NULL,
    `threshold` double NOT NULL,
    `window_minutes` int NOT NULL DEFAULT 5,
    `sustained_minutes` int NOT NULL DEFAULT 5,
    `cooldown_minutes` int NOT NULL DEFAULT 10,
    `filters` json NULL,
    `last_triggered_at` datetime(6) NULL,
    `notify_email` bool NOT NULL DEFAULT true,
    `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    UNIQUE INDEX `idx_ops_alert_rules_name_unique` (`name`),
    INDEX `idx_ops_alert_rules_enabled` (`enabled`)
) CHARSET utf8mb4 COLLATE utf8mb4_bin;

-- ============================================================
-- ops_alert_events: 告警事件 (源: 033)
-- ============================================================
CREATE TABLE `ops_alert_events` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `rule_id` bigint NULL,
    `severity` varchar(16) NOT NULL,
    `status` varchar(16) NOT NULL DEFAULT 'firing',
    `title` varchar(200) NULL,
    `description` longtext NULL,
    `metric_value` double NULL,
    `threshold_value` double NULL,
    `dimensions` json NULL,
    `fired_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `resolved_at` datetime(6) NULL,
    `email_sent` bool NOT NULL DEFAULT false,
    `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    INDEX `idx_ops_alert_events_rule_status` (`rule_id`, `status`),
    INDEX `idx_ops_alert_events_fired_at` (`fired_at` DESC)
) CHARSET utf8mb4 COLLATE utf8mb4_bin;

-- ============================================================
-- ops_metrics_hourly: 小时级预聚合 (源: 033 vNext + 034 avg/max)
-- ============================================================
CREATE TABLE `ops_metrics_hourly` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `bucket_start` datetime(6) NOT NULL,
    `platform` varchar(32) NULL,
    `group_id` bigint NULL,
    `success_count` bigint NOT NULL DEFAULT 0,
    `error_count_total` bigint NOT NULL DEFAULT 0,
    `business_limited_count` bigint NOT NULL DEFAULT 0,
    `error_count_sla` bigint NOT NULL DEFAULT 0,
    `upstream_error_count_excl_429_529` bigint NOT NULL DEFAULT 0,
    `upstream_429_count` bigint NOT NULL DEFAULT 0,
    `upstream_529_count` bigint NOT NULL DEFAULT 0,
    `token_consumed` bigint NOT NULL DEFAULT 0,
    `duration_p50_ms` int NULL,
    `duration_p90_ms` int NULL,
    `duration_p95_ms` int NULL,
    `duration_p99_ms` int NULL,
    `duration_avg_ms` double NULL,
    `duration_max_ms` int NULL,
    `ttft_p50_ms` int NULL,
    `ttft_p90_ms` int NULL,
    `ttft_p95_ms` int NULL,
    `ttft_p99_ms` int NULL,
    `ttft_avg_ms` double NULL,
    `ttft_max_ms` int NULL,
    `computed_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    UNIQUE INDEX `idx_ops_metrics_hourly_unique_dim` (`bucket_start`, `platform`, `group_id`),
    INDEX `idx_ops_metrics_hourly_bucket` (`bucket_start` DESC),
    INDEX `idx_ops_metrics_hourly_platform_bucket` (`platform`, `bucket_start` DESC),
    INDEX `idx_ops_metrics_hourly_group_bucket` (`group_id`, `bucket_start` DESC)
) CHARSET utf8mb4 COLLATE utf8mb4_bin;

-- ============================================================
-- ops_metrics_daily: 日级预聚合 (源: 033 vNext + 034 avg/max)
-- ============================================================
CREATE TABLE `ops_metrics_daily` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `bucket_date` date NOT NULL,
    `platform` varchar(32) NULL,
    `group_id` bigint NULL,
    `success_count` bigint NOT NULL DEFAULT 0,
    `error_count_total` bigint NOT NULL DEFAULT 0,
    `business_limited_count` bigint NOT NULL DEFAULT 0,
    `error_count_sla` bigint NOT NULL DEFAULT 0,
    `upstream_error_count_excl_429_529` bigint NOT NULL DEFAULT 0,
    `upstream_429_count` bigint NOT NULL DEFAULT 0,
    `upstream_529_count` bigint NOT NULL DEFAULT 0,
    `token_consumed` bigint NOT NULL DEFAULT 0,
    `duration_p50_ms` int NULL,
    `duration_p90_ms` int NULL,
    `duration_p95_ms` int NULL,
    `duration_p99_ms` int NULL,
    `duration_avg_ms` double NULL,
    `duration_max_ms` int NULL,
    `ttft_p50_ms` int NULL,
    `ttft_p90_ms` int NULL,
    `ttft_p95_ms` int NULL,
    `ttft_p99_ms` int NULL,
    `ttft_avg_ms` double NULL,
    `ttft_max_ms` int NULL,
    `computed_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    UNIQUE INDEX `idx_ops_metrics_daily_unique_dim` (`bucket_date`, `platform`, `group_id`),
    INDEX `idx_ops_metrics_daily_bucket` (`bucket_date` DESC),
    INDEX `idx_ops_metrics_daily_platform_bucket` (`platform`, `bucket_date` DESC),
    INDEX `idx_ops_metrics_daily_group_bucket` (`group_id`, `bucket_date` DESC)
) CHARSET utf8mb4 COLLATE utf8mb4_bin;

-- ============================================================
-- ops_alert_silences: 告警静默 (源: 037)
-- ============================================================
CREATE TABLE `ops_alert_silences` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `rule_id` bigint NOT NULL,
    `platform` varchar(64) NOT NULL,
    `group_id` bigint NULL,
    `region` varchar(64) NULL,
    `until` datetime(6) NOT NULL,
    `reason` longtext NULL,
    `created_by` bigint NULL,
    `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    INDEX `idx_ops_alert_silences_lookup` (`rule_id`, `platform`, `group_id`, `region`, `until`)
) CHARSET utf8mb4 COLLATE utf8mb4_bin;

-- ============================================================
-- ops_system_logs: 统一日志 (源: 054)
-- ============================================================
CREATE TABLE `ops_system_logs` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `level` varchar(16) NOT NULL,
    `component` varchar(128) NOT NULL DEFAULT '',
    `message` longtext NOT NULL,
    `request_id` varchar(128) NULL,
    `client_request_id` varchar(128) NULL,
    `user_id` bigint NULL,
    `account_id` bigint NULL,
    `platform` varchar(32) NULL,
    `model` varchar(128) NULL,
    `extra` json NULL,
    PRIMARY KEY (`id`),
    INDEX `idx_ops_system_logs_created_at_id` (`created_at` DESC, `id` DESC),
    INDEX `idx_ops_system_logs_level_created_at` (`level`, `created_at` DESC),
    INDEX `idx_ops_system_logs_component_created_at` (`component`, `created_at` DESC),
    INDEX `idx_ops_system_logs_request_id` (`request_id`),
    INDEX `idx_ops_system_logs_client_request_id` (`client_request_id`),
    INDEX `idx_ops_system_logs_user_id_created_at` (`user_id`, `created_at` DESC),
    INDEX `idx_ops_system_logs_account_id_created_at` (`account_id`, `created_at` DESC),
    INDEX `idx_ops_system_logs_platform_model_created_at` (`platform`, `model`, `created_at` DESC)
) CHARSET utf8mb4 COLLATE utf8mb4_bin;

-- ============================================================
-- ops_system_log_cleanup_audits: 日志清理审计 (源: 054)
-- ============================================================
CREATE TABLE `ops_system_log_cleanup_audits` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `operator_id` bigint NOT NULL,
    `conditions` json NULL,
    `deleted_rows` bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (`id`),
    INDEX `idx_ops_system_log_cleanup_audits_created_at` (`created_at` DESC, `id` DESC)
) CHARSET utf8mb4 COLLATE utf8mb4_bin;

-- ============================================================
-- usage_dashboard_hourly: 使用量仪表盘小时聚合 (源: 034 + 107)
-- ============================================================
CREATE TABLE `usage_dashboard_hourly` (
    `bucket_start` datetime(6) NOT NULL,
    `total_requests` bigint NOT NULL DEFAULT 0,
    `input_tokens` bigint NOT NULL DEFAULT 0,
    `output_tokens` bigint NOT NULL DEFAULT 0,
    `cache_creation_tokens` bigint NOT NULL DEFAULT 0,
    `cache_read_tokens` bigint NOT NULL DEFAULT 0,
    `total_cost` decimal(20, 10) NOT NULL DEFAULT 0,
    `actual_cost` decimal(20, 10) NOT NULL DEFAULT 0,
    `total_duration_ms` bigint NOT NULL DEFAULT 0,
    `active_users` bigint NOT NULL DEFAULT 0,
    `account_cost` decimal(20, 10) NOT NULL DEFAULT 0,
    `computed_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`bucket_start`),
    INDEX `idx_usage_dashboard_hourly_bucket_start` (`bucket_start` DESC)
) CHARSET utf8mb4 COLLATE utf8mb4_bin;

-- ============================================================
-- usage_dashboard_daily: 使用量仪表盘日聚合 (源: 034 + 107)
-- ============================================================
CREATE TABLE `usage_dashboard_daily` (
    `bucket_date` date NOT NULL,
    `total_requests` bigint NOT NULL DEFAULT 0,
    `input_tokens` bigint NOT NULL DEFAULT 0,
    `output_tokens` bigint NOT NULL DEFAULT 0,
    `cache_creation_tokens` bigint NOT NULL DEFAULT 0,
    `cache_read_tokens` bigint NOT NULL DEFAULT 0,
    `total_cost` decimal(20, 10) NOT NULL DEFAULT 0,
    `actual_cost` decimal(20, 10) NOT NULL DEFAULT 0,
    `total_duration_ms` bigint NOT NULL DEFAULT 0,
    `active_users` bigint NOT NULL DEFAULT 0,
    `account_cost` decimal(20, 10) NOT NULL DEFAULT 0,
    `computed_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`bucket_date`),
    INDEX `idx_usage_dashboard_daily_bucket_date` (`bucket_date` DESC)
) CHARSET utf8mb4 COLLATE utf8mb4_bin;

-- ============================================================
-- usage_dashboard_hourly_users: 小时活跃用户去重 (源: 034)
-- ============================================================
CREATE TABLE `usage_dashboard_hourly_users` (
    `bucket_start` datetime(6) NOT NULL,
    `user_id` bigint NOT NULL,
    PRIMARY KEY (`bucket_start`, `user_id`),
    INDEX `idx_usage_dashboard_hourly_users_bucket_start` (`bucket_start`)
) CHARSET utf8mb4 COLLATE utf8mb4_bin;

-- ============================================================
-- usage_dashboard_daily_users: 日活跃用户去重 (源: 034)
-- ============================================================
CREATE TABLE `usage_dashboard_daily_users` (
    `bucket_date` date NOT NULL,
    `user_id` bigint NOT NULL,
    PRIMARY KEY (`bucket_date`, `user_id`),
    INDEX `idx_usage_dashboard_daily_users_bucket_date` (`bucket_date`)
) CHARSET utf8mb4 COLLATE utf8mb4_bin;

-- ============================================================
-- usage_dashboard_aggregation_watermark: 聚合水位标记 (源: 034)
-- ============================================================
CREATE TABLE `usage_dashboard_aggregation_watermark` (
    `id` int NOT NULL,
    `last_aggregated_at` datetime(6) NOT NULL DEFAULT '1970-01-01 00:00:00',
    `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`)
) CHARSET utf8mb4 COLLATE utf8mb4_bin;

-- 初始化水位标记行
INSERT INTO `usage_dashboard_aggregation_watermark` (`id`) VALUES (1);

-- ============================================================
-- scheduler_outbox: 调度发件箱 (源: 036)
-- ============================================================
CREATE TABLE `scheduler_outbox` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `event_type` longtext NOT NULL,
    `account_id` bigint NULL,
    `group_id` bigint NULL,
    `payload` json NULL,
    `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    INDEX `idx_scheduler_outbox_created_at` (`created_at`)
) CHARSET utf8mb4 COLLATE utf8mb4_bin;

-- ============================================================
-- scheduled_test_plans: 定时测试计划 (源: 066 + 070)
-- ============================================================
CREATE TABLE `scheduled_test_plans` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `account_id` bigint NOT NULL,
    `model_id` varchar(100) NOT NULL DEFAULT '',
    `cron_expression` varchar(100) NOT NULL DEFAULT '*/30 * * * *',
    `enabled` bool NOT NULL DEFAULT true,
    `max_results` int NOT NULL DEFAULT 50,
    `auto_recover` bool NOT NULL DEFAULT false,
    `last_run_at` datetime(6) NULL,
    `next_run_at` datetime(6) NULL,
    `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    INDEX `idx_stp_account_id` (`account_id`),
    INDEX `idx_stp_enabled_next_run` (`enabled`, `next_run_at`),
    CONSTRAINT `scheduled_test_plans_accounts_id` FOREIGN KEY (`account_id`) REFERENCES `accounts` (`id`) ON DELETE CASCADE
) CHARSET utf8mb4 COLLATE utf8mb4_bin;

-- ============================================================
-- scheduled_test_results: 定时测试结果 (源: 066)
-- ============================================================
CREATE TABLE `scheduled_test_results` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `plan_id` bigint NOT NULL,
    `status` varchar(20) NOT NULL DEFAULT 'success',
    `response_text` longtext NOT NULL,
    `error_message` longtext NOT NULL,
    `latency_ms` bigint NOT NULL DEFAULT 0,
    `started_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `finished_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    INDEX `idx_str_plan_created` (`plan_id`, `created_at` DESC),
    CONSTRAINT `scheduled_test_results_scheduled_test_plans_id` FOREIGN KEY (`plan_id`) REFERENCES `scheduled_test_plans` (`id`) ON DELETE CASCADE
) CHARSET utf8mb4 COLLATE utf8mb4_bin;

-- ============================================================
-- channels: 渠道管理 (源: 081)
-- ============================================================
CREATE TABLE `channels` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `name` varchar(100) NOT NULL,
    `description` longtext NULL,
    `status` varchar(20) NOT NULL DEFAULT 'active',
    `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    UNIQUE INDEX `idx_channels_name` (`name`),
    INDEX `idx_channels_status` (`status`)
) CHARSET utf8mb4 COLLATE utf8mb4_bin;

-- ============================================================
-- channel_groups: 渠道-分组关联 (源: 081)
-- ============================================================
CREATE TABLE `channel_groups` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `channel_id` bigint NOT NULL,
    `group_id` bigint NOT NULL,
    `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    UNIQUE INDEX `idx_channel_groups_group_id` (`group_id`),
    INDEX `idx_channel_groups_channel_id` (`channel_id`),
    CONSTRAINT `channel_groups_channels_id` FOREIGN KEY (`channel_id`) REFERENCES `channels` (`id`) ON DELETE CASCADE,
    CONSTRAINT `channel_groups_groups_id` FOREIGN KEY (`group_id`) REFERENCES `groups` (`id`) ON DELETE CASCADE
) CHARSET utf8mb4 COLLATE utf8mb4_bin;

-- ============================================================
-- channel_model_pricing: 渠道模型定价 (源: 081)
-- ============================================================
CREATE TABLE `channel_model_pricing` (
    `id` bigint NOT NULL AUTO_INCREMENT,
    `channel_id` bigint NOT NULL,
    `models` json NOT NULL,
    `input_price` decimal(20, 12) NULL,
    `output_price` decimal(20, 12) NULL,
    `cache_write_price` decimal(20, 12) NULL,
    `cache_read_price` decimal(20, 12) NULL,
    `image_output_price` decimal(20, 8) NULL,
    `created_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    INDEX `idx_channel_model_pricing_channel_id` (`channel_id`),
    CONSTRAINT `channel_model_pricing_channels_id` FOREIGN KEY (`channel_id`) REFERENCES `channels` (`id`) ON DELETE CASCADE
) CHARSET utf8mb4 COLLATE utf8mb4_bin;
