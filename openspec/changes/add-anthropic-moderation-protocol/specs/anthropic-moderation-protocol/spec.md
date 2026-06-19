## ADDED Requirements

### Requirement: Protocol type configuration
The system SHALL support configuring the moderation protocol type with two options: `openai_moderation` (default) and `anthropic_messages`. When the protocol type is changed, the system SHALL use the corresponding API format for content moderation requests.

#### Scenario: Default protocol is OpenAI Moderation
- **WHEN** a user creates a new moderation configuration without specifying protocol type
- **THEN** the system SHALL use `openai_moderation` as the default protocol

#### Scenario: Switch to Anthropic Messages protocol
- **WHEN** a user sets the protocol type to `anthropic_messages` in the configuration
- **THEN** the system SHALL send moderation requests using the Anthropic Messages API format

#### Scenario: Backward compatibility with existing configurations
- **WHEN** an existing configuration does not have the `protocol` field (legacy configuration)
- **THEN** the system SHALL treat it as `openai_moderation` protocol and function identically to before

### Requirement: Anthropic Messages API endpoint and authentication
The system SHALL call the Anthropic Messages API at the configured base URL with endpoint `/v1/messages`. Authentication SHALL use the `x-api-key` header with the configured API key. The system SHALL include the `anthropic-version` header with value `2023-06-01`.

#### Scenario: Successful API call with correct headers
- **WHEN** the system sends a moderation request using `anthropic_messages` protocol
- **THEN** the request SHALL include `x-api-key: {api_key}` header and `anthropic-version: 2023-06-01` header
- **THEN** the request SHALL be sent to `{base_url}/v1/messages`

#### Scenario: Multiple API keys rotation
- **WHEN** multiple API keys are configured and the system needs to make a request
- **THEN** the system SHALL rotate through available keys using the existing round-robin mechanism

### Requirement: System prompt configuration for LLM-based moderation
The system SHALL support configuring a system prompt (up to 4000 characters) that defines the LLM's moderation behavior. The system prompt SHALL instruct the LLM to return a structured JSON response with `flagged` (boolean) and `category_scores` (object with category scores).

#### Scenario: Custom system prompt is used in requests
- **WHEN** a user configures a custom system prompt for `anthropic_messages` protocol
- **THEN** the system SHALL include this prompt in the `system` field of every moderation request

#### Scenario: Default system prompt when not configured
- **WHEN** no custom system prompt is configured and protocol is `anthropic_messages`
- **THEN** the system SHALL use a built-in default prompt that requests JSON output with `flagged` and `category_scores` fields

#### Scenario: System prompt character limit
- **WHEN** a user attempts to save a system prompt exceeding 4000 characters
- **THEN** the system SHALL reject the configuration with an appropriate error message

### Requirement: Anthropic response parsing
The system SHALL parse the Anthropic Messages API response to extract the moderation result. It SHALL locate the `text` content block in the response, parse it as JSON, and extract `flagged` and `category_scores` fields. The system SHALL handle malformed responses gracefully.

#### Scenario: Successful response parsing
- **WHEN** the Anthropic API returns a valid response with a `text` content block containing valid JSON
- **THEN** the system SHALL extract `flagged` (boolean) and `category_scores` (object) from the parsed JSON

#### Scenario: Response with thinking block before text block
- **WHEN** the Anthropic API response contains both `thinking` and `text` content blocks
- **THEN** the system SHALL extract the moderation result from the `text` block, ignoring the `thinking` block

#### Scenario: Malformed JSON in response
- **WHEN** the Anthropic API response contains invalid JSON in the `text` block
- **THEN** the system SHALL log the error and treat the request as a moderation error (allow the request through)

#### Scenario: Missing required fields in response
- **WHEN** the parsed JSON is missing `flagged` or `category_scores` fields
- **THEN** the system SHALL log the error and treat the request as a moderation error

### Requirement: Frontend protocol configuration UI
The frontend SHALL display a protocol type selector in the basic settings tab. When `anthropic_messages` protocol is selected, the system SHALL display a system prompt editor and hide OpenAI-specific options that are not applicable.

#### Scenario: Protocol selector is visible
- **WHEN** a user opens the risk control settings dialog
- **THEN** a protocol type selector SHALL be visible with options: "OpenAI Moderation" and "Anthropic Messages (LLM)"

#### Scenario: System prompt editor shown for Anthropic protocol
- **WHEN** the user selects `anthropic_messages` protocol
- **THEN** the system SHALL display a system prompt textarea editor
- **THEN** the system SHALL display the character count of the current prompt

#### Scenario: Protocol-specific options visibility
- **WHEN** the user selects `anthropic_messages` protocol
- **THEN** the model field SHALL be editable (to support different LLM models like qwen3.7-plus)

### Requirement: API key test for Anthropic protocol
The system SHALL support testing API keys configured for the Anthropic Messages protocol. The test SHALL send a sample moderation request and display the result including the parsed moderation decision.

#### Scenario: Successful API key test
- **WHEN** a user tests an API key configured for `anthropic_messages` protocol
- **THEN** the system SHALL send a test request to the configured endpoint
- **THEN** the system SHALL display the moderation result with category scores

#### Scenario: API key test with custom audit content
- **WHEN** a user provides test prompt text and clicks test
- **THEN** the system SHALL include the custom content in the test request
- **THEN** the system SHALL display the moderation result for that specific content

### Requirement: Input excerpt preservation for blocked content
The system SHALL NOT redact or sanitize user input in the `input_excerpt` field when the content is flagged/blocked. The complete original input MUST be preserved for audit and review purposes, regardless of which moderation protocol is used.

#### Scenario: Blocked content input is not redacted
- **WHEN** a user input is flagged as violating moderation rules (flagged=true)
- **THEN** the `input_excerpt` field SHALL contain the original user input without any redaction
- **THEN** URLs, tokens, API keys, and other sensitive patterns in the input SHALL remain visible

#### Scenario: Non-blocked content input handling
- **WHEN** a user input passes moderation (flagged=false) and is recorded
- **THEN** the system MAY apply redaction to the `input_excerpt` field for privacy protection

#### Scenario: Protocol-independent behavior
- **WHEN** content moderation is performed using either `openai_moderation` or `anthropic_messages` protocol
- **THEN** the input excerpt preservation behavior SHALL be identical for blocked content
