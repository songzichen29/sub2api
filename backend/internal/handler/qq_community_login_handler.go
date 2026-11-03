package handler

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
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
