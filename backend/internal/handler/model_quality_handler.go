package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type ModelQualityHandler struct{ service *service.ModelQualityService }

func NewModelQualityHandler(svc *service.ModelQualityService) *ModelQualityHandler {
	return &ModelQualityHandler{service: svc}
}

func qualityViewer(c *gin.Context) (int64, bool) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "user not found in context")
		return 0, false
	}
	return subject.UserID, true
}

func qualityError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrQualityInvalid):
		response.BadRequest(c, err.Error())
	case errors.Is(err, service.ErrQualityBusy):
		response.Error(c, http.StatusConflict, "检测正在进行，请待其结束或取消后再修改/重试")
	case errors.Is(err, service.ErrQualityNotFound):
		response.NotFound(c, "检测记录不存在")
	default:
		response.ErrorFrom(c, err)
	}
}

func (h *ModelQualityHandler) List(c *gin.Context)      { h.list(c, false) }
func (h *ModelQualityHandler) AdminList(c *gin.Context) { h.list(c, true) }
func (h *ModelQualityHandler) list(c *gin.Context, admin bool) {
	user, ok := qualityViewer(c)
	if !ok {
		return
	}
	items, err := h.service.List(c.Request.Context(), user, admin)
	if err != nil {
		qualityError(c, err)
		return
	}
	response.Success(c, items)
}

func (h *ModelQualityHandler) Options(c *gin.Context) {
	user, ok := qualityViewer(c)
	if !ok {
		return
	}
	options, err := h.service.Options(c.Request.Context(), user)
	if err != nil {
		qualityError(c, err)
		return
	}
	response.Success(c, options)
}

func (h *ModelQualityHandler) Save(c *gin.Context) {
	user, ok := qualityViewer(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var input service.ModelQualityInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "invalid detection configuration")
		return
	}
	p, err := h.service.Save(c.Request.Context(), c.Param("id"), user, input)
	if err != nil {
		qualityError(c, err)
		return
	}
	response.Success(c, p)
}

func (h *ModelQualityHandler) Run(c *gin.Context) {
	r, err := h.service.Queue(c.Request.Context(), c.Param("id"), false)
	if err != nil {
		qualityError(c, err)
		return
	}
	response.Accepted(c, r)
}

func (h *ModelQualityHandler) History(c *gin.Context)           { h.history(c, false) }
func (h *ModelQualityHandler) AdminHistory(c *gin.Context)      { h.history(c, true) }
func (h *ModelQualityHandler) GroupHistory(c *gin.Context)      { h.history(c, false) }
func (h *ModelQualityHandler) AdminGroupHistory(c *gin.Context) { h.history(c, true) }
func (h *ModelQualityHandler) history(c *gin.Context, admin bool) {
	user, ok := qualityViewer(c)
	if !ok {
		return
	}
	page := 1
	if raw := c.Query("page"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100000 {
			response.BadRequest(c, "invalid page")
			return
		}
		page = n
	}
	var runs []*service.ModelQualityRun
	var err error
	if raw := c.Param("groupID"); raw != "" {
		groupID, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || groupID <= 0 {
			response.BadRequest(c, "invalid group")
			return
		}
		runs, err = h.service.GroupHistory(c.Request.Context(), groupID, user, admin, page)
	} else {
		runs, err = h.service.History(c.Request.Context(), c.Param("id"), user, admin, page)
	}
	if err != nil {
		qualityError(c, err)
		return
	}
	response.Success(c, runs)
}

func (h *ModelQualityHandler) Detail(c *gin.Context)      { h.detail(c, false) }
func (h *ModelQualityHandler) AdminDetail(c *gin.Context) { h.detail(c, true) }
func (h *ModelQualityHandler) detail(c *gin.Context, admin bool) {
	user, ok := qualityViewer(c)
	if !ok {
		return
	}
	r, err := h.service.Detail(c.Request.Context(), c.Param("id"), user, admin)
	if err != nil {
		qualityError(c, err)
		return
	}
	response.Success(c, r)
}

func (h *ModelQualityHandler) GroupResults(c *gin.Context)      { h.results(c, false) }
func (h *ModelQualityHandler) AdminGroupResults(c *gin.Context) { h.results(c, true) }
func (h *ModelQualityHandler) results(c *gin.Context, admin bool) {
	user, ok := qualityViewer(c)
	if !ok {
		return
	}
	groupID, err := strconv.ParseInt(c.Param("groupID"), 10, 64)
	if err != nil || groupID <= 0 {
		response.BadRequest(c, "invalid group")
		return
	}
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil {
		response.BadRequest(c, "invalid page")
		return
	}
	result, err := h.service.GroupResults(c.Request.Context(), groupID, user, admin, c.Query("benchmark"), page)
	if err != nil {
		qualityError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ModelQualityHandler) Cancel(c *gin.Context) {
	if err := h.service.Cancel(c.Request.Context(), c.Param("id")); err != nil {
		qualityError(c, err)
		return
	}
	response.Accepted(c, gin.H{"cancel_requested": true})
}
