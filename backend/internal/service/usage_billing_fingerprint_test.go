package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildUsageBillingFingerprintIncludesEveryCostComponent(t *testing.T) {
	base := UsageBillingCommand{
		UserID:              1,
		AccountID:           2,
		APIKeyID:            3,
		Model:               "gpt-5",
		InputTokens:         10,
		OutputTokens:        5,
		SubscriptionCost:    0.1,
		APIKeyQuotaCost:     0.2,
		APIKeyRateLimitCost: 0.3,
		AccountQuotaCost:    0.4,
	}
	baseFingerprint := buildUsageBillingFingerprint(&base)
	require.NotEmpty(t, baseFingerprint)
	require.Equal(t, baseFingerprint, buildUsageBillingFingerprint(&base))

	tests := []struct {
		name   string
		mutate func(*UsageBillingCommand)
	}{
		{name: "subscription cost", mutate: func(command *UsageBillingCommand) { command.SubscriptionCost++ }},
		{name: "api key quota cost", mutate: func(command *UsageBillingCommand) { command.APIKeyQuotaCost++ }},
		{name: "api key rate limit cost", mutate: func(command *UsageBillingCommand) { command.APIKeyRateLimitCost++ }},
		{name: "account quota cost", mutate: func(command *UsageBillingCommand) { command.AccountQuotaCost++ }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := base
			test.mutate(&changed)
			require.NotEqual(t, baseFingerprint, buildUsageBillingFingerprint(&changed))
		})
	}
}
