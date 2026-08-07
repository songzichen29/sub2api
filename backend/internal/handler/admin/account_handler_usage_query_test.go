package admin

import "testing"

func TestEncryptUsageQueryTokenSub2APIReusesAccountCredentials(t *testing.T) {
	extra := map[string]any{
		"usage_query": map[string]any{
			"enabled":      true,
			"provider":     "sub2api",
			"base_url":     "https://duplicated.example/v1",
			"access_token": "must-not-be-stored",
			"user_id":      "unused",
		},
	}

	if err := encryptUsageQueryToken(extra, nil, nil); err != nil {
		t.Fatalf("encryptUsageQueryToken() error = %v", err)
	}
	uq, ok := extra["usage_query"].(map[string]any)
	if !ok {
		t.Fatal("usage_query was removed")
	}
	if uq["enabled"] != true || uq["provider"] != "sub2api" {
		t.Fatalf("usage_query identity fields = %#v", uq)
	}
	for _, key := range []string{"base_url", "access_token", "user_id"} {
		if _, exists := uq[key]; exists {
			t.Errorf("usage_query.%s must not be persisted for sub2api", key)
		}
	}
}
