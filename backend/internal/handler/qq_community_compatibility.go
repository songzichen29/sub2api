package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/qqcommunity"
	"github.com/gin-gonic/gin"
)

func (h *QQCommunityHandler) Models(c *gin.Context) {
	if !requireQQBotAdmin(c) {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var draft qqcommunity.Config
	if c.ShouldBindJSON(&draft) != nil {
		response.BadRequest(c, "配置格式不正确")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	models, err := h.bot.Models(ctx, c.Param("kind"), draft)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, gin.H{"models": models})
}
func (h *QQBotHandler) communityProfile(c *gin.Context, action string) {
	user, ok := qqUserID(c)
	if !ok {
		return
	}
	bot := h.service.Community
	cfg, err := bot.Config(c.Request.Context())
	if err != nil {
		response.InternalError(c, "读取机器人配置失败")
		return
	}
	switch action {
	case "BindingStatus":
		b, p, err := bot.Binding(c.Request.Context(), cfg.AppID, user)
		if err != nil {
			response.InternalError(c, "读取绑定失败")
			return
		}
		var bound any
		pending := ""
		var expiry *time.Time
		if b != nil {
			bound = gin.H{"user_id": b.UserID, "qq": b.MemberID, "group_id": b.GroupID, "created_at": b.CreatedAt}
		}
		if p != nil {
			pending = p.MemberID
			expiry = &p.ExpiresAt
		}
		c.Header("Cache-Control", "no-store")
		response.Success(c, gin.H{"enabled": cfg.Enabled, "bot_qq": cfg.AppID, "binding": bound, "pending_qq": pending, "expires_at": expiry})
	case "CreateCode":
		p, err := bot.CreateBindingCode(c.Request.Context(), user)
		if err != nil {
			response.BadRequest(c, "生成绑定码失败")
			return
		}
		c.Header("Cache-Control", "no-store")
		response.Success(c, p)
	case "Confirm":
		var input struct {
			QQ string `json:"qq"`
		}
		if c.ShouldBindJSON(&input) != nil || input.QQ == "" {
			response.BadRequest(c, "请确认待绑定 QQ")
			return
		}
		if err = bot.ConfirmBinding(c.Request.Context(), user, input.QQ); err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		_ = bot.SettleInvites(c.Request.Context(), input.QQ)
		response.Success(c, gin.H{"bound": true})
	case "Unbind":
		b, _, err := bot.Binding(c.Request.Context(), cfg.AppID, user)
		if err != nil || b == nil {
			response.BadRequest(c, "没有绑定")
			return
		}
		if err = bot.Unbind(c.Request.Context(), user, qqcommunity.UnbindRequest{AppID: cfg.AppID, MemberID: b.MemberID, CreatedAt: b.CreatedAt}); err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		response.Success(c, gin.H{"unbound": true})
	}
}
