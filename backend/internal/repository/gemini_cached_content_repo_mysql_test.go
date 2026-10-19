package repository

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGeminiCachedContentMySQLQueryRebindsPostgresSyntax(t *testing.T) {
	repo := &geminiCachedContentRepository{mysql: true}
	query := repo.geminiCachedContentQuery(`UPDATE gemini_cached_contents
 SET mutation_token = $2, mutation_lease_until = NOW() + make_interval(secs => $3::double precision)
 WHERE id = $1 AND expire_time = $4 AND note = '$5'`)

	require.NotContains(t, query, "$1")
	require.NotContains(t, query, "make_interval")
	require.NotContains(t, query, "::double precision")
	require.Contains(t, query, "mutation_lease_until = DATE_ADD(NOW(), INTERVAL ? SECOND)")
	require.Contains(t, query, "WHERE id = ? AND expire_time = ?")
	require.Contains(t, query, "note = '$5'")
}

func TestGeminiCachedContentMySQLQueryReordersArguments(t *testing.T) {
	repo := &geminiCachedContentRepository{mysql: true}
	query, args := repo.geminiCachedContentQueryArgs(`UPDATE gemini_cached_contents
 SET mutation_token = $2, mutation_lease_until = NOW() + make_interval(secs => $3::double precision)
 WHERE id = $1 AND expire_time = $4`, "id", "token", "lease", "baseline")

	require.Equal(t, `UPDATE gemini_cached_contents
 SET mutation_token = ?, mutation_lease_until = DATE_ADD(NOW(), INTERVAL ? SECOND)
 WHERE id = ? AND expire_time = ?`, query)
	require.Equal(t, []any{"token", "lease", "id", "baseline"}, args)
}
