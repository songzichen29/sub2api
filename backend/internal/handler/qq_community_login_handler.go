package handler

import (
	"errors"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/qqcommunity"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

func (h *QQCommunityHandler) NapCatLogin(c *gin.Context) {
	if !requireQQBotAdmin(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	out, err := h.bot.NapCatLogin(c.Request.Context(), c.Request.Method == http.MethodPost)
	if err != nil {
		response.Error(c, http.StatusBadGateway, err.Error())
		return
	}
	response.Success(c, out)
}

func (h *QQCommunityHandler) RestartNapCatLogin(c *gin.Context) {
	if !requireQQBotAdmin(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	var body struct {
		ExpectedQQ string `json:"expected_qq"`
		Confirmed  bool   `json:"confirmed"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if c.ShouldBindJSON(&body) != nil || !body.Confirmed || body.ExpectedQQ == "" || len(body.ExpectedQQ) > 32 {
		response.BadRequest(c, "请确认重新登录，并核对当前机器人 QQ")
		return
	}
	middleware.SetAuditAction(c, "qqbot.login.restart")
	out, err := h.bot.RestartNapCatLogin(c.Request.Context(), body.ExpectedQQ, body.Confirmed)
	if err != nil {
		status := http.StatusBadGateway
		switch {
		case errors.Is(err, qqcommunity.ErrNapCatRestartConfirmation), errors.Is(err, qqcommunity.ErrNapCatLoginInProgress):
			status = http.StatusConflict
		case errors.Is(err, qqcommunity.ErrNapCatRestartCooldown):
			status = http.StatusTooManyRequests
			c.Header("Retry-After", "60")
		}
		response.Error(c, status, err.Error())
		return
	}
	response.Success(c, out)
}
