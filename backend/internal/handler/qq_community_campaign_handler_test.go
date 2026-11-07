package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestQQCampaignAdminPermission(t *testing.T) {
	h := NewQQCommunityHandler(nil)
	for _, handler := range []gin.HandlerFunc{h.ListCodeCampaigns, h.SaveCodeCampaign, h.ListCodeClaims, h.ResendCodeClaim, h.DisableCodeClaim, h.ActivityStats} {
		for _, role := range []string{"", "user"} {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			if role != "" {
				c.Set(string(middleware.ContextKeyUserRole), role)
			}
			handler(c)
			require.Equal(t, http.StatusForbidden, w.Code)
		}
	}
}
func TestQQCampaignRejectsMalformedRequestBeforeDatabase(t *testing.T) {
	h := NewQQCommunityHandler(nil)
	for _, tc := range []struct {
		handler           gin.HandlerFunc
		method, url, body string
	}{{h.SaveCodeCampaign, "POST", "/", `{"value":"bad"}`}, {h.ActivityStats, "GET", "/?page=wrong", ""}, {h.ListCodeClaims, "GET", "/?page_size=wrong", ""}} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set(string(middleware.ContextKeyUserRole), "admin")
		c.Request = httptest.NewRequest(tc.method, tc.url, strings.NewReader(tc.body))
		c.Request.Header.Set("Content-Type", "application/json")
		tc.handler(c)
		require.Equal(t, http.StatusBadRequest, w.Code)
	}
}
func TestQQCampaignCSVNeutralizesFormulaCells(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	writeCampaignCSV(c, "test.csv", [][]string{{"=HYPERLINK(1)", "  +SUM(1)", "@test", "plain", "5"}})
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "'=HYPERLINK")
	require.Contains(t, w.Body.String(), "'  +SUM")
	require.Contains(t, w.Body.String(), "'@test,plain,5")
}
