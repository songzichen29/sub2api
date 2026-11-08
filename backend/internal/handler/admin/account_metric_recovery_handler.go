package admin

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type accountMetricRecoveryConfigRequest struct {
	ExpectedRevision int64                               `json:"expected_revision"`
	Config           service.AccountMetricRecoveryConfig `json:"config"`
}

type accountMetricRecoveryReleaseRequest struct {
	ExpectedRevision        int64 `json:"expected_revision"`
	ExpectedPauseGeneration int64 `json:"expected_pause_generation"`
}

func parseAccountMetricID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return 0, false
	}
	return id, true
}

func accountMetricActorID(c *gin.Context) *int64 {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		return nil
	}
	value := subject.UserID
	return &value
}

func (h *AccountHandler) metricRecoveryUnavailable(c *gin.Context) bool {
	if h.accountMetricRecovery != nil {
		return false
	}
	response.Error(c, http.StatusServiceUnavailable, "Account metric recovery service not available")
	return true
}

// GetAccountMetricRecovery returns the opt-in monitor state and recent metrics.
// GET /api/v1/admin/accounts/:id/metric-recovery
func (h *AccountHandler) GetAccountMetricRecovery(c *gin.Context) {
	if h.metricRecoveryUnavailable(c) {
		return
	}
	accountID, ok := parseAccountMetricID(c)
	if !ok {
		return
	}
	status, err := h.accountMetricRecovery.GetStatus(c.Request.Context(), accountID, time.Now().UTC())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, status)
}

// SaveAccountMetricRecovery configures per-account metric rules using a
// revision-aware write. A disabled config remains a no-op for scheduling.
// PUT /api/v1/admin/accounts/:id/metric-recovery
func (h *AccountHandler) SaveAccountMetricRecovery(c *gin.Context) {
	if h.metricRecoveryUnavailable(c) {
		return
	}
	accountID, ok := parseAccountMetricID(c)
	if !ok {
		return
	}
	var request accountMetricRecoveryConfigRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.ExpectedRevision < 0 {
		response.BadRequest(c, "Invalid account metric recovery configuration")
		return
	}
	monitor, err := h.accountMetricRecovery.SaveConfig(c.Request.Context(), accountID, request.ExpectedRevision, request.Config, accountMetricActorID(c), time.Now().UTC())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, monitor)
}

// ReleaseAccountMetricRecovery performs an explicit, revision-aware release.
// POST /api/v1/admin/accounts/:id/metric-recovery/release
func (h *AccountHandler) ReleaseAccountMetricRecovery(c *gin.Context) {
	if h.metricRecoveryUnavailable(c) {
		return
	}
	accountID, ok := parseAccountMetricID(c)
	if !ok {
		return
	}
	var request accountMetricRecoveryReleaseRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.ExpectedRevision < 0 || request.ExpectedPauseGeneration < 0 {
		response.BadRequest(c, "Invalid account metric pause revision")
		return
	}
	monitor, err := h.accountMetricRecovery.Release(c.Request.Context(), accountID, request.ExpectedRevision, request.ExpectedPauseGeneration, accountMetricActorID(c), time.Now().UTC())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, monitor)
}

// ListAccountMetricRecoveryEvents returns immutable audit events for an account.
// GET /api/v1/admin/accounts/:id/metric-recovery/events
func (h *AccountHandler) ListAccountMetricRecoveryEvents(c *gin.Context) {
	if h.metricRecoveryUnavailable(c) {
		return
	}
	accountID, ok := parseAccountMetricID(c)
	if !ok {
		return
	}
	beforeID := int64(0)
	if raw := strings.TrimSpace(c.Query("before_id")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			response.BadRequest(c, "Invalid before_id")
			return
		}
		beforeID = parsed
	}
	limit := 50
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			response.BadRequest(c, "Invalid limit")
			return
		}
		limit = parsed
	}
	events, err := h.accountMetricRecovery.ListEvents(c.Request.Context(), accountID, beforeID, limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"events": events})
}
