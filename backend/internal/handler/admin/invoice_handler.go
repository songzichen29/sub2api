package admin

import (
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// InvoiceHandler exposes administrator controls for offline ordinary invoices.
type InvoiceHandler struct {
	invoiceService *service.InvoiceService
}

func NewInvoiceHandler(invoiceService *service.InvoiceService) *InvoiceHandler {
	return &InvoiceHandler{invoiceService: invoiceService}
}

// GetSettings returns the current invoice minimum amount.
// GET /api/v1/admin/payment/invoices/config
func (h *InvoiceHandler) GetSettings(c *gin.Context) {
	settings, err := h.invoiceService.GetSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}

type updateInvoiceSettingsRequest struct {
	MinAmount float64 `json:"min_amount"`
}

// UpdateSettings changes the invoice application threshold.
// PUT /api/v1/admin/payment/invoices/config
func (h *InvoiceHandler) UpdateSettings(c *gin.Context) {
	var req updateInvoiceSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	settings, err := h.invoiceService.UpdateSettings(c.Request.Context(), req.MinAmount)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}

// ListApplications returns invoice applications with administrator filters.
// GET /api/v1/admin/payment/invoices
func (h *InvoiceHandler) ListApplications(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	params, ok := parseInvoiceApplicationListParams(c, page, pageSize)
	if !ok {
		return
	}
	applications, total, err := h.invoiceService.ListAdminApplications(c.Request.Context(), params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, applications, int64(total), page, pageSize)
}

// GetApplication returns an invoice application and all selected order snapshots.
// GET /api/v1/admin/payment/invoices/:id
func (h *InvoiceHandler) GetApplication(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	application, err := h.invoiceService.GetAdminApplication(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, application)
}

// UpdateApplication records the administrator's offline processing result.
// PUT /api/v1/admin/payment/invoices/:id
func (h *InvoiceHandler) UpdateApplication(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req service.UpdateInvoiceApplicationInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "Administrator not authenticated")
		return
	}
	application, err := h.invoiceService.UpdateApplication(c.Request.Context(), id, subject.UserID, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, application)
}

// ExportApplications exports the currently filtered applications as CSV.
// GET /api/v1/admin/payment/invoices/export
func (h *InvoiceHandler) ExportApplications(c *gin.Context) {
	params, ok := parseInvoiceApplicationListParams(c, 1, 10000)
	if !ok {
		return
	}
	applications, _, err := h.invoiceService.ListAdminApplications(c.Request.Context(), params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=invoice-applications-%s.csv", time.Now().Format("20060102")))
	writer := csv.NewWriter(c.Writer)
	_ = writer.Write([]string{"application_no", "user_id", "user_email", "status", "title", "tax_number", "amount", "order_nos", "invoice_number", "rejection_reason", "created_at", "processed_at", "invoiced_at"})
	for _, application := range applications {
		orderNos := make([]string, 0, len(application.Orders))
		for _, order := range application.Orders {
			orderNos = append(orderNos, order.OrderNo)
		}
		_ = writer.Write([]string{
			application.ApplicationNo,
			strconv.FormatInt(application.UserID, 10),
			application.UserEmail,
			application.Status,
			application.HeaderTitle,
			application.HeaderTaxNumber,
			fmt.Sprintf("%.2f", application.TotalAmount),
			strings.Join(orderNos, ","),
			application.InvoiceNumber,
			stringValue(application.RejectionReason),
			application.CreatedAt.Format(time.RFC3339),
			formatInvoiceTime(application.ProcessedAt),
			formatInvoiceTime(application.InvoicedAt),
		})
	}
	writer.Flush()
}

func parseInvoiceApplicationListParams(c *gin.Context, page, pageSize int) (service.InvoiceApplicationListParams, bool) {
	params := service.InvoiceApplicationListParams{
		Page: page, PageSize: pageSize, Status: c.Query("status"), Keyword: c.Query("keyword"),
	}
	if rawUserID := strings.TrimSpace(c.Query("user_id")); rawUserID != "" {
		userID, err := strconv.ParseInt(rawUserID, 10, 64)
		if err != nil || userID <= 0 {
			response.BadRequest(c, "Invalid user_id")
			return service.InvoiceApplicationListParams{}, false
		}
		params.UserID = userID
	}
	startAt, endAt, err := parseInvoiceDateRange(c.Query("start_date"), c.Query("end_date"))
	if err != nil {
		response.BadRequest(c, err.Error())
		return service.InvoiceApplicationListParams{}, false
	}
	params.StartAt = startAt
	params.EndAt = endAt
	return params, true
}

func parseInvoiceDateRange(startRaw, endRaw string) (*time.Time, *time.Time, error) {
	var startAt, endAt *time.Time
	if startRaw = strings.TrimSpace(startRaw); startRaw != "" {
		parsed, err := time.Parse("2006-01-02", startRaw)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid start_date")
		}
		startAt = &parsed
	}
	if endRaw = strings.TrimSpace(endRaw); endRaw != "" {
		parsed, err := time.Parse("2006-01-02", endRaw)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid end_date")
		}
		parsed = parsed.AddDate(0, 0, 1)
		endAt = &parsed
	}
	if startAt != nil && endAt != nil && !endAt.After(*startAt) {
		return nil, nil, fmt.Errorf("end_date must not be before start_date")
	}
	return startAt, endAt, nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func formatInvoiceTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format(time.RFC3339)
}
