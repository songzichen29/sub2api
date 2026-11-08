package admin

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupOperationsUpdateOmittedAndDisabled(t *testing.T) {
	var untouched UpdateGroupRequest
	require.NoError(t, json.Unmarshal([]byte(`{"name":"renamed"}`), &untouched))
	require.Nil(t, untouched.UserConcurrencyLimit)
	require.Nil(t, untouched.FallbackGroupIDOnUnavailable)

	var disabled UpdateGroupRequest
	require.NoError(t, json.Unmarshal([]byte(`{"user_concurrency_limit":0,"fallback_group_id_on_unavailable":0}`), &disabled))
	require.NotNil(t, disabled.UserConcurrencyLimit)
	require.Zero(t, *disabled.UserConcurrencyLimit)
	require.NotNil(t, disabled.FallbackGroupIDOnUnavailable)
	require.Zero(t, *disabled.FallbackGroupIDOnUnavailable)
}
