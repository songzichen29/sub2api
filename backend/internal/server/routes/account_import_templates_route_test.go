package routes

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRegisterSettingsRoutesIncludesAccountImportTemplates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handlers := &handler.Handlers{
		Admin: &handler.AdminHandlers{
			Setting: &adminhandler.SettingHandler{},
		},
	}
	registerSettingsRoutes(router.Group("/api/v1/admin"), handlers)

	wantPath := "/api/v1/admin/settings/account-import-templates"
	registered := map[string]bool{}
	for _, route := range router.Routes() {
		if route.Path == wantPath {
			registered[route.Method] = true
		}
	}

	require.True(t, registered[http.MethodGet], "GET account import templates route is missing")
	require.True(t, registered[http.MethodPut], "PUT account import templates route is missing")
}
