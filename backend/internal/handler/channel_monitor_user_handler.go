package handler

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ChannelMonitorUserHandler 渠道监控用户只读 handler。
type ChannelMonitorUserHandler struct {
	monitorService *service.ChannelMonitorService
	settingService *service.SettingService
	groupReader    channelMonitorBillingReader
}

type channelMonitorBillingReader interface {
	GetAvailableGroups(context.Context, int64) ([]service.Group, error)
	GetUserGroupRates(context.Context, int64) (map[int64]float64, error)
}

// NewChannelMonitorUserHandler 创建 handler。
// settingService 用于每次请求前读取功能开关；关闭时 List/GetStatus 直接返回空/404。
func NewChannelMonitorUserHandler(
	monitorService *service.ChannelMonitorService,
	settingService *service.SettingService,
	apiKeyService *service.APIKeyService,
) *ChannelMonitorUserHandler {
	return &ChannelMonitorUserHandler{
		monitorService: monitorService,
		settingService: settingService,
		groupReader:    apiKeyService,
	}
}

// featureEnabled 返回当前渠道监控功能是否开启。
// settingService 为 nil（测试场景）视为启用。
func (h *ChannelMonitorUserHandler) featureEnabled(c *gin.Context) bool {
	if h.settingService == nil {
		return true
	}
	runtime := h.settingService.GetChannelMonitorRuntime(c.Request.Context())
	return runtime.Enabled && runtime.Mode == service.ChannelMonitorModeV1
}

// quotaVisible 返回用户端是否展示配额/余额快照（channel_monitor_show_quota，
// fail-closed：未配置/非 "true" 一律视为关闭）。settingService 为 nil 时 fail-closed。
func (h *ChannelMonitorUserHandler) quotaVisible(c *gin.Context) bool {
	if h.settingService == nil {
		return false
	}
	return h.settingService.GetChannelMonitorRuntime(c.Request.Context()).ShowQuota
}

// --- Response ---

type channelMonitorUserListItem struct {
	ID                   int64                                `json:"id"`
	Name                 string                               `json:"name"`
	Provider             string                               `json:"provider"`
	GroupName            string                               `json:"group_name"`
	BillingGroupName     string                               `json:"billing_group_name,omitempty"`
	RateMultiplier       *float64                             `json:"rate_multiplier,omitempty"`
	PrimaryModel         string                               `json:"primary_model"`
	PrimaryStatus        string                               `json:"primary_status"`
	PrimaryLatencyMs     *int                                 `json:"primary_latency_ms"`
	PrimaryPingLatencyMs *int                                 `json:"primary_ping_latency_ms"`
	Availability7d       float64                              `json:"availability_7d"`
	ExtraModels          []dto.ChannelMonitorExtraModelStatus `json:"extra_models"`
	Timeline             []channelMonitorUserTimelinePoint    `json:"timeline"`
	// LatestQuota 主模型最近配额快照；channel_monitor_show_quota=false 时
	// 由 userMonitorViewToItem 的调用方传入 false 剥离（服务端脱敏，非仅前端隐藏）。
	LatestQuota *domain.MonitorQuotaSnapshot `json:"latest_quota,omitempty"`
}

// channelMonitorUserTimelinePoint 主模型最近一次检测的 timeline 点。
// 仅用于用户视图 list 响应，admin 视图不使用。
type channelMonitorUserTimelinePoint struct {
	Status        string `json:"status"`
	LatencyMs     *int   `json:"latency_ms"`
	PingLatencyMs *int   `json:"ping_latency_ms"`
	CheckedAt     string `json:"checked_at"`
}

type channelMonitorUserDetailResponse struct {
	ID        int64                         `json:"id"`
	Name      string                        `json:"name"`
	Provider  string                        `json:"provider"`
	GroupName string                        `json:"group_name"`
	Models    []channelMonitorUserModelStat `json:"models"`
}

type channelMonitorUserModelStat struct {
	Model           string  `json:"model"`
	LatestStatus    string  `json:"latest_status"`
	LatestLatencyMs *int    `json:"latest_latency_ms"`
	Availability7d  float64 `json:"availability_7d"`
	Availability15d float64 `json:"availability_15d"`
	Availability30d float64 `json:"availability_30d"`
	AvgLatency7dMs  *int    `json:"avg_latency_7d_ms"`
}

