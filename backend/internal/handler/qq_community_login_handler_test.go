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

func TestQQRestartRequiresAdministratorAndExplicitConfirmation(t *testing.T) {
	for _, tc := range []struct {
		role, body string
		code       int
	}{
		{"", `{"confirmed":true,"expected_qq":"1849138657"}`, http.StatusForbidden},
		{"user", `{"confirmed":true,"expected_qq":"1849138657"}`, http.StatusForbidden},
		{"admin", `{}`, http.StatusBadRequest},
		{"admin", `{"confirmed":false,"expected_qq":"1849138657"}`, http.StatusBadRequest},
		{"admin", `{"confirmed":true}`, http.StatusBadRequest},
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		if tc.role != "" {
			c.Set(string(middleware.ContextKeyUserRole), tc.role)
		}
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/qqbot/login/restart", strings.NewReader(tc.body))
		c.Request.Header.Set("Content-Type", "application/json")
		// A nil service ensures rejected requests never reach QQ or the database.
		(&QQCommunityHandler{}).RestartNapCatLogin(c)
		require.Equal(t, tc.code, w.Code)
	}
}
