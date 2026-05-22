package routes

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func RegisterAccountImportRoutes(v1 *gin.RouterGroup, h *handler.Handlers, redisClient *redis.Client) {
	rateLimiter := middleware.NewRateLimiter(redisClient)
	accountImport := v1.Group("/account-import")
	{
		accountImport.GET("/status", h.AccountImport.GetStatus)
		accountImport.POST("/verify", rateLimiter.LimitWithOptions("account-import-verify", 20, time.Minute, middleware.RateLimitOptions{
			FailureMode: middleware.RateLimitFailClose,
		}), h.AccountImport.Verify)
		accountImport.GET("/templates", h.AccountImport.GetTemplates)
		accountImport.GET("/options", h.AccountImport.GetOptions)
		accountImport.POST("/data", h.AccountImport.ImportData)
	}
}
