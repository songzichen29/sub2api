UPDATE `accounts`
SET `extra` = JSON_SET(COALESCE(`extra`, JSON_OBJECT()), '$.codex_fingerprint_seed', UUID())
WHERE `deleted_at` IS NULL
  AND `platform` = 'openai'
  AND `type` = 'oauth'
  AND JSON_UNQUOTE(JSON_EXTRACT(`extra`, '$.codex_fingerprint_mode')) IN ('device', 'session', 'full')
  AND (
      JSON_EXTRACT(`extra`, '$.codex_fingerprint_seed') IS NULL
      OR JSON_UNQUOTE(JSON_EXTRACT(`extra`, '$.codex_fingerprint_seed')) = ''
  );
