package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestPluginRepositoryUpdateConfigBindsValuesInPlaceholderOrder(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE sub2api_plugin_installations
		SET config_encrypted = ?, updated_at = NOW()
		WHERE id = ? AND binary_sha256 = ?
	`)).
		WithArgs("encrypted-config", int64(42), "expected-sha256").
		WillReturnResult(sqlmock.NewResult(0, 1))

	repo := &pluginRepository{db: db}
	err = repo.UpdateConfig(context.Background(), 42, "encrypted-config", "expected-sha256")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
