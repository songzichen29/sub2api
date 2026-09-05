package admin

import (
	"context"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionplan"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// PaymentHandler handles admin payment management.
type PaymentHandler struct {
	paymentService *service.PaymentService
	configService  *service.PaymentConfigService
	couponService  *service.CouponService
}

// NewPaymentHandler creates a new admin PaymentHandler.
func NewPaymentHandler(paymentService *service.PaymentService, configService *service.PaymentConfigService, couponService ...*service.CouponService) *PaymentHandler {
	var cs *service.CouponService
	if len(couponService) > 0 {
		cs = couponService[0]
	}
	return &PaymentHandler{
		paymentService: paymentService,
		configService:  configService,
		couponService:  cs,
	}
}

// --- Dashboard ---

// GetDashboard returns payment dashboard statistics.
// GET /api/v1/admin/payment/dashboard
func (h *PaymentHandler) GetDashboard(c *gin.Context) {
	startRaw := firstNonEmptyQuery(c.Query("start"), c.Query("start_date"))
	endRaw := firstNonEmptyQuery(c.Query("end"), c.Query("end_date"))
	if (startRaw == "") != (endRaw == "") {
		response.BadRequest(c, "start and end are required together")
		return
	}

	if startRaw != "" {
		const maxRangeDays = 370
		userTZ := c.Query("timezone")
		start, err := timezone.ParseInUserLocation("2006-01-02", startRaw, userTZ)
		if err != nil {
			response.BadRequest(c, "invalid start date")
			return
		}
		end, err := timezone.ParseInUserLocation("2006-01-02", endRaw, userTZ)
		if err != nil {
			response.BadRequest(c, "invalid end date")
			return
		}
		start = timezone.StartOfDayInUserLocation(start, userTZ)
		endExclusive := timezone.StartOfDayInUserLocation(end, userTZ).AddDate(0, 0, 1)
		if !endExclusive.After(start) {
			response.BadRequest(c, "end date must be greater than or equal to start date")
			return
		}
		if endExclusive.Sub(start) > maxRangeDays*24*time.Hour {
			response.BadRequest(c, "date range is too large")
			return
		}
		stats, err := h.paymentService.GetDashboardStatsRange(c.Request.Context(), start, endExclusive)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		response.Success(c, stats)
		return
	}

	days := 30
	if d := c.Query("days"); d != "" {
		if v, err := strconv.Atoi(d); err == nil && v > 0 {
			days = v
		}
	}
	stats, err := h.paymentService.GetDashboardStats(c.Request.Context(), days)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, stats)
}

func firstNonEmptyQuery(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// GetDailyStats returns payment daily statistics for an explicit date range.
// GET /api/v1/admin/payment/daily-stats?start=2026-06-01&end=2026-06-30
func (h *PaymentHandler) GetDailyStats(c *gin.Context) {
	const maxRangeDays = 370

	userTZ := c.Query("timezone")
	startRaw := c.Query("start")
	endRaw := c.Query("end")
	if startRaw == "" || endRaw == "" {
		response.BadRequest(c, "start and end are required")
		return
	}

	start, err := timezone.ParseInUserLocation("2006-01-02", startRaw, userTZ)
	if err != nil {
		response.BadRequest(c, "invalid start date")
		return
	}
	end, err := timezone.ParseInUserLocation("2006-01-02", endRaw, userTZ)
	if err != nil {
		response.BadRequest(c, "invalid end date")
		return
	}
	start = timezone.StartOfDayInUserLocation(start, userTZ)
	endExclusive := timezone.StartOfDayInUserLocation(end, userTZ).AddDate(0, 0, 1)

	if !endExclusive.After(start) {
		response.BadRequest(c, "end date must be greater than or equal to start date")
		return
	}
	if endExclusive.Sub(start) > maxRangeDays*24*time.Hour {
		response.BadRequest(c, "date range is too large")
		return
	}

	series, err := h.paymentService.GetDailyStatsRange(c.Request.Context(), start, endExclusive)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, series)
}

// --- Orders ---

