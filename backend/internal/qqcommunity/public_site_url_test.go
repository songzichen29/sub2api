package qqcommunity

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type publicURLFixture struct {
	fixtureEmail
	origin string
}

func (f publicURLFixture) FrontendURL(context.Context) string { return f.origin }

func TestCampaignPublicURLNeverUsesRendererOrigin(t *testing.T) {
	s := New(nil, nil, false, nil)
	require.NoError(t, s.ConfigureReplyPages("http://127.0.0.1:8080", nil))
	require.Empty(t, s.publicSiteURL(context.Background(), "/redeem"))
	for _, origin := range []string{"http://127.0.0.1:8080", "https://127.0.0.1:8080", "https://localhost", "https://[::1]", "https://10.0.0.1", "https://u:p@example.com", "https://example.com?token=x", "https://example.com/#x"} {
		s.SetBindingEmailDelivery(publicURLFixture{origin: origin})
		require.Empty(t, s.publicSiteURL(context.Background(), "/redeem"), origin)
	}
	s.SetBindingEmailDelivery(publicURLFixture{origin: "https://example.test/"})
	require.Equal(t, "https://example.test/redeem", s.publicSiteURL(context.Background(), "/redeem"))
	require.Equal(t, "http://127.0.0.1:8080", s.pageOrigin)
}
