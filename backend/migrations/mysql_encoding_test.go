package migrations

import (
	"bytes"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMySQLMigrationsDoNotStartWithUTF8BOM(t *testing.T) {
	files, err := fs.Glob(MySQLFS, "*.sql")
	require.NoError(t, err)

	for _, name := range files {
		content, err := MySQLFS.ReadFile(name)
		require.NoError(t, err, name)
		require.False(t, bytes.HasPrefix(content, []byte{0xEF, 0xBB, 0xBF}), "%s must not start with UTF-8 BOM", name)
	}
}
