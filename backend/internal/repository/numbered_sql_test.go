package repository

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNumberedSQLRebindPreservesCASParameterIdentity(t *testing.T) {
	query, args := bindNumberedSQL(`UPDATE keys SET used_at=$5,ip=$4 WHERE id=$1 AND hash=$2 AND version=$3 AND expires_at>$5`, "id", "hash", "version", "ip", "time")
	require.Equal(t, `UPDATE keys SET used_at=?,ip=? WHERE id=? AND hash=? AND version=? AND expires_at>?`, query)
	require.Equal(t, []any{"time", "ip", "id", "hash", "version", "time"}, args)
}

func TestNumberedSQLRebindSkipsQuotedText(t *testing.T) {
	query, args := bindNumberedSQL(`SELECT '$1', '$2''$3', "$4" FROM t WHERE a=$2 OR b=$1 OR c=$2`, 10, 20)
	require.Equal(t, `SELECT '$1', '$2''$3', "$4" FROM t WHERE a=? OR b=? OR c=?`, query)
	require.Equal(t, []any{20, 10, 20}, args)
}
