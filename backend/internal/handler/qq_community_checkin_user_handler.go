package handler

import (
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/qqcommunity"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

func (h *QQCommunityHandler) UserCheckin(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	out, err := h.bot.UserCheckin(c.Request.Context(), subject.UserID, c.Query("month"))
	if err != nil {
		checkinError(c, err)
		return
	}
	response.Success(c, out)
}

func (h *QQCommunityHandler) ClaimUserCheckin(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var input struct {
		Day string `json:"day" binding:"required,len=10"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, qqcommunity.ErrCheckinDay.Error())
		return
	}
	out, err := h.bot.ClaimUserCheckin(c.Request.Context(), subject.UserID, input.Day)
	if err != nil {
		checkinError(c, err)
		return
	}
	response.Success(c, out)
}

func checkinError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, qqcommunity.ErrCheckinDay), errors.Is(err, qqcommunity.ErrCheckinMonth), errors.Is(err, qqcommunity.ErrCheckinClosed), errors.Is(err, qqcommunity.ErrCheckinAccount), errors.Is(err, qqcommunity.ErrCheckinEligibility):
		response.BadRequest(c, err.Error())
	default:
		response.InternalError(c, "签到信息暂未确认，请稍后重试；同一账号每日只会发放一次")
	}
}
