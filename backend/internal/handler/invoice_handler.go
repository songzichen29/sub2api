package handler

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// InvoiceHandler exposes the authenticated user's offline invoice workflow.
type InvoiceHandler struct {
	invoiceService *service.InvoiceService
}

func NewInvoiceHandler(invoiceService *service.InvoiceService) *InvoiceHandler {
	return &InvoiceHandler{invoiceService: invoiceService}
}

// GetInvoiceApplicationData returns the current threshold and the user's selectable orders.
// GET /api/v1/payment/invoices/eligible-orders
func (h *InvoiceHandler) GetInvoiceApplicationData(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	settings, err := h.invoiceService.GetSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	orders, err := h.invoiceService.ListEligibleOrders(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"min_amount": settings.MinAmount, "orders": orders})
}

// ListInvoiceHeaders returns reusable invoice headers belonging to the current user.
// GET /api/v1/payment/invoice-headers
func (h *InvoiceHandler) ListInvoiceHeaders(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	headers, err := h.invoiceService.ListHeaders(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, headers)
}

// CreateInvoiceHeader creates a reusable invoice header.
// POST /api/v1/payment/invoice-headers
func (h *InvoiceHandler) CreateInvoiceHeader(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	var req service.InvoiceHeaderInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	header, err := h.invoiceService.CreateHeader(c.Request.Context(), subject.UserID, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, header)
}

// UpdateInvoiceHeader updates a header owned by the current user.
// PUT /api/v1/payment/invoice-headers/:id
func (h *InvoiceHandler) UpdateInvoiceHeader(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	headerID, ok := parseInvoiceID(c, "id")
	if !ok {
		return
	}
	var req service.InvoiceHeaderInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	header, err := h.invoiceService.UpdateHeader(c.Request.Context(), subject.UserID, headerID, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, header)
}

// DeleteInvoiceHeader removes a reusable header. Existing applications retain their snapshots.
// DELETE /api/v1/payment/invoice-headers/:id
func (h *InvoiceHandler) DeleteInvoiceHeader(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	headerID, ok := parseInvoiceID(c, "id")
	if !ok {
		return
	}
	if err := h.invoiceService.DeleteHeader(c.Request.Context(), subject.UserID, headerID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "deleted"})
}

// CreateInvoiceApplication locks the selected completed orders and creates an offline invoice request.
// POST /api/v1/payment/invoices
func (h *InvoiceHandler) CreateInvoiceApplication(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	var req service.CreateInvoiceApplicationInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	application, err := h.invoiceService.CreateApplication(c.Request.Context(), subject.UserID, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, application)
}

// ListInvoiceApplications returns the current user's invoice history.
// GET /api/v1/payment/invoices
func (h *InvoiceHandler) ListInvoiceApplications(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	applications, total, err := h.invoiceService.ListUserApplications(c.Request.Context(), subject.UserID, page, pageSize, c.Query("status"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, applications, int64(total), page, pageSize)
}

// GetInvoiceApplication returns one application owned by the current user.
// GET /api/v1/payment/invoices/:id
func (h *InvoiceHandler) GetInvoiceApplication(c *gin.Context) {
	subject, ok := requireAuth(c)
	if !ok {
		return
	}
	applicationID, ok := parseInvoiceID(c, "id")
	if !ok {
		return
	}
	application, err := h.invoiceService.GetUserApplication(c.Request.Context(), subject.UserID, applicationID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, application)
}

func parseInvoiceID(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid "+name)
		return 0, false
	}
	return id, true
}
