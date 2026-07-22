package service

import (
	"context"
	"fmt"
	"math"
	"net/mail"
	"sort"
	"strconv"
	"strings"
	"time"

	"entgo.io/ent/dialect"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/invoiceapplication"
	"github.com/Wei-Shaw/sub2api/ent/invoiceapplicationorder"
	"github.com/Wei-Shaw/sub2api/ent/invoiceheader"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	InvoiceDefaultMinAmount = 300.0

	InvoiceOrderStatusUnapplied  = "UNAPPLIED"
	InvoiceOrderStatusProcessing = "PROCESSING"
	InvoiceOrderStatusInvoiced   = "INVOICED"

	InvoiceApplicationStatusPending    = "PENDING"
	InvoiceApplicationStatusProcessing = "PROCESSING"
	InvoiceApplicationStatusInvoiced   = "INVOICED"
	InvoiceApplicationStatusRejected   = "REJECTED"

	InvoiceHeaderTypePersonal = "personal"
	InvoiceHeaderTypeCompany  = "company"
)

// InvoiceService owns ordinary-invoice configuration, applications, and reusable headers.
// Invoices are issued offline; this service only manages the request workflow and audit data.
type InvoiceService struct {
	entClient *dbent.Client
}

func NewInvoiceService(entClient *dbent.Client) *InvoiceService {
	return &InvoiceService{entClient: entClient}
}

type InvoiceSettings struct {
	MinAmount float64 `json:"min_amount"`
}

