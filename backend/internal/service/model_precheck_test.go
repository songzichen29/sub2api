//go:build unit

package service

import (
	"testing"
	"time"

	gocache "github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/require"
)

func TestPeekAvailableModelsSnapshot_RoundTripRestrictiveSnapshot(t *testing.T) {
	cache := gocache.New(time.Minute, time.Minute)
	groupID := int64(12)
	want := availableModelsSnapshot{
		Models:      []string{"claude-sonnet-4"},
		Restrictive: true,
	}

	storeAvailableModelsSnapshot(cache, time.Minute, &groupID, PlatformAnthropic, want)
	got, ok := peekAvailableModelsSnapshot(cache, &groupID, PlatformAnthropic)
	require.True(t, ok)
	require.Equal(t, want, got)
}

func TestSnapshotSupportsRequestedModel_RestrictiveSnapshotRejectsUnknownModel(t *testing.T) {
	snapshot := availableModelsSnapshot{
		Models:      []string{"gpt-4.1"},
		Restrictive: true,
	}

	require.False(t, SnapshotSupportsRequestedModel(snapshot, PlatformOpenAI, "gpt-5"))
	require.True(t, SnapshotSupportsRequestedModel(snapshot, PlatformOpenAI, "gpt-4.1"))
}
