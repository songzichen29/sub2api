package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestBuildMarketplaceModelItems_ExcludesSubscriptionGroups(t *testing.T) {
	channels := []service.AvailableChannel{
		{
			Name:        "mixed-channel",
			Description: "contains both standard and subscription groups",
			Groups: []service.AvailableGroupRef{
				{
					ID:               1,
					Name:             "standard-group",
					Platform:         service.PlatformOpenAI,
					SubscriptionType: service.SubscriptionTypeStandard,
					RateMultiplier:   1,
				},
				{
					ID:               2,
					Name:             "subscription-group",
					Platform:         service.PlatformOpenAI,
					SubscriptionType: service.SubscriptionTypeSubscription,
					RateMultiplier:   1,
				},
			},
			SupportedModels: []service.SupportedModel{
				{Name: "gpt-5", Platform: service.PlatformOpenAI},
			},
		},
		{
			Name:        "subscription-only",
			Description: "only subscription group",
			Groups: []service.AvailableGroupRef{
				{
					ID:               3,
					Name:             "subscription-only-group",
					Platform:         service.PlatformOpenAI,
					SubscriptionType: service.SubscriptionTypeSubscription,
					RateMultiplier:   1,
				},
			},
			SupportedModels: []service.SupportedModel{
				{Name: "gpt-4.1", Platform: service.PlatformOpenAI},
			},
		},
	}

	items := buildMarketplaceModelItems(channels)
	if len(items) != 1 {
		t.Fatalf("expected only 1 marketplace item after filtering subscription groups, got %d", len(items))
	}

	if items[0].ModelName != "gpt-5" {
		t.Fatalf("expected remaining model to be gpt-5, got %q", items[0].ModelName)
	}

	if len(items[0].Channels) != 1 {
		t.Fatalf("expected 1 channel entry, got %d", len(items[0].Channels))
	}

	if len(items[0].Channels[0].Groups) != 1 {
		t.Fatalf("expected only standard group to remain, got %d groups", len(items[0].Channels[0].Groups))
	}

	group := items[0].Channels[0].Groups[0]
	if group.ID != 1 {
		t.Fatalf("expected remaining group id to be 1, got %d", group.ID)
	}

	if group.SubscriptionType != service.SubscriptionTypeStandard {
		t.Fatalf("expected remaining group to be standard, got %q", group.SubscriptionType)
	}

	facets := collectMarketplaceGroupFacets(items)
	if len(facets) != 1 {
		t.Fatalf("expected only 1 group facet after filtering, got %d", len(facets))
	}

	if facets[0].ID != 1 {
		t.Fatalf("expected facet group id to be 1, got %d", facets[0].ID)
	}
}
