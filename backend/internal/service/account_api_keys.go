package service

import (
	"strings"
	"unicode"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const MaxAccountAPIKeys = 20

// NormalizeAccountAPIKeys validates the opt-in ordered pool. api_key remains the
// primary credential for legacy integrations; neither errors nor DTOs expose keys.
func NormalizeAccountAPIKeys(accountType string, credentials map[string]any) error {
	raw, present := credentials["api_keys"]
	if !present {
		return nil
	}
	invalid := func(message string) error {
		return infraerrors.BadRequest("INVALID_ACCOUNT_API_KEYS", message)
	}
	if accountType != AccountTypeAPIKey {
		return invalid("ordered API keys are only supported on API-key accounts")
	}
	var keys []string
	switch values := raw.(type) {
	case []string:
		keys = append([]string(nil), values...)
	case []any:
		keys = make([]string, len(values))
		for i, value := range values {
			key, ok := value.(string)
			if !ok {
				return invalid("each API key must be a string")
			}
			keys[i] = key
		}
	default:
		return invalid("api_keys must be an ordered array of strings")
	}
	if len(keys) == 0 || len(keys) > MaxAccountAPIKeys {
		return invalid("provide between 1 and 20 API keys")
	}
	seen := make(map[string]struct{}, len(keys))
	for i, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" || len(key) > 8192 || strings.IndexFunc(key, func(r rune) bool {
			return unicode.IsSpace(r) || unicode.IsControl(r)
		}) >= 0 {
			return invalid("API keys must be non-empty, at most 8192 bytes and contain no whitespace or control characters")
		}
		if _, duplicate := seen[key]; duplicate {
			return invalid("duplicate API keys are not allowed")
		}
		seen[key] = struct{}{}
		keys[i] = key
	}
	credentials["api_key"] = keys[0]
	if len(keys) == 1 {
		delete(credentials, "api_keys")
	} else {
		credentials["api_keys"] = keys
	}
	return nil
}

// OrderedAPIKeys returns a private copy. Callers must never mutate shared account
// snapshots to select a key for a request.
func (a *Account) OrderedAPIKeys() []string {
	if a == nil || a.Type != AccountTypeAPIKey {
		return nil
	}
	var keys []string
	switch values := a.Credentials["api_keys"].(type) {
	case []string:
		keys = append([]string(nil), values...)
	case []any:
		for _, value := range values {
			key, ok := value.(string)
			if !ok {
				return nil
			}
			keys = append(keys, key)
		}
	}
	if len(keys) > 0 && len(keys) <= MaxAccountAPIKeys {
		return keys
	}
	if key := strings.TrimSpace(a.GetCredential("api_key")); key != "" {
		return []string{key}
	}
	return nil
}

func (a *Account) HasAPIKeyPool() bool {
	return len(a.OrderedAPIKeys()) > 1
}
