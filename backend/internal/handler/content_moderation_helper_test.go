package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
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

func TestSetOpsSelectedAccountInvokesContentModerationBinder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)

	var gotAccountID int64
	var gotAccountName string
	c.Set(contentModerationAccountBinderKey, contentModerationAccountBinder(func(ctx context.Context, accountID int64, accountName string) {
		gotAccountID = accountID
		gotAccountName = accountName
	}))

	setOpsSelectedAccount(c, 42, "openai", "account-a")

	require.Equal(t, int64(42), gotAccountID)
	require.Equal(t, "account-a", gotAccountName)
	require.Equal(t, int64(42), c.Request.Context().Value(ctxkey.AccountID))
	require.Equal(t, "openai", c.Request.Context().Value(ctxkey.Platform))
}