// ListOrders returns a paginated list of all payment orders.
// GET /api/v1/admin/payment/orders
func (h *PaymentHandler) ListOrders(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	var userID int64
	if uid := c.Query("user_id"); uid != "" {
		if v, err := strconv.ParseInt(uid, 10, 64); err == nil {
			userID = v
		}
	}
	orders, total, err := h.paymentService.AdminListOrders(c.Request.Context(), userID, service.OrderListParams{
		Page:        page,
		PageSize:    pageSize,
		Status:      c.Query("status"),
		OrderType:   c.Query("order_type"),
		PaymentType: c.Query("payment_type"),
		Keyword:     c.Query("keyword"),
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, enrichAdminPaymentOrdersForResponse(c.Request.Context(), h.paymentService, orders), int64(total), page, pageSize)
}

// GetOrderDetail returns detailed information about a single order.
// GET /api/v1/admin/payment/orders/:id
func (h *PaymentHandler) GetOrderDetail(c *gin.Context) {
	orderID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	order, err := h.paymentService.GetOrderByID(c.Request.Context(), orderID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	auditLogs, _ := h.paymentService.GetOrderAuditLogs(c.Request.Context(), orderID)
	response.Success(c, gin.H{"order": enrichAdminPaymentOrderForResponse(c.Request.Context(), h.paymentService, order), "auditLogs": auditLogs})
}

// CancelOrder cancels a pending order (admin).
// POST /api/v1/admin/payment/orders/:id/cancel
func (h *PaymentHandler) CancelOrder(c *gin.Context) {
	orderID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	msg, err := h.paymentService.AdminCancelOrder(c.Request.Context(), orderID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": msg})
}

// RetryFulfillment retries fulfillment for a paid order.
// POST /api/v1/admin/payment/orders/:id/retry
func (h *PaymentHandler) RetryFulfillment(c *gin.Context) {
	orderID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.paymentService.RetryFulfillment(c.Request.Context(), orderID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "fulfillment retried"})
}

type AdminPaymentOrderResult struct {
	ID                  int64      `json:"id"`
	UserID              int64      `json:"user_id"`
	UserEmail           string     `json:"user_email,omitempty"`
	UserName            string     `json:"user_name,omitempty"`
	UserNotes           *string    `json:"user_notes,omitempty"`
	Amount              float64    `json:"amount"`
	PayAmount           float64    `json:"pay_amount"`
	FeeRate             float64    `json:"fee_rate"`
	Currency            string     `json:"currency"`
	RechargeCode        string     `json:"recharge_code,omitempty"`
	OutTradeNo          string     `json:"out_trade_no"`
	PaymentType         string     `json:"payment_type"`
	PaymentTradeNo      string     `json:"payment_trade_no,omitempty"`
	PayURL              *string    `json:"pay_url,omitempty"`
	QRCode              *string    `json:"qr_code,omitempty"`
	QRCodeImg           *string    `json:"qr_code_img,omitempty"`
	OrderType           string     `json:"order_type"`
	PlanID              *int64     `json:"plan_id,omitempty"`
	SubscriptionGroupID *int64     `json:"subscription_group_id,omitempty"`
	SubscriptionDays    *int       `json:"subscription_days,omitempty"`
	ProviderInstanceID  *string    `json:"provider_instance_id,omitempty"`
	ProviderKey         *string    `json:"provider_key,omitempty"`
	Status              string     `json:"status"`
	RefundAmount        float64    `json:"refund_amount"`
	RefundReason        *string    `json:"refund_reason,omitempty"`
	RefundAt            *time.Time `json:"refund_at,omitempty"`
	ForceRefund         bool       `json:"force_refund,omitempty"`
	RefundRequestedAt   *time.Time `json:"refund_requested_at,omitempty"`
	RefundRequestReason *string    `json:"refund_request_reason,omitempty"`
	RefundRequestedBy   *string    `json:"refund_requested_by,omitempty"`
	ExpiresAt           time.Time  `json:"expires_at"`
	PaidAt              *time.Time `json:"paid_at,omitempty"`
	CompletedAt         *time.Time `json:"completed_at,omitempty"`
	FailedAt            *time.Time `json:"failed_at,omitempty"`
	FailedReason        *string    `json:"failed_reason,omitempty"`
	ClientIP            string     `json:"client_ip,omitempty"`
	SrcHost             string     `json:"src_host,omitempty"`
	SrcURL              *string    `json:"src_url,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

func sanitizeAdminPaymentOrdersForResponse(orders []*dbent.PaymentOrder) []*AdminPaymentOrderResult { //nolint:unused // retained for callers in downstream builds
	out := make([]*AdminPaymentOrderResult, 0, len(orders))
	for _, order := range orders {
		if item := sanitizeAdminPaymentOrderForResponse(order); item != nil {
			out = append(out, item)
		}
	}
	return out
}

func sanitizeAdminPaymentOrderForResponse(order *dbent.PaymentOrder) *AdminPaymentOrderResult {
	if order == nil {
		return nil
	}
	return &AdminPaymentOrderResult{
		ID:                  order.ID,
		UserID:              order.UserID,
		UserEmail:           order.UserEmail,
		UserName:            order.UserName,
		UserNotes:           order.UserNotes,
		Amount:              order.Amount,
		PayAmount:           order.PayAmount,
		FeeRate:             order.FeeRate,
		Currency:            service.PaymentOrderCurrency(order),
		RechargeCode:        order.RechargeCode,
		OutTradeNo:          order.OutTradeNo,
		PaymentType:         order.PaymentType,
		PaymentTradeNo:      order.PaymentTradeNo,
		PayURL:              order.PayURL,
		QRCode:              order.QrCode,
		QRCodeImg:           order.QrCodeImg,
		OrderType:           order.OrderType,
		PlanID:              order.PlanID,
		SubscriptionGroupID: order.SubscriptionGroupID,
		SubscriptionDays:    order.SubscriptionDays,
		ProviderInstanceID:  order.ProviderInstanceID,
		ProviderKey:         order.ProviderKey,
		Status:              order.Status,
		RefundAmount:        order.RefundAmount,
		RefundReason:        order.RefundReason,
		RefundAt:            order.RefundAt,
		ForceRefund:         order.ForceRefund,
		RefundRequestedAt:   order.RefundRequestedAt,
		RefundRequestReason: order.RefundRequestReason,
		RefundRequestedBy:   order.RefundRequestedBy,
		ExpiresAt:           order.ExpiresAt,
		PaidAt:              order.PaidAt,
		CompletedAt:         order.CompletedAt,
		FailedAt:            order.FailedAt,
		FailedReason:        order.FailedReason,
		ClientIP:            order.ClientIP,
		SrcHost:             order.SrcHost,
		SrcURL:              order.SrcURL,
		CreatedAt:           order.CreatedAt,
		UpdatedAt:           order.UpdatedAt,
	}
}

type adminPaymentOrderResponse struct {
	*AdminPaymentOrderResult
	ProductName string `json:"product_name,omitempty"`
	GroupName   string `json:"group_name,omitempty"`
}

func enrichAdminPaymentOrdersForResponse(ctx context.Context, paymentService *service.PaymentService, orders []*dbent.PaymentOrder) []*adminPaymentOrderResponse {
	if len(orders) == 0 {
		return []*adminPaymentOrderResponse{}
	}

	planIDs := make([]int64, 0, len(orders))
	groupIDs := make([]int64, 0, len(orders))
	planSeen := make(map[int64]struct{})
	groupSeen := make(map[int64]struct{})
	for _, order := range orders {
		if order == nil {
			continue
		}
		if order.PlanID != nil {
			if _, ok := planSeen[*order.PlanID]; !ok {
				planSeen[*order.PlanID] = struct{}{}
				planIDs = append(planIDs, *order.PlanID)
			}
		}
		if order.SubscriptionGroupID != nil {
			if _, ok := groupSeen[*order.SubscriptionGroupID]; !ok {
				groupSeen[*order.SubscriptionGroupID] = struct{}{}
				groupIDs = append(groupIDs, *order.SubscriptionGroupID)
			}
		}
	}

	planMap := map[int64]string{}
	groupMap := map[int64]string{}
	if client := paymentService.GetEntClient(); client != nil {
		if len(planIDs) > 0 {
			if plans, err := client.SubscriptionPlan.Query().Where(subscriptionplan.IDIn(planIDs...)).All(ctx); err == nil {
				for _, p := range plans {
					planMap[p.ID] = p.Name
				}
			}
		}
		if len(groupIDs) > 0 {
			if groups, err := client.Group.Query().Where(group.IDIn(groupIDs...)).All(ctx); err == nil {
				for _, g := range groups {
					groupMap[g.ID] = g.Name
				}
			}
		}
	}

	out := make([]*adminPaymentOrderResponse, 0, len(orders))
	for _, order := range orders {
		sanitized := sanitizeAdminPaymentOrderForResponse(order)
		if sanitized == nil {
			continue
		}
		item := &adminPaymentOrderResponse{AdminPaymentOrderResult: sanitized}
		if sanitized.PlanID != nil {
			item.ProductName = planMap[*sanitized.PlanID]
		}
		if sanitized.SubscriptionGroupID != nil {
			item.GroupName = groupMap[*sanitized.SubscriptionGroupID]
		}
		out = append(out, item)
	}
	return out
}

func enrichAdminPaymentOrderForResponse(ctx context.Context, paymentService *service.PaymentService, order *dbent.PaymentOrder) *adminPaymentOrderResponse {
	items := enrichAdminPaymentOrdersForResponse(ctx, paymentService, []*dbent.PaymentOrder{order})
	if len(items) == 0 {
		return nil
	}
	return items[0]
}

// AdminProcessRefundRequest is the request body for admin refund processing.
type AdminProcessRefundRequest struct {
	Amount        float64 `json:"amount"`
	Reason        string  `json:"reason"`
	Force         bool    `json:"force"`
	DeductBalance bool    `json:"deduct_balance"`
	RefundMode    string  `json:"refund_mode"`
}

type AdminRefundPreviewRequest struct {
	Amount     float64 `json:"amount"`
	RefundMode string  `json:"refund_mode"`
}

// PreviewRefund previews refund amount for an order (admin).
// POST /api/v1/admin/payment/orders/:id/refund-preview
func (h *PaymentHandler) PreviewRefund(c *gin.Context) {
	orderID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	var req AdminRefundPreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	amount := req.Amount
	if req.RefundMode == "proportional" {
		amount = 0
	}
	result, err := h.paymentService.PreviewRefund(c.Request.Context(), orderID, amount)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// ProcessRefund processes a refund for an order (admin).
// POST /api/v1/admin/payment/orders/:id/refund
func (h *PaymentHandler) ProcessRefund(c *gin.Context) {
	orderID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	var req AdminProcessRefundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	plan, earlyResult, err := h.paymentService.PrepareRefund(c.Request.Context(), orderID, req.Amount, req.Reason, req.Force, req.DeductBalance)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if earlyResult != nil {
		response.Success(c, earlyResult)
		return
	}

	result, err := h.paymentService.ExecuteRefund(c.Request.Context(), plan)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// QueryAndFinalizeRefund queries the provider refund status and finalizes a pending refund.
// POST /api/v1/admin/payment/orders/:id/refund/query
func (h *PaymentHandler) QueryAndFinalizeRefund(c *gin.Context) {
	orderID, ok := parseIDParam(c, "id")
	if !ok {
		return
	}

	result, err := h.paymentService.QueryAndFinalizeRefund(c.Request.Context(), orderID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// --- Subscription Plans ---

// ListPlans returns all subscription plans.
// GET /api/v1/admin/payment/plans
func (h *PaymentHandler) ListPlans(c *gin.Context) {
	plans, err := h.configService.ListPlans(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	groupInfo := h.configService.GetGroupInfoMap(c.Request.Context(), plans)
	response.Success(c, adminSubscriptionPlansForResponse(plans, groupInfo))
}

type AdminSubscriptionPlanResult struct {
	ID              int64     `json:"id"`
	GroupID         int64     `json:"group_id"`
	GroupPlatform   string    `json:"group_platform,omitempty"`
	GroupName       string    `json:"group_name,omitempty"`
	RateMultiplier  float64   `json:"rate_multiplier,omitempty"`
	DailyLimitUSD   *float64  `json:"daily_limit_usd,omitempty"`
	WeeklyLimitUSD  *float64  `json:"weekly_limit_usd,omitempty"`
	MonthlyLimitUSD *float64  `json:"monthly_limit_usd,omitempty"`
	ModelScopes     []string  `json:"supported_model_scopes,omitempty"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	Price           float64   `json:"price"`
	OriginalPrice   *float64  `json:"original_price,omitempty"`
	Currency        string    `json:"currency,omitempty"`
	ValidityDays    int       `json:"validity_days"`
	ValidityUnit    string    `json:"validity_unit"`
	Features        string    `json:"features"`
	ProductName     string    `json:"product_name"`
	ForSale         bool      `json:"for_sale"`
	SortOrder       int       `json:"sort_order"`
	CreatedAt       time.Time `json:"created_at,omitempty"`
	UpdatedAt       time.Time `json:"updated_at,omitempty"`
}

func adminSubscriptionPlansForResponse(plans []*dbent.SubscriptionPlan, groupInfo map[int64]service.PlanGroupInfo) []AdminSubscriptionPlanResult {
	result := make([]AdminSubscriptionPlanResult, 0, len(plans))
	for _, p := range plans {
		if p == nil {
			continue
		}
		gi := groupInfo[p.GroupID]
		result = append(result, AdminSubscriptionPlanResult{
			ID:              int64(p.ID),
			GroupID:         p.GroupID,
			GroupPlatform:   gi.Platform,
			GroupName:       gi.Name,
			RateMultiplier:  gi.RateMultiplier,
			DailyLimitUSD:   gi.DailyLimitUSD,
			WeeklyLimitUSD:  gi.WeeklyLimitUSD,
			MonthlyLimitUSD: gi.MonthlyLimitUSD,
			ModelScopes:     gi.ModelScopes,
			Name:            p.Name,
			Description:     p.Description,
			Price:           p.Price,
			OriginalPrice:   p.OriginalPrice,
			Currency:        p.Currency,
			ValidityDays:    p.ValidityDays,
			ValidityUnit:    p.ValidityUnit,
			Features:        p.Features,
			ProductName:     p.ProductName,
			ForSale:         p.ForSale,
			SortOrder:       p.SortOrder,
			CreatedAt:       p.CreatedAt,
			UpdatedAt:       p.UpdatedAt,
		})
	}
	return result
}

// CreatePlan creates a new subscription plan.
// POST /api/v1/admin/payment/plans
func (h *PaymentHandler) CreatePlan(c *gin.Context) {
	var req service.CreatePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	plan, err := h.configService.CreatePlan(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, plan)
}

// UpdatePlan updates an existing subscription plan.
// PUT /api/v1/admin/payment/plans/:id
func (h *PaymentHandler) UpdatePlan(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req service.UpdatePlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	plan, err := h.configService.UpdatePlan(c.Request.Context(), id, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, plan)
}

// DeletePlan deletes a subscription plan.
// DELETE /api/v1/admin/payment/plans/:id
func (h *PaymentHandler) DeletePlan(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.configService.DeletePlan(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "deleted"})
}

// --- Provider Instances ---

// ListProviders returns all payment provider instances.
// GET /api/v1/admin/payment/providers
func (h *PaymentHandler) ListProviders(c *gin.Context) {
	providers, err := h.configService.ListProviderInstancesWithConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, providers)
}

