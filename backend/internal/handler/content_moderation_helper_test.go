package handler

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestContentModerationRequestIDMatchesUsageBillingFormat(t *testing.T) {
	t.Run("client request id takes precedence", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ctxkey.RequestID, "local-123")
		ctx = context.WithValue(ctx, ctxkey.ClientRequestID, " client-456 ")

		require.Equal(t, "client:client-456", contentModerationRequestID(ctx))
	})

	t.Run("local request id is prefixed", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ctxkey.RequestID, " local-123 ")

		require.Equal(t, "local:local-123", contentModerationRequestID(ctx))
	})

	t.Run("empty context returns empty id", func(t *testing.T) {
		require.Empty(t, contentModerationRequestID(nil))
	})
}
