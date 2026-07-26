package admin

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestDiffSettingsOpenAIFreeImageBridgeDoesNotExposeSecret(t *testing.T) {
	before := &service.SystemSettings{
		OpenAIFreeImageBridgeURL:     "http://old-bridge:8787",
		OpenAIFreeImageBridgeAuthKey: "old-secret",
	}
	after := &service.SystemSettings{
		OpenAIFreeImageBridgeURL:     "http://new-bridge:8787",
		OpenAIFreeImageBridgeAuthKey: "new-secret",
	}

	changed := diffSettings(before, after, nil, nil, UpdateSettingsRequest{
		OpenAIFreeImageBridgeAuthKey: "new-secret",
	})

	require.Contains(t, changed, "openai_free_image_bridge_url")
	require.Contains(t, changed, "openai_free_image_bridge_auth_key")
	require.NotContains(t, changed, "new-secret")
}
