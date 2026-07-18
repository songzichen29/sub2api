package repository

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReplaceSQLNowWithClock(t *testing.T) {
	in := "SELECT NOW(), DATE(NOW()), TIMESTAMPDIFF(SECOND, a, NOW())"
	out := replaceSQLNowWithClock(in)

	require.NotContains(t, out, "NOW()")
	require.Equal(t, 3, strings.Count(out, "clock.now_ts"))
}
