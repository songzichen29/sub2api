package service

import (
	"context"
	"testing"
	"time"

	gocache "github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/require"
)

func TestNormalizeOpenAIMessagesDispatchModelConfig(t *testing.T) {
	t.Parallel()

	cfg := normalizeOpenAIMessagesDispatchModelConfig(OpenAIMessagesDispatchModelConfig{
		OpusMappedModel:   " gpt-5.4-high ",
		SonnetMappedModel: "gpt-5.3-codex",
		HaikuMappedModel:  " gpt-5.4-mini-medium ",
		ExactModelMappings: map[string]string{
			" claude-sonnet-4-5-20250929 ": " gpt-5.2-high ",
			"":                             "gpt-5.4",
			"claude-opus-4-6":              " ",
		},
	})

	require.Equal(t, "gpt-5.4", cfg.OpusMappedModel)
	require.Equal(t, "gpt-5.3-codex", cfg.SonnetMappedModel)
	require.Equal(t, "gpt-5.4-mini", cfg.HaikuMappedModel)
	require.Equal(t, map[string]string{
		"claude-sonnet-4-5-20250929": "gpt-5.2",
	}, cfg.ExactModelMappings)
}

func TestBuildMessagesDispatchModelCandidates(t *testing.T) {
	t.Parallel()

	group := &Group{
		MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{
			HaikuMappedModel: " gpt-5.4-mini-high ",
			ExactModelMappings: map[string]string{
				"claude-haiku-4-5-20251001": " gpt-5.2-high ",
			},
		},
	}

	candidates := buildMessagesDispatchModelCandidates(group, "claude-haiku-4-5-20251001")
	require.Equal(t,
		[]string{"gpt-5.2", "gpt-5.4-mini", "gpt-5.4", "gpt-5.3-codex", "gpt-5.5"},
		candidates,
	)
}

func TestResolveMessagesDispatchMappedModel_FallsBackToAllowedModelFromCandidates(t *testing.T) {
	t.Parallel()

	svc := &OpenAIGatewayService{
		modelsListCache: gocache.New(time.Minute, time.Minute),
	}
	groupID := int64(9)
	storeAvailableModelsSnapshot(svc.modelsListCache, 0, &groupID, PlatformOpenAI, availableModelsSnapshot{
		Models:      []string{"gpt-5.4", "gpt-5.3-codex"},
		Restrictive: true,
	})

	group := &Group{
		MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{
			HaikuMappedModel: "gpt-5.4-mini",
		},
	}

	got := svc.ResolveMessagesDispatchMappedModel(context.TODO(), &groupID, group, "claude-haiku-4-5-20251001")
	require.Equal(t, "gpt-5.4", got)
}

func TestResolveMessagesDispatchMappedModel_PrefersConfiguredAllowedModel(t *testing.T) {
	t.Parallel()

	svc := &OpenAIGatewayService{
		modelsListCache: gocache.New(time.Minute, time.Minute),
	}
	groupID := int64(10)
	storeAvailableModelsSnapshot(svc.modelsListCache, 0, &groupID, PlatformOpenAI, availableModelsSnapshot{
		Models:      []string{"gpt-5.4", "gpt-5.4-mini"},
		Restrictive: true,
	})

	group := &Group{
		MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{
			HaikuMappedModel: "gpt-5.4-mini",
		},
	}

	got := svc.ResolveMessagesDispatchMappedModel(context.TODO(), &groupID, group, "claude-haiku-4-5-20251001")
	require.Equal(t, "gpt-5.4-mini", got)
}

func TestResolveMessagesDispatchMappedModel_SkipsImageModels(t *testing.T) {
	t.Parallel()

	svc := &OpenAIGatewayService{
		modelsListCache: gocache.New(time.Minute, time.Minute),
	}
	groupID := int64(11)
	storeAvailableModelsSnapshot(svc.modelsListCache, 0, &groupID, PlatformOpenAI, availableModelsSnapshot{
		Models:      []string{"gpt-image-2", "gpt-5.3-codex"},
		Restrictive: true,
	})

	group := &Group{
		MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{
			HaikuMappedModel: "gpt-image-2",
		},
	}

	got := svc.ResolveMessagesDispatchMappedModel(context.TODO(), &groupID, group, "claude-haiku-4-5-20251001")
	require.Equal(t, "gpt-5.3-codex", got)
}

func TestSanitizeGroupMessagesDispatchFields_ClearsNonOpenAIPlatform(t *testing.T) {
	t.Parallel()

	group := &Group{
		Platform:              PlatformAnthropic,
		AllowMessagesDispatch: true,
		DefaultMappedModel:    "gpt-5.6-sol",
		MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{
			SonnetMappedModel: "gpt-5.3-codex",
			ExactModelMappings: map[string]string{
				"claude-fable-5": "gpt-5.6-sol",
			},
		},
	}

	sanitizeGroupMessagesDispatchFields(group)

	require.False(t, group.AllowMessagesDispatch)
	require.Empty(t, group.DefaultMappedModel)
	require.Equal(t, OpenAIMessagesDispatchModelConfig{}, group.MessagesDispatchModelConfig)
}

func TestSanitizeGroupMessagesDispatchFields_PreservesCompositeDispatchToggle(t *testing.T) {
	t.Parallel()

	group := &Group{
		Platform:              PlatformComposite,
		AllowMessagesDispatch: true,
		DefaultMappedModel:    "gpt-5.6-sol",
		MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{
			SonnetMappedModel: "gpt-5.3-codex",
			ExactModelMappings: map[string]string{
				"claude-fable-5": "gpt-5.6-sol",
			},
		},
	}

	sanitizeGroupMessagesDispatchFields(group)

	require.True(t, group.AllowMessagesDispatch)
	require.Empty(t, group.DefaultMappedModel)
	require.Equal(t, OpenAIMessagesDispatchModelConfig{}, group.MessagesDispatchModelConfig)
}