func userMonitorViewToItem(v *service.UserMonitorView, includeQuota bool) channelMonitorUserListItem {
	extras := make([]dto.ChannelMonitorExtraModelStatus, 0, len(v.ExtraModels))
	for _, e := range v.ExtraModels {
		extras = append(extras, dto.ChannelMonitorExtraModelStatus{
			Model:     e.Model,
			Status:    e.Status,
			LatencyMs: e.LatencyMs,
		})
	}
	timeline := make([]channelMonitorUserTimelinePoint, 0, len(v.Timeline))
	for _, p := range v.Timeline {
		timeline = append(timeline, channelMonitorUserTimelinePoint{
			Status:        p.Status,
			LatencyMs:     p.LatencyMs,
			PingLatencyMs: p.PingLatencyMs,
			CheckedAt:     p.CheckedAt.UTC().Format(time.RFC3339),
		})
	}
	item := channelMonitorUserListItem{
		ID:                   v.ID,
		Name:                 v.Name,
		Provider:             v.Provider,
		GroupName:            v.GroupName,
		PrimaryModel:         v.PrimaryModel,
		PrimaryStatus:        v.PrimaryStatus,
		PrimaryLatencyMs:     v.PrimaryLatencyMs,
		PrimaryPingLatencyMs: v.PrimaryPingLatencyMs,
		Availability7d:       v.Availability7d,
		ExtraModels:          extras,
		Timeline:             timeline,
	}
	if includeQuota {
		item.LatestQuota = v.LatestQuota
	}
	return item
}

func userMonitorDetailToResponse(d *service.UserMonitorDetail) *channelMonitorUserDetailResponse {
	models := make([]channelMonitorUserModelStat, 0, len(d.Models))
	for _, m := range d.Models {
		models = append(models, channelMonitorUserModelStat{
			Model:           m.Model,
			LatestStatus:    m.LatestStatus,
			LatestLatencyMs: m.LatestLatencyMs,
			Availability7d:  m.Availability7d,
			Availability15d: m.Availability15d,
			Availability30d: m.Availability30d,
			AvgLatency7dMs:  m.AvgLatency7dMs,
		})
	}
	return &channelMonitorUserDetailResponse{
		ID:        d.ID,
		Name:      d.Name,
		Provider:  d.Provider,
		GroupName: d.GroupName,
		Models:    models,
	}
}

// --- Handlers ---

// List GET /api/v1/channel-monitors
func (h *ChannelMonitorUserHandler) List(c *gin.Context) {
	if !h.featureEnabled(c) {
		response.Success(c, gin.H{"items": []channelMonitorUserListItem{}})
		return
	}
	views, err := h.monitorService.ListUserView(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	includeQuota := h.quotaVisible(c)
	cfg, err := h.settingService.GetChannelMonitorDisplayConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	cfg.SortUserViews(views)
	items := make([]channelMonitorUserListItem, 0, len(views))
	for _, v := range views {
		items = append(items, userMonitorViewToItem(v, includeQuota))
	}
	if !h.addBillingRates(c, items, cfg) {
		return
	}
	response.Success(c, gin.H{"items": items})
}

func (h *ChannelMonitorUserHandler) addBillingRates(c *gin.Context, items []channelMonitorUserListItem, cfg *service.ChannelMonitorDisplayConfig) bool {
	mapping := make(map[int64]int64, len(cfg.GroupMappings))
	for _, link := range cfg.GroupMappings {
		mapping[link.MonitorID] = link.GroupID
	}
	hasVisibleLink := false
	for _, item := range items {
		if mapping[item.ID] > 0 {
			hasVisibleLink = true
			break
		}
	}
	if !hasVisibleLink {
		return true
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return false
	}
	if h.groupReader == nil {
		response.InternalError(c, "Billing group reader unavailable")
		return false
	}
	groups, err := h.groupReader.GetAvailableGroups(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return false
	}
	rates, err := h.groupReader.GetUserGroupRates(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return false
	}
	allowed := make(map[int64]service.Group, len(groups))
	for _, group := range groups {
		allowed[group.ID] = group
	}
	now := time.Now()
	for i := range items {
		group, ok := allowed[mapping[items[i].ID]]
		// Do not expose unavailable/private/deleted group names or prices.
		if !ok {
			continue
		}
		rate := group.RateMultiplier
		if custom, ok := rates[group.ID]; ok {
			rate = custom
		}
		rate *= group.PeakMultiplierAt(now)
		items[i].RateMultiplier = &rate
		items[i].BillingGroupName = dto.GroupFromService(&group).Localized(c.GetHeader("Accept-Language")).Name
	}
	return true
}

// GetStatus GET /api/v1/channel-monitors/:id/status
func (h *ChannelMonitorUserHandler) GetStatus(c *gin.Context) {
	if !h.featureEnabled(c) {
		response.ErrorFrom(c, service.ErrChannelMonitorNotFound)
		return
	}
	// 复用 admin.ParseChannelMonitorID 保持错误码与日志一致。
	id, ok := admin.ParseChannelMonitorID(c)
	if !ok {
		return
	}
	detail, err := h.monitorService.GetUserDetail(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, userMonitorDetailToResponse(detail))
}
