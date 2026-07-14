UPDATE accounts
SET extra = JSON_SET(COALESCE(extra, JSON_OBJECT()), '$.openai_long_context_billing_enabled', false),
    updated_at = NOW()
WHERE platform = 'openai'
  AND parent_account_id IS NULL
  AND (
        extra IS NULL
        OR JSON_EXTRACT(extra, '$.openai_long_context_billing_enabled') IS NULL
        OR JSON_TYPE(JSON_EXTRACT(extra, '$.openai_long_context_billing_enabled')) <> 'BOOLEAN'
      );

UPDATE accounts AS shadow
JOIN accounts AS parent ON parent.id = shadow.parent_account_id
SET shadow.extra = JSON_SET(
        COALESCE(shadow.extra, JSON_OBJECT()),
        '$.openai_long_context_billing_enabled',
        COALESCE(
            CASE
                WHEN parent.platform <> 'openai' THEN false
                WHEN JSON_TYPE(JSON_EXTRACT(parent.extra, '$.openai_long_context_billing_enabled')) = 'BOOLEAN'
                    THEN JSON_EXTRACT(parent.extra, '$.openai_long_context_billing_enabled')
                ELSE false
            END,
            false
        )
    ),
    shadow.updated_at = NOW()
WHERE shadow.platform = 'openai'
  AND shadow.quota_dimension = 'spark'
  AND (
        shadow.extra IS NULL
        OR JSON_EXTRACT(shadow.extra, '$.openai_long_context_billing_enabled') IS NULL
        OR JSON_TYPE(JSON_EXTRACT(shadow.extra, '$.openai_long_context_billing_enabled')) <> 'BOOLEAN'
        OR JSON_EXTRACT(shadow.extra, '$.openai_long_context_billing_enabled') <> COALESCE(
            CASE
                WHEN parent.platform <> 'openai' THEN CAST('false' AS JSON)
                WHEN JSON_TYPE(JSON_EXTRACT(parent.extra, '$.openai_long_context_billing_enabled')) = 'BOOLEAN'
                    THEN JSON_EXTRACT(parent.extra, '$.openai_long_context_billing_enabled')
                ELSE CAST('false' AS JSON)
            END,
            CAST('false' AS JSON)
        )
      );