type InvoiceHeaderInput struct {
	TitleType string `json:"title_type"`
	Title     string `json:"title"`
	TaxNumber string `json:"tax_number"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	Address   string `json:"address"`
	IsDefault bool   `json:"is_default"`
}

type InvoiceHeaderView struct {
	ID        int64     `json:"id"`
	TitleType string    `json:"title_type"`
	Title     string    `json:"title"`
	TaxNumber string    `json:"tax_number"`
	Email     string    `json:"email"`
	Phone     string    `json:"phone"`
	Address   *string   `json:"address,omitempty"`
	IsDefault bool      `json:"is_default"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type InvoiceEligibleOrder struct {
	ID          int64      `json:"id"`
	OrderNo     string     `json:"order_no"`
	OrderType   string     `json:"order_type"`
	Amount      float64    `json:"amount"`
	PaidAt      *time.Time `json:"paid_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type CreateInvoiceApplicationInput struct {
	OrderIDs []int64 `json:"order_ids"`
	HeaderID int64   `json:"header_id"`
}

type InvoiceApplicationOrderView struct {
	OrderID   int64      `json:"order_id"`
	OrderNo   string     `json:"order_no"`
	OrderType string     `json:"order_type"`
	Amount    float64    `json:"amount"`
	PaidAt    *time.Time `json:"paid_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type InvoiceApplicationView struct {
	ID              int64                         `json:"id"`
	ApplicationNo   string                        `json:"application_no"`
	UserID          int64                         `json:"user_id"`
	UserEmail       string                        `json:"user_email,omitempty"`
	UserName        string                        `json:"user_name,omitempty"`
	Status          string                        `json:"status"`
	InvoiceType     string                        `json:"invoice_type"`
	HeaderType      string                        `json:"header_type"`
	HeaderTitle     string                        `json:"header_title"`
	HeaderTaxNumber string                        `json:"header_tax_number"`
	HeaderEmail     string                        `json:"header_email"`
	HeaderPhone     string                        `json:"header_phone"`
	HeaderAddress   *string                       `json:"header_address,omitempty"`
	TotalAmount     float64                       `json:"total_amount"`
	HandledBy       *int64                        `json:"handled_by,omitempty"`
	RejectionReason *string                       `json:"rejection_reason,omitempty"`
	AdminNote       *string                       `json:"admin_note,omitempty"`
	InvoiceNumber   string                        `json:"invoice_number"`
	ProcessedAt     *time.Time                    `json:"processed_at,omitempty"`
	InvoicedAt      *time.Time                    `json:"invoiced_at,omitempty"`
	CreatedAt       time.Time                     `json:"created_at"`
	UpdatedAt       time.Time                     `json:"updated_at"`
	Orders          []InvoiceApplicationOrderView `json:"orders"`
}

type InvoiceApplicationListParams struct {
	Page     int
	PageSize int
	UserID   int64
	Status   string
	Keyword  string
	StartAt  *time.Time
	EndAt    *time.Time
}

type UpdateInvoiceApplicationInput struct {
	Status          string `json:"status"`
	RejectionReason string `json:"rejection_reason"`
	AdminNote       string `json:"admin_note"`
	InvoiceNumber   string `json:"invoice_number"`
}

func (s *InvoiceService) GetSettings(ctx context.Context) (*InvoiceSettings, error) {
	setting, err := s.entClient.InvoiceSetting.Get(ctx, 1)
	if err == nil {
		return &InvoiceSettings{MinAmount: roundInvoiceAmount(setting.MinAmount)}, nil
	}
	if dbent.IsNotFound(err) {
		return &InvoiceSettings{MinAmount: InvoiceDefaultMinAmount}, nil
	}
	return nil, fmt.Errorf("get invoice settings: %w", err)
}

func (s *InvoiceService) UpdateSettings(ctx context.Context, minAmount float64) (*InvoiceSettings, error) {
	if math.IsNaN(minAmount) || math.IsInf(minAmount, 0) || minAmount <= 0 || roundInvoiceAmount(minAmount) != minAmount {
		return nil, infraerrors.BadRequest("INVALID_INVOICE_MIN_AMOUNT", "invoice minimum amount must be a positive amount with at most two decimal places")
	}
	setting, err := s.entClient.InvoiceSetting.Get(ctx, 1)
	if dbent.IsNotFound(err) {
		setting, err = s.entClient.InvoiceSetting.Create().SetMinAmount(minAmount).Save(ctx)
	} else if err == nil {
		setting, err = s.entClient.InvoiceSetting.UpdateOneID(setting.ID).SetMinAmount(minAmount).Save(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("update invoice settings: %w", err)
	}
	return &InvoiceSettings{MinAmount: roundInvoiceAmount(setting.MinAmount)}, nil
}

func (s *InvoiceService) ListHeaders(ctx context.Context, userID int64) ([]InvoiceHeaderView, error) {
	headers, err := s.entClient.InvoiceHeader.Query().
		Where(invoiceheader.UserIDEQ(userID)).
		Order(dbent.Desc(invoiceheader.FieldIsDefault), dbent.Desc(invoiceheader.FieldUpdatedAt), dbent.Desc(invoiceheader.FieldID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list invoice headers: %w", err)
	}
	result := make([]InvoiceHeaderView, 0, len(headers))
	for _, header := range headers {
		result = append(result, invoiceHeaderView(header))
	}
	return result, nil
}

func (s *InvoiceService) CreateHeader(ctx context.Context, userID int64, input InvoiceHeaderInput) (InvoiceHeaderView, error) {
	input, err := normalizeInvoiceHeaderInput(input)
	if err != nil {
		return InvoiceHeaderView{}, err
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return InvoiceHeaderView{}, fmt.Errorf("start invoice header transaction: %w", err)
	}
	defer rollbackInvoiceTx(tx)

	if !input.IsDefault {
		hasHeader, queryErr := tx.InvoiceHeader.Query().Where(invoiceheader.UserIDEQ(userID)).Exist(ctx)
		if queryErr != nil {
			return InvoiceHeaderView{}, fmt.Errorf("check invoice headers: %w", queryErr)
		}
		input.IsDefault = !hasHeader
	}
	if input.IsDefault {
		if _, err = tx.InvoiceHeader.Update().Where(invoiceheader.UserIDEQ(userID)).SetIsDefault(false).Save(ctx); err != nil {
			return InvoiceHeaderView{}, fmt.Errorf("clear default invoice header: %w", err)
		}
	}
	header, err := createInvoiceHeader(ctx, tx, userID, input)
	if err != nil {
		return InvoiceHeaderView{}, err
	}
	if err = tx.Commit(); err != nil {
		return InvoiceHeaderView{}, fmt.Errorf("commit invoice header transaction: %w", err)
	}
	return invoiceHeaderView(header), nil
}

func (s *InvoiceService) UpdateHeader(ctx context.Context, userID, headerID int64, input InvoiceHeaderInput) (InvoiceHeaderView, error) {
	input, err := normalizeInvoiceHeaderInput(input)
	if err != nil {
		return InvoiceHeaderView{}, err
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return InvoiceHeaderView{}, fmt.Errorf("start invoice header transaction: %w", err)
	}
	defer rollbackInvoiceTx(tx)

	header, err := tx.InvoiceHeader.Query().Where(invoiceheader.IDEQ(headerID), invoiceheader.UserIDEQ(userID)).Only(ctx)
	if dbent.IsNotFound(err) {
		return InvoiceHeaderView{}, infraerrors.NotFound("INVOICE_HEADER_NOT_FOUND", "invoice header not found")
	}
	if err != nil {
		return InvoiceHeaderView{}, fmt.Errorf("get invoice header: %w", err)
	}
	if input.IsDefault {
		if _, err = tx.InvoiceHeader.Update().Where(invoiceheader.UserIDEQ(userID), invoiceheader.IDNEQ(header.ID)).SetIsDefault(false).Save(ctx); err != nil {
			return InvoiceHeaderView{}, fmt.Errorf("clear default invoice header: %w", err)
		}
	}
	update := tx.InvoiceHeader.UpdateOneID(header.ID).
		SetTitleType(input.TitleType).
		SetTitle(input.Title).
		SetTaxNumber(input.TaxNumber).
		SetEmail(input.Email).
		SetPhone(input.Phone).
		SetIsDefault(input.IsDefault)
	if input.Address == "" {
		update.ClearAddress()
	} else {
		update.SetAddress(input.Address)
	}
	updated, err := update.Save(ctx)
	if err != nil {
		return InvoiceHeaderView{}, fmt.Errorf("update invoice header: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return InvoiceHeaderView{}, fmt.Errorf("commit invoice header transaction: %w", err)
	}
	return invoiceHeaderView(updated), nil
}

func (s *InvoiceService) DeleteHeader(ctx context.Context, userID, headerID int64) error {
	deleted, err := s.entClient.InvoiceHeader.Delete().Where(invoiceheader.IDEQ(headerID), invoiceheader.UserIDEQ(userID)).Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete invoice header: %w", err)
	}
	if deleted == 0 {
		return infraerrors.NotFound("INVOICE_HEADER_NOT_FOUND", "invoice header not found")
	}
	return nil
}

func (s *InvoiceService) ListEligibleOrders(ctx context.Context, userID int64) ([]InvoiceEligibleOrder, error) {
	orders, err := s.entClient.PaymentOrder.Query().
		Where(invoiceEligibleOrderPredicates(userID)...).
		Order(dbent.Desc(paymentorder.FieldPaidAt), dbent.Desc(paymentorder.FieldID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list invoice eligible orders: %w", err)
	}
	result := make([]InvoiceEligibleOrder, 0, len(orders))
	for _, order := range orders {
		if PaymentOrderCurrency(order) != payment.DefaultPaymentCurrency {
			continue
		}
		result = append(result, InvoiceEligibleOrder{
			ID: order.ID, OrderNo: order.OutTradeNo, OrderType: order.OrderType,
			Amount: roundInvoiceAmount(order.PayAmount), PaidAt: order.PaidAt, CompletedAt: order.CompletedAt,
		})
	}
	return result, nil
}

func (s *InvoiceService) CreateApplication(ctx context.Context, userID int64, input CreateInvoiceApplicationInput) (InvoiceApplicationView, error) {
	orderIDs, err := normalizeInvoiceOrderIDs(input.OrderIDs)
	if err != nil {
		return InvoiceApplicationView{}, err
	}
	if input.HeaderID <= 0 {
		return InvoiceApplicationView{}, infraerrors.BadRequest("INVOICE_HEADER_REQUIRED", "an invoice header is required")
	}
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return InvoiceApplicationView{}, err
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return InvoiceApplicationView{}, fmt.Errorf("start invoice application transaction: %w", err)
	}
	defer rollbackInvoiceTx(tx)

	headerQuery := tx.InvoiceHeader.Query().
		Where(invoiceheader.IDEQ(input.HeaderID), invoiceheader.UserIDEQ(userID))
	if invoiceRowLocksSupported(tx) {
		headerQuery.ForUpdate()
	}
	header, err := headerQuery.Only(ctx)
	if dbent.IsNotFound(err) {
		return InvoiceApplicationView{}, infraerrors.NotFound("INVOICE_HEADER_NOT_FOUND", "invoice header not found")
	}
	if err != nil {
		return InvoiceApplicationView{}, fmt.Errorf("get invoice header: %w", err)
	}
	ordersQuery := tx.PaymentOrder.Query().
		Where(paymentorder.IDIn(orderIDs...), paymentorder.UserIDEQ(userID)).
		Order(dbent.Asc(paymentorder.FieldID))
	if invoiceRowLocksSupported(tx) {
		ordersQuery.ForUpdate()
	}
	orders, err := ordersQuery.All(ctx)
	if err != nil {
		return InvoiceApplicationView{}, fmt.Errorf("get invoice orders: %w", err)
	}
	if len(orders) != len(orderIDs) {
		return InvoiceApplicationView{}, infraerrors.BadRequest("INVOICE_ORDER_INVALID", "one or more selected orders do not belong to you")
	}

	var totalAmount float64
	for _, order := range orders {
		if !isInvoiceEligibleOrder(order) {
			return InvoiceApplicationView{}, infraerrors.Conflict("INVOICE_ORDER_UNAVAILABLE", fmt.Sprintf("order %d is no longer eligible for invoicing", order.ID))
		}
		if PaymentOrderCurrency(order) != payment.DefaultPaymentCurrency {
			return InvoiceApplicationView{}, infraerrors.BadRequest("INVOICE_ORDER_CURRENCY_UNSUPPORTED", fmt.Sprintf("order %d is not paid in CNY", order.ID))
		}
		totalAmount += order.PayAmount
	}
	totalAmount = roundInvoiceAmount(totalAmount)
	if totalAmount < settings.MinAmount {
		return InvoiceApplicationView{}, infraerrors.BadRequest("INVOICE_MIN_AMOUNT_NOT_MET", fmt.Sprintf("selected order total must be at least %.2f", settings.MinAmount))
	}

	application, err := tx.InvoiceApplication.Create().
		SetUserID(userID).
		SetStatus(InvoiceApplicationStatusPending).
		SetInvoiceType("ordinary").
		SetHeaderType(header.TitleType).
		SetHeaderTitle(header.Title).
		SetHeaderTaxNumber(header.TaxNumber).
		SetHeaderEmail(header.Email).
		SetHeaderPhone(header.Phone).
		SetNillableHeaderAddress(header.Address).
		SetTotalAmount(totalAmount).
		Save(ctx)
	if err != nil {
		return InvoiceApplicationView{}, fmt.Errorf("create invoice application: %w", err)
	}

	updated, err := tx.PaymentOrder.Update().
		Where(invoiceEligibleOrderPredicates(userID)...).
		Where(paymentorder.IDIn(orderIDs...)).
		SetInvoiceStatus(InvoiceOrderStatusProcessing).
		SetInvoiceApplicationID(application.ID).
		Save(ctx)
	if err != nil {
		return InvoiceApplicationView{}, fmt.Errorf("lock invoice orders: %w", err)
	}
	if updated != len(orderIDs) {
		return InvoiceApplicationView{}, infraerrors.Conflict("INVOICE_ORDER_UNAVAILABLE", "one or more selected orders were changed by another request")
	}
	for _, order := range orders {
		if _, err := tx.InvoiceApplicationOrder.Create().
			SetApplicationID(application.ID).
			SetOrderID(order.ID).
			SetOrderNo(order.OutTradeNo).
			SetOrderType(order.OrderType).
			SetAmount(roundInvoiceAmount(order.PayAmount)).
			SetNillablePaidAt(order.PaidAt).
			Save(ctx); err != nil {
			return InvoiceApplicationView{}, fmt.Errorf("create invoice application order: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return InvoiceApplicationView{}, fmt.Errorf("commit invoice application transaction: %w", err)
	}
	return s.GetUserApplication(ctx, userID, application.ID)
}

func (s *InvoiceService) ListUserApplications(ctx context.Context, userID int64, page, pageSize int, status string) ([]InvoiceApplicationView, int, error) {
	params := InvoiceApplicationListParams{Page: page, PageSize: pageSize, UserID: userID, Status: status}
	return s.listApplications(ctx, params, false)
}

func (s *InvoiceService) GetUserApplication(ctx context.Context, userID, applicationID int64) (InvoiceApplicationView, error) {
	application, err := s.entClient.InvoiceApplication.Query().
		Where(invoiceapplication.IDEQ(applicationID), invoiceapplication.UserIDEQ(userID)).
		WithOrders(func(q *dbent.InvoiceApplicationOrderQuery) { q.Order(dbent.Asc(invoiceapplicationorder.FieldID)) }).
		Only(ctx)
	if dbent.IsNotFound(err) {
		return InvoiceApplicationView{}, infraerrors.NotFound("INVOICE_APPLICATION_NOT_FOUND", "invoice application not found")
	}
	if err != nil {
		return InvoiceApplicationView{}, fmt.Errorf("get invoice application: %w", err)
	}
	return invoiceApplicationView(application), nil
}

func (s *InvoiceService) ListAdminApplications(ctx context.Context, params InvoiceApplicationListParams) ([]InvoiceApplicationView, int, error) {
	return s.listApplications(ctx, params, true)
}

func (s *InvoiceService) GetAdminApplication(ctx context.Context, applicationID int64) (InvoiceApplicationView, error) {
	application, err := s.entClient.InvoiceApplication.Query().
		Where(invoiceapplication.IDEQ(applicationID)).
		WithUser().
		WithOrders(func(q *dbent.InvoiceApplicationOrderQuery) { q.Order(dbent.Asc(invoiceapplicationorder.FieldID)) }).
		Only(ctx)
	if dbent.IsNotFound(err) {
		return InvoiceApplicationView{}, infraerrors.NotFound("INVOICE_APPLICATION_NOT_FOUND", "invoice application not found")
	}
	if err != nil {
		return InvoiceApplicationView{}, fmt.Errorf("get invoice application: %w", err)
	}
	return invoiceApplicationView(application), nil
}

func (s *InvoiceService) UpdateApplication(ctx context.Context, applicationID, adminUserID int64, input UpdateInvoiceApplicationInput) (InvoiceApplicationView, error) {
	input, err := normalizeInvoiceApplicationUpdate(input)
	if err != nil {
		return InvoiceApplicationView{}, err
	}
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return InvoiceApplicationView{}, fmt.Errorf("start invoice application transaction: %w", err)
	}
	defer rollbackInvoiceTx(tx)

	applicationQuery := tx.InvoiceApplication.Query().Where(invoiceapplication.IDEQ(applicationID))
	if invoiceRowLocksSupported(tx) {
		applicationQuery.ForUpdate()
	}
	application, err := applicationQuery.Only(ctx)
	if dbent.IsNotFound(err) {
		return InvoiceApplicationView{}, infraerrors.NotFound("INVOICE_APPLICATION_NOT_FOUND", "invoice application not found")
	}
	if err != nil {
		return InvoiceApplicationView{}, fmt.Errorf("get invoice application: %w", err)
	}
	if !canTransitionInvoiceApplication(application.Status, input.Status) {
		return InvoiceApplicationView{}, infraerrors.Conflict("INVALID_INVOICE_STATUS_TRANSITION", "invoice application status cannot be changed this way")
	}
	now := time.Now()
	update := tx.InvoiceApplication.UpdateOneID(application.ID).
		SetStatus(input.Status).
		SetHandledBy(adminUserID).
		SetProcessedAt(now).
		SetAdminNote(input.AdminNote)
	switch input.Status {
	case InvoiceApplicationStatusProcessing:
		update.ClearRejectionReason().ClearInvoicedAt().SetInvoiceNumber("")
	case InvoiceApplicationStatusRejected:
		update.SetRejectionReason(input.RejectionReason).ClearInvoicedAt().SetInvoiceNumber("")
	case InvoiceApplicationStatusInvoiced:
		update.ClearRejectionReason().SetInvoiceNumber(input.InvoiceNumber).SetInvoicedAt(now)
	}
	application, err = update.Save(ctx)
	if err != nil {
		return InvoiceApplicationView{}, fmt.Errorf("update invoice application: %w", err)
	}

	switch input.Status {
	case InvoiceApplicationStatusRejected:
		expectedOrders, countErr := tx.InvoiceApplicationOrder.Query().
			Where(invoiceapplicationorder.ApplicationIDEQ(applicationID)).
			Count(ctx)
		if countErr != nil {
			return InvoiceApplicationView{}, fmt.Errorf("count invoice application orders: %w", countErr)
		}
		if expectedOrders == 0 {
			return InvoiceApplicationView{}, infraerrors.Conflict("INVOICE_ORDER_STATE_INVALID", "invoice application has no order snapshots")
		}
		updated, releaseErr := tx.PaymentOrder.Update().
			Where(paymentorder.InvoiceApplicationIDEQ(applicationID), paymentorder.InvoiceStatusEQ(InvoiceOrderStatusProcessing)).
			SetInvoiceStatus(InvoiceOrderStatusUnapplied).
			ClearInvoiceApplicationID().
			Save(ctx)
		if releaseErr != nil {
			return InvoiceApplicationView{}, fmt.Errorf("release invoice orders: %w", releaseErr)
		}
		if updated != expectedOrders {
			return InvoiceApplicationView{}, infraerrors.Conflict("INVOICE_ORDER_STATE_INVALID", "invoice orders are no longer in a releasable state")
		}
	case InvoiceApplicationStatusInvoiced:
		expectedOrders, countErr := tx.InvoiceApplicationOrder.Query().
			Where(invoiceapplicationorder.ApplicationIDEQ(applicationID)).
			Count(ctx)
		if countErr != nil {
			return InvoiceApplicationView{}, fmt.Errorf("count invoice application orders: %w", countErr)
		}
		if expectedOrders == 0 {
			return InvoiceApplicationView{}, infraerrors.Conflict("INVOICE_ORDER_STATE_INVALID", "invoice application has no order snapshots")
		}
		updated, updateErr := tx.PaymentOrder.Update().
			Where(paymentorder.InvoiceApplicationIDEQ(applicationID), paymentorder.InvoiceStatusEQ(InvoiceOrderStatusProcessing)).
			SetInvoiceStatus(InvoiceOrderStatusInvoiced).
			Save(ctx)
		if updateErr != nil {
			return InvoiceApplicationView{}, fmt.Errorf("complete invoice orders: %w", updateErr)
		}
		if updated != expectedOrders {
			return InvoiceApplicationView{}, infraerrors.Conflict("INVOICE_ORDER_STATE_INVALID", "invoice orders are no longer in a processable state")
		}
	}
	if err = tx.Commit(); err != nil {
		return InvoiceApplicationView{}, fmt.Errorf("commit invoice application transaction: %w", err)
	}
	return s.GetAdminApplication(ctx, applicationID)
}

func invoiceRowLocksSupported(tx *dbent.Tx) bool {
	return tx != nil && tx.Client().Driver().Dialect() != dialect.SQLite
}

func (s *InvoiceService) listApplications(ctx context.Context, params InvoiceApplicationListParams, includeUser bool) ([]InvoiceApplicationView, int, error) {
	page, pageSize := normalizeInvoicePagination(params.Page, params.PageSize)
	predicates := invoiceApplicationPredicates(params)
	query := s.entClient.InvoiceApplication.Query().Where(predicates...)
	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count invoice applications: %w", err)
	}
	query = s.entClient.InvoiceApplication.Query().
		Where(predicates...).
		Order(dbent.Desc(invoiceapplication.FieldCreatedAt), dbent.Desc(invoiceapplication.FieldID)).
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		WithOrders(func(q *dbent.InvoiceApplicationOrderQuery) { q.Order(dbent.Asc(invoiceapplicationorder.FieldID)) })
	if includeUser {
		query.WithUser()
	}
	applications, err := query.All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list invoice applications: %w", err)
	}
	result := make([]InvoiceApplicationView, 0, len(applications))
	for _, application := range applications {
		result = append(result, invoiceApplicationView(application))
	}
	return result, total, nil
}

func invoiceApplicationPredicates(params InvoiceApplicationListParams) []predicate.InvoiceApplication {
	predicates := make([]predicate.InvoiceApplication, 0, 5)
	if params.UserID > 0 {
		predicates = append(predicates, invoiceapplication.UserIDEQ(params.UserID))
	}
	if status := strings.ToUpper(strings.TrimSpace(params.Status)); status != "" {
		predicates = append(predicates, invoiceapplication.StatusEQ(status))
	}
	if params.StartAt != nil {
		predicates = append(predicates, invoiceapplication.CreatedAtGTE(*params.StartAt))
	}
	if params.EndAt != nil {
		predicates = append(predicates, invoiceapplication.CreatedAtLT(*params.EndAt))
	}
	if keyword := strings.TrimSpace(params.Keyword); keyword != "" {
		matches := []predicate.InvoiceApplication{
			invoiceapplication.HasUserWith(user.EmailContainsFold(keyword)),
			invoiceapplication.HasUserWith(user.UsernameContainsFold(keyword)),
			invoiceapplication.HasOrdersWith(invoiceapplicationorder.OrderNoContainsFold(keyword)),
		}
		if id, ok := parseInvoiceKeywordID(keyword); ok {
			matches = append(matches, invoiceapplication.IDEQ(id))
		}
		predicates = append(predicates, invoiceapplication.Or(matches...))
	}
	return predicates
}

func normalizeInvoiceHeaderInput(input InvoiceHeaderInput) (InvoiceHeaderInput, error) {
	input.TitleType = strings.ToLower(strings.TrimSpace(input.TitleType))
	input.Title = strings.TrimSpace(input.Title)
	input.TaxNumber = strings.TrimSpace(input.TaxNumber)
	input.Email = strings.TrimSpace(input.Email)
	input.Phone = strings.TrimSpace(input.Phone)
	input.Address = strings.TrimSpace(input.Address)
	if input.TitleType != InvoiceHeaderTypePersonal && input.TitleType != InvoiceHeaderTypeCompany {
		return InvoiceHeaderInput{}, infraerrors.BadRequest("INVALID_INVOICE_HEADER_TYPE", "invoice header type must be personal or company")
	}
	if input.Title == "" {
		return InvoiceHeaderInput{}, infraerrors.BadRequest("INVOICE_HEADER_TITLE_REQUIRED", "invoice title is required")
	}
	if input.TitleType == InvoiceHeaderTypeCompany && input.TaxNumber == "" {
		return InvoiceHeaderInput{}, infraerrors.BadRequest("INVOICE_TAX_NUMBER_REQUIRED", "company invoice headers require a tax number")
	}
	if input.Email != "" {
		if _, err := mail.ParseAddress(input.Email); err != nil {
			return InvoiceHeaderInput{}, infraerrors.BadRequest("INVALID_INVOICE_EMAIL", "invoice email is invalid")
		}
	}
	return input, nil
}

func createInvoiceHeader(ctx context.Context, tx *dbent.Tx, userID int64, input InvoiceHeaderInput) (*dbent.InvoiceHeader, error) {
	create := tx.InvoiceHeader.Create().
		SetUserID(userID).
		SetTitleType(input.TitleType).
		SetTitle(input.Title).
		SetTaxNumber(input.TaxNumber).
		SetEmail(input.Email).
		SetPhone(input.Phone).
		SetIsDefault(input.IsDefault)
	if input.Address != "" {
		create.SetAddress(input.Address)
	}
	header, err := create.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create invoice header: %w", err)
	}
	return header, nil
}

func normalizeInvoiceOrderIDs(orderIDs []int64) ([]int64, error) {
	if len(orderIDs) == 0 {
		return nil, infraerrors.BadRequest("INVOICE_ORDERS_REQUIRED", "select at least one order")
	}
	seen := make(map[int64]struct{}, len(orderIDs))
	unique := make([]int64, 0, len(orderIDs))
	for _, orderID := range orderIDs {
		if orderID <= 0 {
			return nil, infraerrors.BadRequest("INVALID_INVOICE_ORDER", "invoice order id must be positive")
		}
		if _, exists := seen[orderID]; exists {
			continue
		}
		seen[orderID] = struct{}{}
		unique = append(unique, orderID)
	}
	sort.Slice(unique, func(i, j int) bool { return unique[i] < unique[j] })
	return unique, nil
}

func isInvoiceEligibleOrder(order *dbent.PaymentOrder) bool {
	return order != nil &&
		order.Status == OrderStatusCompleted &&
		order.RefundAmount == 0 &&
		order.InvoiceStatus == InvoiceOrderStatusUnapplied &&
		order.InvoiceApplicationID == nil
}

func invoiceEligibleOrderPredicates(userID int64) []predicate.PaymentOrder {
	return []predicate.PaymentOrder{
		paymentorder.UserIDEQ(userID),
		paymentorder.StatusEQ(OrderStatusCompleted),
		paymentorder.RefundAmountEQ(0),
		paymentorder.InvoiceStatusEQ(InvoiceOrderStatusUnapplied),
		paymentorder.InvoiceApplicationIDIsNil(),
	}
}

func normalizeInvoiceApplicationUpdate(input UpdateInvoiceApplicationInput) (UpdateInvoiceApplicationInput, error) {
	input.Status = strings.ToUpper(strings.TrimSpace(input.Status))
	input.RejectionReason = strings.TrimSpace(input.RejectionReason)
	input.AdminNote = strings.TrimSpace(input.AdminNote)
	input.InvoiceNumber = strings.TrimSpace(input.InvoiceNumber)
	if input.Status != InvoiceApplicationStatusProcessing && input.Status != InvoiceApplicationStatusInvoiced && input.Status != InvoiceApplicationStatusRejected {
		return UpdateInvoiceApplicationInput{}, infraerrors.BadRequest("INVALID_INVOICE_APPLICATION_STATUS", "invoice application status must be PROCESSING, INVOICED, or REJECTED")
	}
	if input.Status == InvoiceApplicationStatusRejected && input.RejectionReason == "" {
		return UpdateInvoiceApplicationInput{}, infraerrors.BadRequest("INVOICE_REJECTION_REASON_REQUIRED", "a rejection reason is required")
	}
	if input.Status == InvoiceApplicationStatusInvoiced && input.InvoiceNumber == "" {
		return UpdateInvoiceApplicationInput{}, infraerrors.BadRequest("INVOICE_NUMBER_REQUIRED", "invoice number is required")
	}
	if len(input.InvoiceNumber) > 128 {
		return UpdateInvoiceApplicationInput{}, infraerrors.BadRequest("INVOICE_NUMBER_TOO_LONG", "invoice number is too long")
	}
	return input, nil
}

func canTransitionInvoiceApplication(current, next string) bool {
	if current == next {
		return next == InvoiceApplicationStatusProcessing
	}
	switch current {
	case InvoiceApplicationStatusPending:
		return next == InvoiceApplicationStatusProcessing || next == InvoiceApplicationStatusInvoiced || next == InvoiceApplicationStatusRejected
	case InvoiceApplicationStatusProcessing:
		return next == InvoiceApplicationStatusInvoiced || next == InvoiceApplicationStatusRejected
	default:
		return false
	}
}

func invoiceHeaderView(header *dbent.InvoiceHeader) InvoiceHeaderView {
	return InvoiceHeaderView{
		ID: header.ID, TitleType: header.TitleType, Title: header.Title, TaxNumber: header.TaxNumber,
		Email: header.Email, Phone: header.Phone, Address: header.Address, IsDefault: header.IsDefault,
		CreatedAt: header.CreatedAt, UpdatedAt: header.UpdatedAt,
	}
}

func invoiceApplicationView(application *dbent.InvoiceApplication) InvoiceApplicationView {
	view := InvoiceApplicationView{
		ID: application.ID, ApplicationNo: fmt.Sprintf("INV-%06d", application.ID), UserID: application.UserID,
		Status: application.Status, InvoiceType: application.InvoiceType, HeaderType: application.HeaderType,
		HeaderTitle: application.HeaderTitle, HeaderTaxNumber: application.HeaderTaxNumber,
		HeaderEmail: application.HeaderEmail, HeaderPhone: application.HeaderPhone, HeaderAddress: application.HeaderAddress,
		TotalAmount: roundInvoiceAmount(application.TotalAmount), HandledBy: application.HandledBy,
		RejectionReason: application.RejectionReason, AdminNote: application.AdminNote, InvoiceNumber: application.InvoiceNumber,
		ProcessedAt: application.ProcessedAt, InvoicedAt: application.InvoicedAt,
		CreatedAt: application.CreatedAt, UpdatedAt: application.UpdatedAt,
		Orders: make([]InvoiceApplicationOrderView, 0, len(application.Edges.Orders)),
	}
	if application.Edges.User != nil {
		view.UserEmail = application.Edges.User.Email
		view.UserName = application.Edges.User.Username
	}
	for _, order := range application.Edges.Orders {
		view.Orders = append(view.Orders, InvoiceApplicationOrderView{
			OrderID: order.OrderID, OrderNo: order.OrderNo, OrderType: order.OrderType,
			Amount: roundInvoiceAmount(order.Amount), PaidAt: order.PaidAt, CreatedAt: order.CreatedAt,
		})
	}
	return view
}

func normalizeInvoicePagination(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 10000 {
		pageSize = 10000
	}
	return page, pageSize
}

func roundInvoiceAmount(amount float64) float64 {
	return math.Round(amount*100) / 100
}

func rollbackInvoiceTx(tx *dbent.Tx) {
	if tx != nil {
		_ = tx.Rollback()
	}
}

func parseInvoiceKeywordID(keyword string) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(keyword), 10, 64)
	return id, err == nil && id > 0
}
