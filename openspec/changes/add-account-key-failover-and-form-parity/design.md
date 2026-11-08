# Design

## Credential contract

Store an ordered `credentials.api_keys` string array and keep `api_key` equal to
its first entry for compatibility. Validate count, key length, duplicates and
control characters without echoing credentials in errors. Pool keys are sensitive
for DTOs, audit logs and partial credential updates. Return only a safe key count.
Submitting no new authentication fields preserves the list; submitting a single
replacement key removes the old pool. OAuth and service-account credentials are
not silently converted to API-key pools.

## Selection and failure boundaries

Each independent request starts at the first key; a successful earlier key becomes
preferred immediately, without background model requests. Retry only definitive
upstream failures before delivering any response bytes. Do not replay transport
failures of ambiguous acceptance, successful streams, invalid input, or content
policy refusals. Attempts are bounded by the configured list. Clone headers and
request bodies per attempt and close rejected responses. Preserve TLS/proxy,
billing, admission and account identity. Requests without a pool use the original
transport path unchanged.

An exhausted pool must not become a permanently disabled account due to the final
key's authentication failure. Preserve administrator restrictions and use a short
recoverable scheduling cooldown. Provider-specific stateful/async/WS ownership
must be audited before the feature is declared complete; do not silently send an
existing resource or continuation under an unrelated key.

## Form parity

Inventory both modal controls and submitted payloads. Add missing configuration
for account scheduling thresholds, applicable OpenAI quota/credit/plan controls,
and Grok controls. Reuse existing labels, eligibility rules and backend validators.
Do not fabricate live usage, history or actions that require a persisted account.
Keep original defaults unless the administrator explicitly changes a value.

Ordinary API-key creation and provider-specific creation share the extra builder
for quota, reset/notification and usage-query settings. An enabled NewAPI query
with missing required fields blocks creation; Sub2API reuses account credentials.
Enabled third-party queries take priority over built-in platform usage eligibility.
The third-party usage-query panel shows only the used percentage when the total
is known, without adding a progress bar or monetary duplicate; native platform
usage windows retain their existing bars. Error retry and explicit refresh
remain available; unknown totals retain their balance without a fabricated
ratio.

## Tooling

The OpenSpec CLI is unavailable in this environment. Artifacts use the repository's
existing spec-driven format; implementation and evidence are tracked in tasks.md.
