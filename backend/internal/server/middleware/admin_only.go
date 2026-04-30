package middleware

import (
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// AdminOnly 管理员权限中间件
// 必须在JWTAuth中间件之后使用
func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, ok := GetUserRoleFromContext(c)
		if !ok {
			AbortWithError(c, 401, "UNAUTHORIZED", "上下文中未找到用户信息")
			return
		}

		// 检查是否为管理员
		if role != service.RoleAdmin {
			AbortWithError(c, 403, "FORBIDDEN", "需要管理员权限")
			return
		}

		c.Next()
	}
}
