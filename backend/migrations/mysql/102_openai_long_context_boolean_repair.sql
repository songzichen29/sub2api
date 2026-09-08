UPDATE accounts
SET extra = JSON_SET(COALESCE(extra, JSON_OBJECT()), '$.openai_long_context_billing_enabled', CAST('false' AS JSON)),
    updated_at = NOW(6)
WHERE platform = 'openai' AND parent_account_id IS NULL
  AND COALESCE(JSON_TYPE(JSON_EXTRACT(extra, '$.openai_long_context_billing_enabled')), 'NULL') <> 'BOOLEAN';

UPDATE accounts AS shadow
JOIN accounts AS parent ON parent.id = shadow.parent_account_id
SET shadow.extra = JSON_SET(COALESCE(shadow.extra, JSON_OBJECT()), '$.openai_long_context_billing_enabled',
        CAST(CASE WHEN parent.platform = 'openai'
                       AND JSON_TYPE(JSON_EXTRACT(parent.extra, '$.openai_long_context_billing_enabled')) = 'BOOLEAN'
                  THEN JSON_UNQUOTE(JSON_EXTRACT(parent.extra, '$.openai_long_context_billing_enabled'))
                  ELSE 'false' END AS JSON)),
    shadow.updated_at = NOW(6)
WHERE shadow.platform = 'openai' AND shadow.quota_dimension = 'spark'
  AND (COALESCE(JSON_TYPE(JSON_EXTRACT(shadow.extra, '$.openai_long_context_billing_enabled')), 'NULL') <> 'BOOLEAN'
       OR NOT (JSON_UNQUOTE(JSON_EXTRACT(shadow.extra, '$.openai_long_context_billing_enabled')) <=>
           CASE WHEN parent.platform = 'openai'
                     AND JSON_TYPE(JSON_EXTRACT(parent.extra, '$.openai_long_context_billing_enabled')) = 'BOOLEAN'
                THEN JSON_UNQUOTE(JSON_EXTRACT(parent.extra, '$.openai_long_context_billing_enabled'))
                ELSE 'false' END));
