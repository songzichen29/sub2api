package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"strconv"
)

// AccountActivity lists committed activity for the authenticated user only.
func (h *QQCommunityHandler) AccountActivity(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	page, pageSize := 1, 20
	for name, target := range map[string]*int{"page": &page, "page_size": &pageSize} {
		if raw, exists := c.GetQuery(name); exists {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 1 {
				response.BadRequest(c, "Invalid "+name)
				return
			}
			*target = value
		}
	}
	if pageSize > 100 {
		pageSize = 100
	}
	if page-1 > 1000000/pageSize {
		response.BadRequest(c, "Invalid page")
		return
	}
	items, total, err := h.bot.AccountActivity(c.Request.Context(), subject.UserID, page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Paginated(c, items, total, page, pageSize)
}
