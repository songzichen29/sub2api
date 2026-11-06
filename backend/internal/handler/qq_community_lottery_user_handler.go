package handler

import (
	"errors"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/qqcommunity"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

func (h *QQCommunityHandler) UserLotterySpin(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var input struct {
		RequestID string `json:"request_id" binding:"required,min=16,max=64"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "抽奖请求标识不正确")
		return
	}
	out, err := h.bot.UserLotterySpin(c.Request.Context(), subject.UserID, input.RequestID)
	if err != nil {
		switch {
		case errors.Is(err, qqcommunity.ErrLotteryClosed), errors.Is(err, qqcommunity.ErrLotteryNoChances), errors.Is(err, qqcommunity.ErrLotteryAccount), errors.Is(err, qqcommunity.ErrLotteryRequest):
			response.BadRequest(c, err.Error())
		default:
			response.InternalError(c, "暂时无法确认抽奖结果，请重试查询本次结果")
		}
		return
	}
	response.Success(c, out)
}

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
