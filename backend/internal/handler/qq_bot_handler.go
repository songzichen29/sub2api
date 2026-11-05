package handler

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/qqbot"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

type QQBotHandler struct {
	service   *qqbot.Service
	Community *QQCommunityHandler
}

func NewQQBotHandler(s *qqbot.Service) *QQBotHandler {
	h := &QQBotHandler{service: s}
	if s.Community != nil {
		h.Community = NewQQCommunityHandler(s.Community)
	}
	return h
}
func (h *QQBotHandler) OneBot(c *gin.Context) {
	if h.Community != nil {
		h.service.Community.ServeOneBot(c.Writer, c.Request)
		return
	}
	h.service.ServeOneBot(c.Writer, c.Request)
}
func (h *QQBotHandler) ChatRuntime(c *gin.Context) {
	if h.Community == nil || h.service.Community == nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	h.service.Community.ServeRuntime(c.Writer, c.Request)
}
func (h *QQBotHandler) ChatRuntimeTools(c *gin.Context) {
	if h.Community == nil || h.service.Community == nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	h.service.Community.ServeRuntimeTools(c.Writer, c.Request)
}
func (h *QQBotHandler) GetConfig(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	cfg, err := h.service.Config(c.Request.Context())
	if err != nil {
		qqError(c, err)
		return
	}
	response.Success(c, cfg)
}
func (h *QQBotHandler) SaveConfig(c *gin.Context) {
	if h.Community != nil {
		response.Error(c, http.StatusGone, "QQ 管理已升级，请刷新页面后使用 QQ 机器人配置")
		return
	}
	var cfg qqbot.Config
	if c.ShouldBindJSON(&cfg) != nil {
		response.BadRequest(c, "配置格式不正确")
		return
	}
	result, err := h.service.SaveConfig(c.Request.Context(), cfg)
	if err != nil {
		qqError(c, err)
		return
	}
	response.Success(c, result)
}
func (h *QQBotHandler) Connection(c *gin.Context) {
	if h.Community != nil {
		response.Success(c, h.service.Community.ConnectionStatus())
		return
	}
	response.Success(c, h.service.ConnectionStatus())
}
func (h *QQBotHandler) TestModel(c *gin.Context) {
	if h.Community != nil {
		response.Error(c, http.StatusGone, "请使用新版 QQ 机器人模型测试")
		return
	}
	var cfg qqbot.Config
	if c.ShouldBindJSON(&cfg) != nil {
		response.BadRequest(c, "配置格式不正确")
		return
	}
	result, err := h.service.TestModel(c.Request.Context(), cfg)
	if err != nil {
		qqError(c, err)
		return
	}
	response.Success(c, gin.H{"reply": result})
}
func qqUserID(c *gin.Context) (int64, bool) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "未登录")
		return 0, false
	}
	return subject.UserID, true
}
func (h *QQBotHandler) BindingStatus(c *gin.Context) {
	if h.Community != nil {
		h.communityProfile(c, "BindingStatus")
		return
	}
	c.Header("Cache-Control", "no-store")
	id, ok := qqUserID(c)
	if !ok {
		return
	}
	b, err := h.service.BindingStatus(c.Request.Context(), id)
	if err != nil {
		qqError(c, err)
		return
	}
	response.Success(c, b)
}
func (h *QQBotHandler) CreateCode(c *gin.Context) {
	if h.Community != nil {
		h.communityProfile(c, "CreateCode")
		return
	}
	id, ok := qqUserID(c)
	if !ok {
		return
	}
	code, expiry, err := h.service.CreateCode(c.Request.Context(), id)
	if err != nil {
		qqError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, gin.H{"code": code, "expires_at": expiry})
}
func (h *QQBotHandler) Confirm(c *gin.Context) {
	if h.Community != nil {
		h.communityProfile(c, "Confirm")
		return
	}
	id, ok := qqUserID(c)
	if !ok {
		return
	}
	var input struct {
		QQ string `json:"qq" binding:"required"`
	}
	if c.ShouldBindJSON(&input) != nil {
		response.BadRequest(c, "请确认待绑定的 QQ")
		return
	}
	b, err := h.service.Confirm(c.Request.Context(), id, input.QQ)
	if err != nil {
		qqError(c, err)
		return
	}
	response.Success(c, b)
}
func (h *QQBotHandler) Unbind(c *gin.Context) {
	if h.Community != nil {
		h.communityProfile(c, "Unbind")
		return
	}
	id, ok := qqUserID(c)
	if !ok {
		return
	}
	if err := h.service.Unbind(c.Request.Context(), id); err != nil {
		qqError(c, err)
		return
	}
	response.Success(c, gin.H{"unbound": true})
}
func (h *QQBotHandler) AdminUnbind(c *gin.Context) {
	if h.Community != nil {
		response.Error(c, http.StatusGone, "请使用新版 QQ 绑定管理")
		return
	}
	id, err := strconv.ParseInt(c.Param("userID"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "用户 ID 不正确")
		return
	}
	if err = h.service.Unbind(c.Request.Context(), id); err != nil {
		qqError(c, err)
		return
	}
	response.Success(c, gin.H{"unbound": true})
}
func (h *QQBotHandler) List(c *gin.Context) {
	p, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	n, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if p < 1 || p > 1000000 {
		p = 1
	}
	if n < 1 || n > 100 {
		n = 20
	}
	var items any
	var total int64
	var err error
	switch c.Param("kind") {
	case "bindings":
		items, total, err = h.service.ListBindings(c.Request.Context(), p, n)
	case "rewards":
		items, total, err = h.service.ListRewards(c.Request.Context(), p, n)
	case "events":
		items, total, err = h.service.ListEvents(c.Request.Context(), p, n)
	default:
		response.NotFound(c, "记录类型不存在")
		return
	}
	if err != nil {
		qqError(c, err)
		return
	}
	response.Success(c, gin.H{"items": items, "total": total, "page": p, "page_size": n})
}
func qqError(c *gin.Context, err error) {
	log.Printf("[QQBot] route=%s error=%v", c.FullPath(), err)
	if errors.Is(err, qqbot.ErrInvalidConfig) || errors.Is(err, qqbot.ErrModelRequest) {
		response.BadRequest(c, err.Error())
		return
	}
	if errors.Is(err, qqbot.ErrConflict) {
		response.Error(c, http.StatusConflict, err.Error())
		return
	}
	// User-facing errors are created by the bot service; SQL details stay in logs.
	for _, known := range []error{qqbot.ErrDisabled, qqbot.ErrInvalidCode, qqbot.ErrBindingConflict, qqbot.ErrNotBound, qqbot.ErrInactiveUser, qqbot.ErrAlreadyCheckedIn, qqbot.ErrRewardsDisabled, qqbot.ErrChatLimit, qqbot.ErrCooldown} {
		if errors.Is(err, known) {
			response.BadRequest(c, known.Error())
			return
		}
	}
	response.Error(c, http.StatusBadRequest, "QQ 机器人操作失败，请检查配置或稍后再试")
}
