package qqcommunity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestModelConnectionCredentialScope(t *testing.T) {
	saved := Config{BaseURL: "https://chat.example/v1", APIKey: "chat-key", ImageBaseURL: "https://image.example/v1", ImageAPIKey: "image-key"}
	for _, tc := range []struct {
		name, kind string
		draft      Config
		base, key  string
		fail       bool
	}{
		{"saved chat", "chat", Config{BaseURL: saved.BaseURL}, saved.BaseURL, "chat-key", false},
		{"all image credentials", "image", Config{BaseURL: saved.BaseURL, ImageBaseURL: saved.ImageBaseURL}, saved.ImageBaseURL, "image-key", false},
		{"clear image connection", "image", Config{BaseURL: saved.BaseURL, ClearImageAPIKey: true}, saved.BaseURL, "chat-key", false},
		{"reject forwarded image key", "image", Config{BaseURL: saved.BaseURL}, "", "", true},
		{"reject forwarded chat key", "chat", Config{BaseURL: "https://other.example/v1"}, "", "", true},
		{"clear chat key", "chat", Config{BaseURL: saved.BaseURL, ClearAPIKey: true}, saved.BaseURL, "", false},
		{"new credentials", "chat", Config{BaseURL: "https://other.example/v1", APIKey: "new-key"}, "https://other.example/v1", "new-key", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base, key, err := modelConnection(saved, tc.draft, tc.kind)
			if tc.fail {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.base, base)
			require.Equal(t, tc.key, key)
		})
	}
	saved.ImageBaseURL = ""
	base, key, err := modelConnection(saved, Config{BaseURL: saved.BaseURL}, "image")
	require.NoError(t, err)
	require.Equal(t, saved.BaseURL, base)
	require.Equal(t, "image-key", key)
}

func TestModelsListsAllIDsWithSavedKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/models", r.URL.Path)
		require.Equal(t, "Bearer test-only-key", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"data":[{"id":"text-model"},{"id":"image-model"},{"id":"text-model"},{"id":""}]}`))
	}))
	defer server.Close()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	c := DefaultConfig()
	c.BaseURL = server.URL + "/v1"
	c.APIKey = "test-only-key"
	raw, _ := json.Marshal(c)
	mock.ExpectQuery("SELECT value FROM settings").WithArgs(configKey).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(string(raw)))
	s := New(db, fixtureEncryptor{}, true, nil)
	models, err := s.Models(context.Background(), "chat", Config{BaseURL: c.BaseURL})
	require.NoError(t, err)
	require.Equal(t, []string{"image-model", "text-model"}, models)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestMySQLNestedSearch(t *testing.T) {
	q, args, err := bindSQL(true, `SELECT strpos(lower(COALESCE(b.nickname,'')),lower($1)),strpos(lower($1),lower('a,b'))`, []any{"Alice"})
	require.NoError(t, err)
	require.Equal(t, `SELECT LOCATE(lower(?),lower(COALESCE(b.nickname,''))),LOCATE(lower('a,b'),lower(?))`, q)
	require.Equal(t, []any{"Alice", "Alice"}, args)
	q, args, err = bindSQL(true, `SELECT NOW()-($2*INTERVAL '1 second')`, []any{"unused", 0})
	require.NoError(t, err)
	require.Equal(t, `SELECT NOW(6)-INTERVAL ? SECOND`, q)
	require.Equal(t, []any{0}, args)
}
