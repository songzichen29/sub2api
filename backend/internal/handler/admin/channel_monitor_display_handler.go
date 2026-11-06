package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *SettingHandler) GetChannelMonitorDisplayConfig(c *gin.Context) {
	cfg, err := h.settingService.GetChannelMonitorDisplayConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

func (h *SettingHandler) UpdateChannelMonitorDisplayConfig(c *gin.Context) {
	var cfg service.ChannelMonitorDisplayConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		response.BadRequest(c, "Invalid display config")
		return
	}
	updated, err := h.settingService.UpdateChannelMonitorDisplayConfig(c.Request.Context(), cfg)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, updated)
}
