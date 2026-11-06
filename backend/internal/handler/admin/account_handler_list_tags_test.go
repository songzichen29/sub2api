package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAccountHandlerListIncludesTagsInFullAndLiteResponses(t *testing.T) {
	for _, mode := range []string{"0", "1"} {
		for _, tc := range []struct {
			name string
			tags []string
			want string
		}{
			{name: "tagged", tags: []string{"vip", "生产"}, want: `["vip","生产"]`},
			{name: "empty", tags: []string{}, want: `[]`},
			{name: "legacy_nil", tags: nil, want: `[]`},
		} {
			t.Run("lite="+mode+"/"+tc.name, func(t *testing.T) {
				router, adminSvc := setupAccountListRouter()
				adminSvc.accounts = []service.Account{{
					ID: 501, Name: "tagged-account", Platform: service.PlatformOpenAI,
					Type: service.AccountTypeAPIKey, Status: service.StatusActive, Tags: tc.tags,
				}}
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts?lite="+mode, nil))
				require.Equal(t, http.StatusOK, rec.Code)
				var payload struct {
					Data struct {
						Items []map[string]json.RawMessage `json:"items"`
					} `json:"data"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
				require.Len(t, payload.Data.Items, 1)
				require.Contains(t, payload.Data.Items[0], "tags", "the account table reads tags from the lite response")
				require.JSONEq(t, tc.want, string(payload.Data.Items[0]["tags"]))
			})
		}
	}
}

func TestAccountHandlerListLiteETagChangesWhenTagsChange(t *testing.T) {
	router, adminSvc := setupAccountListRouter()
	adminSvc.accounts = []service.Account{{
		ID: 501, Name: "tagged-account", Platform: service.PlatformOpenAI,
		Type: service.AccountTypeAPIKey, Status: service.StatusActive, Tags: []string{"old"},
	}}
	const url = "/api/v1/admin/accounts?lite=1"
	initial := httptest.NewRecorder()
	router.ServeHTTP(initial, httptest.NewRequest(http.MethodGet, url, nil))
	require.Equal(t, http.StatusOK, initial.Code)
	etag := initial.Header().Get("ETag")
	require.NotEmpty(t, etag)

	// Isolate the response field: even without a timestamp change, replacing or
	// clearing tags must invalidate the body cache used by list auto-refresh.
	for _, tags := range [][]string{{"new", "生产"}, {}} {
		adminSvc.accounts[0].Tags = tags
		req := httptest.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("If-None-Match", etag)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.NotEqual(t, etag, rec.Header().Get("ETag"))
		etag = rec.Header().Get("ETag")

		unchanged := httptest.NewRequest(http.MethodGet, url, nil)
		unchanged.Header.Set("If-None-Match", etag)
		rec304 := httptest.NewRecorder()
		router.ServeHTTP(rec304, unchanged)
		require.Equal(t, http.StatusNotModified, rec304.Code)
	}
}
