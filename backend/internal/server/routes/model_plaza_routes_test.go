package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRegisterModelPlazaRoutesReturnsStructuredDisabledError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	optionalJWT := servermiddleware.OptionalJWTAuthMiddleware(func(c *gin.Context) { c.Next() })

	RegisterModelPlazaRoutes(
		v1,
		&handler.Handlers{ModelPlaza: &handler.ModelPlazaHandler{}},
		optionalJWT,
		nil,
		nil,
	)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/model-plaza?timezone=Asia%2FShanghai", nil))

	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Header().Get("Content-Type"), "application/json")
	var body struct {
		Code    int    `json:"code"`
		Reason  string `json:"reason"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, http.StatusNotFound, body.Code)
	require.Equal(t, "MODEL_PLAZA_DISABLED", body.Reason)
	require.Equal(t, "Model plaza is not enabled", body.Message)
}
