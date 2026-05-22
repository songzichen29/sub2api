-- 018_seed_openai_monitor_templates.sql
-- MySQL counterpart for PostgreSQL migration 139_seed_openai_monitor_templates.sql.
-- Seeds OpenAI channel monitor templates without overwriting user-edited rows.

INSERT IGNORE INTO `channel_monitor_request_templates` (
    `created_at`, `updated_at`, `name`, `provider`, `api_mode`, `description`,
    `extra_headers`, `body_override_mode`, `body_override`
)
VALUES
(
    NOW(6),
    NOW(6),
    'OpenAI Compatible 默认检测',
    'openai',
    'chat_completions',
    '适用于大多数 OpenAI-compatible 上游：POST /v1/chat/completions，后端自动生成 messages 数学 challenge。',
    '{}',
    'off',
    NULL
),
(
    NOW(6),
    NOW(6),
    'OpenAI Compatible 低 token 检测',
    'openai',
    'chat_completions',
    '仍走 /v1/chat/completions，仅把 max_tokens 调低，model/messages/stream 由后端保护。',
    '{}',
    'merge',
    '{"max_tokens": 20}'
),
(
    NOW(6),
    NOW(6),
    'OpenAI Responses / 本站自检',
    'openai',
    'responses',
    '适用于本站或原生 Responses API：POST /v1/responses，默认 payload 自动带 instructions 与 input。',
    '{}',
    'off',
    NULL
),
(
    NOW(6),
    NOW(6),
    'OpenAI Responses 低 token 检测',
    'openai',
    'responses',
    '仍走 /v1/responses，仅把 max_output_tokens 调低，instructions/input/model/stream 由后端保护。',
    '{}',
    'merge',
    '{"max_output_tokens": 20}'
);
