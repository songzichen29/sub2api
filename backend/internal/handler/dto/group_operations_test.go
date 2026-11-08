package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupOperationsDTOVisibility(t *testing.T) {
	targetID := int64(9)
	group := &service.Group{ID: 7, UserConcurrencyLimit: 3, FallbackGroupIDOnUnavailable: &targetID}
	public, err := json.Marshal(GroupFromService(group))
	require.NoError(t, err)
	require.Contains(t, string(public), `"user_concurrency_limit":3`)
	require.NotContains(t, string(public), "fallback_group_id_on_unavailable")
	admin, err := json.Marshal(GroupFromServiceAdmin(group))
	require.NoError(t, err)
	require.Contains(t, string(admin), `"fallback_group_id_on_unavailable":9`)
}
