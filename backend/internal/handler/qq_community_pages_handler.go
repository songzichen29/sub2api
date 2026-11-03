package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
	"net/http"
)

func (h *QQCommunityHandler) ReplyPage(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("X-Robots-Tag", "noindex, nofollow")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var input struct {
		Token string `json:"token"`
	}
	if c.ShouldBindJSON(&input) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "页面参数不正确"})
		return
	}
	page, err := h.bot.ReplyPage(c.Request.Context(), input.Token)
	if err != nil {
		c.JSON(http.StatusGone, gin.H{"error": "页面链接已失效，请在群里重新发送查询指令"})
		return
	}
	c.JSON(http.StatusOK, page)
}

func (h *QQCommunityHandler) PreviewReplyPage(c *gin.Context) {
	if !requireQQBotAdmin(c) {
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var input struct {
		Kind   string `json:"kind"`
		Model  string `json:"model"`
		Filter string `json:"filter"`
		Group  string `json:"group_id"`
		Member string `json:"member_id"`
	}
	if c.ShouldBindJSON(&input) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "页面参数不正确"})
		return
	}
	path, err := h.bot.PreviewReplyPage(c.Request.Context(), input.Kind, input.Model, input.Filter, input.Group, input.Member)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法生成页面，请核对群、绑定状态和模型筛选条件"})
		return
	}
	if c.Query("image") == "1" {
		img, err := h.bot.CaptureReplyPage(c.Request.Context(), path)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "页面截图失败，请检查浏览器运行状态"})
			return
		}
		c.Data(http.StatusOK, "image/jpeg", img)
		return
	}
	response.Success(c, gin.H{"path": path})
}
