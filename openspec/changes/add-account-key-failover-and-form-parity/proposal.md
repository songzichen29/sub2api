# Ordered account API keys and account form parity

## Why

An upstream account currently accepts one API key. Administrators need an ordered
set of keys for the same provider/endpoints: use the first usable key, fall through
on credential or quota failures, and return to earlier keys when they recover.
The create modal also omits configurable features available in the edit modal.

## What Changes

- Opt-in ordered API keys on API-key accounts, with existing single-key behavior preserved.
- Safe retries before response delivery, bounded attempts, recovery of earlier keys,
  and temporary rather than permanent account suspension when every key fails.
- Secret-safe create/edit handling: never return stored keys, preserve them on
  unrelated edits, explicitly replace the ordered list when new keys are submitted.
- Audit and align create/edit configuration capabilities, sharing controls and
  serialization where practical. Runtime observations remain edit-only.
- Regression tests for selection, recovery, exhaustion, replay safety, credential
  redaction, persistence and form payload parity.

## Capabilities

- New capability: `ordered-account-api-keys`.
- Modified capability: account creation/editing configuration parity.

## Impact

Account credentials/DTOs, upstream transport and error handling, provider request
integration, account forms and tests. No database schema migration is required.
Existing production accounts are not automatically converted or reconfigured.