// CreateProvider creates a new payment provider instance.
// POST /api/v1/admin/payment/providers
func (h *PaymentHandler) CreateProvider(c *gin.Context) {
	var req service.CreateProviderInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	inst, err := h.configService.CreateProviderInstance(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.paymentService.RefreshProviders(c.Request.Context())
	response.Created(c, inst)
}

// UpdateProvider updates an existing payment provider instance.
// PUT /api/v1/admin/payment/providers/:id
func (h *PaymentHandler) UpdateProvider(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req service.UpdateProviderInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	inst, err := h.configService.UpdateProviderInstance(c.Request.Context(), id, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.paymentService.RefreshProviders(c.Request.Context())
	response.Success(c, inst)
}

// DeleteProvider deletes a payment provider instance.
// DELETE /api/v1/admin/payment/providers/:id
func (h *PaymentHandler) DeleteProvider(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.configService.DeleteProviderInstance(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.paymentService.RefreshProviders(c.Request.Context())
	response.Success(c, gin.H{"message": "deleted"})
}

// parseIDParam parses an int64 path parameter.
// Returns the parsed ID and true on success; on failure it writes a BadRequest response and returns false.
func parseIDParam(c *gin.Context, paramName string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(paramName), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid "+paramName)
		return 0, false
	}
	return id, true
}

// --- Config ---

// GetConfig returns the payment configuration (admin view).
// GET /api/v1/admin/payment/config
func (h *PaymentHandler) GetConfig(c *gin.Context) {
	cfg, err := h.configService.GetPaymentConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

// UpdateConfig updates the payment configuration.
// PUT /api/v1/admin/payment/config
func (h *PaymentHandler) UpdateConfig(c *gin.Context) {
	var req service.UpdatePaymentConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := h.configService.UpdatePaymentConfig(c.Request.Context(), req); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	cfg, err := h.configService.GetPaymentConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, cfg)
}

type CreateCouponRequest struct {
	Code         string  `json:"code"`
	Type         string  `json:"type" binding:"required"`
	Value        float64 `json:"value" binding:"required"`
	MinAmount    float64 `json:"min_amount"`
	MaxDiscount  float64 `json:"max_discount"`
	Scope        string  `json:"scope"`
	MaxUses      int     `json:"max_uses"`
	PerUserLimit int     `json:"per_user_limit"`
	StartsAt     *int64  `json:"starts_at"`
	ExpiresAt    *int64  `json:"expires_at"`
	Notes        string  `json:"notes"`
}

type UpdateCouponRequest struct {
	Code         *string  `json:"code"`
	Type         *string  `json:"type"`
	Value        *float64 `json:"value"`
	MinAmount    *float64 `json:"min_amount"`
	MaxDiscount  *float64 `json:"max_discount"`
	Scope        *string  `json:"scope"`
	MaxUses      *int     `json:"max_uses"`
	PerUserLimit *int     `json:"per_user_limit"`
	Status       *string  `json:"status"`
	StartsAt     *int64   `json:"starts_at"`
	ExpiresAt    *int64   `json:"expires_at"`
	Notes        *string  `json:"notes"`
}

type couponResponse struct {
	ID           int64      `json:"id"`
	Code         string     `json:"code"`
	Type         string     `json:"type"`
	Value        float64    `json:"value"`
	MinAmount    float64    `json:"min_amount"`
	MaxDiscount  float64    `json:"max_discount"`
	Scope        string     `json:"scope"`
	MaxUses      int        `json:"max_uses"`
	UsedCount    int        `json:"used_count"`
	PerUserLimit int        `json:"per_user_limit"`
	Status       string     `json:"status"`
	StartsAt     *time.Time `json:"starts_at,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	Notes        string     `json:"notes"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type couponUsageResponse struct {
	ID             int64     `json:"id"`
	CouponID       int64     `json:"coupon_id"`
	UserID         int64     `json:"user_id"`
	OrderID        int64     `json:"order_id"`
	DiscountAmount float64   `json:"discount_amount"`
	UsedAt         time.Time `json:"used_at"`
	Status         string    `json:"status"`
}

func couponToResponse(c *service.Coupon) couponResponse {
	if c == nil {
		return couponResponse{}
	}
	return couponResponse{
		ID: c.ID, Code: c.Code, Type: c.Type, Value: c.Value,
		MinAmount: c.MinAmount, MaxDiscount: c.MaxDiscount, Scope: c.Scope,
		MaxUses: c.MaxUses, UsedCount: c.UsedCount, PerUserLimit: c.PerUserLimit,
		Status: c.Status, StartsAt: c.StartsAt, ExpiresAt: c.ExpiresAt, Notes: c.Notes,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}

func couponUsageToResponse(u service.CouponUsage) couponUsageResponse {
	return couponUsageResponse{
		ID: u.ID, CouponID: u.CouponID, UserID: u.UserID, OrderID: u.OrderID,
		DiscountAmount: u.DiscountAmount, UsedAt: u.UsedAt, Status: u.Status,
	}
}

func (h *PaymentHandler) requireCouponService(c *gin.Context) (*service.CouponService, bool) {
	if h.couponService == nil {
		response.InternalError(c, "coupon service is not configured")
		return nil, false
	}
	return h.couponService, true
}

// ListCoupons lists payment coupons.
func (h *PaymentHandler) ListCoupons(c *gin.Context) {
	svc, ok := h.requireCouponService(c)
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	status := strings.TrimSpace(c.Query("status"))
	search := strings.TrimSpace(c.Query("search"))
	if len(search) > 100 {
		search = search[:100]
	}
	items, pageResult, err := svc.List(c.Request.Context(), pagination.PaginationParams{Page: page, PageSize: pageSize}, status, search)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]couponResponse, 0, len(items))
	for i := range items {
		out = append(out, couponToResponse(&items[i]))
	}
	response.Paginated(c, out, pageResult.Total, page, pageSize)
}

func (h *PaymentHandler) GetCoupon(c *gin.Context) {
	svc, ok := h.requireCouponService(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid coupon ID")
		return
	}
	item, err := svc.GetByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, couponToResponse(item))
}

func (h *PaymentHandler) CreateCoupon(c *gin.Context) {
	svc, ok := h.requireCouponService(c)
	if !ok {
		return
	}
	var req CreateCouponRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	input := service.CreateCouponInput{
		Code: req.Code, Type: req.Type, Value: req.Value,
		MinAmount: req.MinAmount, MaxDiscount: req.MaxDiscount, Scope: req.Scope,
		MaxUses: req.MaxUses, PerUserLimit: req.PerUserLimit, Notes: req.Notes,
	}
	if req.StartsAt != nil && *req.StartsAt > 0 {
		t := time.Unix(*req.StartsAt, 0)
		input.StartsAt = &t
	}
	if req.ExpiresAt != nil && *req.ExpiresAt > 0 {
		t := time.Unix(*req.ExpiresAt, 0)
		input.ExpiresAt = &t
	}
	item, err := svc.Create(c.Request.Context(), input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, couponToResponse(item))
}

func (h *PaymentHandler) UpdateCoupon(c *gin.Context) {
	svc, ok := h.requireCouponService(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid coupon ID")
		return
	}
	var req UpdateCouponRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	input := service.UpdateCouponInput{
		Code: req.Code, Type: req.Type, Value: req.Value,
		MinAmount: req.MinAmount, MaxDiscount: req.MaxDiscount, Scope: req.Scope,
		MaxUses: req.MaxUses, PerUserLimit: req.PerUserLimit, Status: req.Status,
		Notes: req.Notes,
	}
	if req.StartsAt != nil {
		if *req.StartsAt > 0 {
			t := time.Unix(*req.StartsAt, 0)
			input.StartsAt = &t
		} else {
			input.StartsAt = nil
		}
	}
	if req.ExpiresAt != nil {
		if *req.ExpiresAt > 0 {
			t := time.Unix(*req.ExpiresAt, 0)
			input.ExpiresAt = &t
		} else {
			input.ExpiresAt = nil
		}
	}
	item, err := svc.Update(c.Request.Context(), id, input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, couponToResponse(item))
}

func (h *PaymentHandler) DeleteCoupon(c *gin.Context) {
	svc, ok := h.requireCouponService(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid coupon ID")
		return
	}
	if err := svc.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "Coupon archived successfully"})
}

func (h *PaymentHandler) ListCouponUsages(c *gin.Context) {
	svc, ok := h.requireCouponService(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid coupon ID")
		return
	}
	page, pageSize := response.ParsePagination(c)
	items, pageResult, err := svc.ListUsages(c.Request.Context(), id, pagination.PaginationParams{Page: page, PageSize: pageSize})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]couponUsageResponse, 0, len(items))
	for _, item := range items {
		out = append(out, couponUsageToResponse(item))
	}
	response.Paginated(c, out, pageResult.Total, page, pageSize)
}
