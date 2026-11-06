package handler

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

// The authenticated subject is the only source of identity for all user views.
// Never forward administrator search filters or user IDs from request data.
func (h *QQCommunityHandler) UserLottery(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	out, err := h.bot.UserLottery(c.Request.Context(), subject.UserID)
	if err != nil {
		response.InternalError(c, "读取抽奖信息失败，请稍后重试")
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, out)
}
func (h *QQCommunityHandler) UserLotteryMembership(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	out, err := h.bot.UserLotteryMembership(c.Request.Context(), subject.UserID)
	if err != nil {
		response.InternalError(c, "暂时无法核验入群状态，请稍后重试")
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, out)
}
func (h *QQCommunityHandler) UserLotteryDraws(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	page, e1 := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, e2 := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if e1 != nil || e2 != nil || page < 1 || page > 100000 || size < 1 || size > 100 {
		response.BadRequest(c, "分页参数不正确")
		return
	}
	out, err := h.bot.UserLotteryDraws(c.Request.Context(), subject.UserID, page, size)
	if err != nil {
		response.InternalError(c, "读取抽奖记录失败，请稍后重试")
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, out)
}
