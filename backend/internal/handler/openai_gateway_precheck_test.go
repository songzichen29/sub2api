//go:build unit

package handler

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpenAIGatewayHandlerPrecheckModelAccess_NoSnapshotNoBlock(t *testing.T) {
	h := &OpenAIGatewayHandler{
		gatewayService: &service.OpenAIGatewayService{},
	}

	groupID := int64(1)
	err := h.precheckModelAccess(context.Background(), &groupID, "gpt-5")
	require.NoError(t, err)
}

func TestGatewayModelPrecheckError_MapsStructuredCodes(t *testing.T) {
	status, code, message, ok := gatewayModelPrecheckError(
		errors.Join(service.ErrModelBlockedByGroup, errors.New(service.ModelBlockedByGroupMessage("gpt-5"))),
	)
	require.True(t, ok)
	require.Equal(t, 400, status)
	require.Equal(t, "model_not_allowed", code)
	require.Contains(t, message, "未开放模型")
}
