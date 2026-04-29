//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveModelMappingNote_PrefersExactThenLongestWildcard(t *testing.T) {
	notes := map[string]string{
		"gpt-5.4": "exact note",
		"gpt-*":   "short wildcard",
		"gpt-5*":  "long wildcard",
	}

	require.Equal(t, "exact note", resolveModelMappingNote(notes, "gpt-5.4"))
	require.Equal(t, "long wildcard", resolveModelMappingNote(notes, "gpt-5-mini"))
	require.Equal(t, "", resolveModelMappingNote(notes, "claude-sonnet-4"))
}

func TestSupportedModels_PropagatesModelMappingNotes(t *testing.T) {
	ch := &Channel{
		FeaturesConfig: map[string]any{
			featureKeyModelMappingNotes: map[string]any{
				"openai": map[string]any{
					"gpt-5.4": "主路由",
					"gpt-4*":  "兼容别名",
				},
			},
		},
		ModelPricing: []ChannelModelPricing{
			{Platform: "openai", Models: []string{"gpt-5.4-mini"}, BillingMode: BillingModeToken},
			{Platform: "openai", Models: []string{"gpt-4o"}, BillingMode: BillingModeToken},
		},
		ModelMapping: map[string]map[string]string{
			"openai": {
				"gpt-5.4": "gpt-5.4-mini",
				"gpt-4*":  "gpt-4o",
			},
		},
	}

	models := ch.SupportedModels()

	notesByName := map[string]string{}
	for _, model := range models {
		notesByName[model.Name] = model.MappingNote
	}

	require.Equal(t, "主路由", notesByName["gpt-5.4"])
	require.Equal(t, "兼容别名", notesByName["gpt-4o"])
	require.Equal(t, "", notesByName["gpt-5.4-mini"])
}
