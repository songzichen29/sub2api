package service

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"os"
	"strings"
	"sync"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentproviderinstance"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/payment/provider"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

// --- Order Status Constants ---

const (
	OrderStatusPending           = payment.OrderStatusPending
	OrderStatusPaid              = payment.OrderStatusPaid
	OrderStatusRecharging        = payment.OrderStatusRecharging
	OrderStatusCompleted         = payment.OrderStatusCompleted
	OrderStatusExpired           = payment.OrderStatusExpired
	OrderStatusCancelled         = payment.OrderStatusCancelled
	OrderStatusFailed            = payment.OrderStatusFailed
	OrderStatusRefundRequested   = payment.OrderStatusRefundRequested
	OrderStatusRefunding         = payment.OrderStatusRefunding
	OrderStatusRefundPending     = payment.OrderStatusRefundPending
	OrderStatusPartiallyRefunded = payment.OrderStatusPartiallyRefunded
	OrderStatusRefunded          = payment.OrderStatusRefunded
	OrderStatusRefundFailed      = payment.OrderStatusRefundFailed
)

const (
	// defaultMaxPendingOrders and defaultOrderTimeoutMin are defined in
	// payment_config_service.go alongside other payment configuration defaults.
	paymentGraceMinutes = 5

	defaultPageSize    = 20
	maxPageSize        = 100
	topUsersLimit      = 10
	amountToleranceCNY = 0.01

	orderIDPrefix = "sub2_"
)

const paymentResumeSigningKeyEnv = "PAYMENT_RESUME_SIGNING_KEY"

// --- Types ---

// generateOutTradeNo creates a unique external order ID for payment providers.
// Format: sub2_20250409aB3kX9mQ (prefix + date + 8-char random)
func generateOutTradeNo() string {
	date := time.Now().Format("20060102")
	rnd := generateRandomString(8)
	return orderIDPrefix + date + rnd
}

func generateRandomString(n int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = charset[rand.IntN(len(charset))]
	}
	return string(b)
}

type CreateOrderRequest struct {
	UserID          int64
	Amount          float64
	PaymentType     string
	OpenID          string
	ClientIP        string
	IsMobile        bool
	IsWeChatBrowser bool
	SrcHost         string
	SrcURL          string
	ReturnURL       string
	PaymentSource   string
	OrderType       string
	PlanID          int64
	SubscriptionID  int64
	RenewalMode     string
	CouponCode      string
}

type CreateOrderResponse struct {
	OrderID                       int64                           `json:"order_id"`
	Amount                        float64                         `json:"amount"`
	PayAmount                     float64                         `json:"pay_amount"`
	FeeRate                       float64                         `json:"fee_rate"`
	DiscountAmount                float64                         `json:"discount_amount"`
	CouponCode                    string                          `json:"coupon_code,omitempty"`
	CouponDiscountAmount          float64                         `json:"coupon_discount_amount"`
	Status                        string                          `json:"status"`
	ResultType                    payment.CreatePaymentResultType `json:"result_type,omitempty"`
	PaymentType                   string                          `json:"payment_type"`
	OutTradeNo                    string                          `json:"out_trade_no,omitempty"`
	PayURL                        string                          `json:"pay_url,omitempty"`
	QRCode                        string                          `json:"qr_code,omitempty"`
	ClientSecret                  string                          `json:"client_secret,omitempty"`
	IntentID                      string                          `json:"intent_id,omitempty"`
	Currency                      string                          `json:"currency,omitempty"`
	CountryCode                   string                          `json:"country_code,omitempty"`
	PaymentEnv                    string                          `json:"payment_env,omitempty"`
	OAuth                         *payment.WechatOAuthInfo        `json:"oauth,omitempty"`
	JSAPI                         *payment.WechatJSAPIPayload     `json:"jsapi,omitempty"`
	JSAPIPayload                  *payment.WechatJSAPIPayload     `json:"jsapi_payload,omitempty"`
	ExpiresAt                     time.Time                       `json:"expires_at"`
	PaymentMode                   string                          `json:"payment_mode,omitempty"`
	ResumeToken                   string                          `json:"resume_token,omitempty"`
	AlipayMobilePrecreateDeepLink bool                            `json:"alipay_mobile_precreate_deep_link,omitempty"`
}

type PreviewPriceRequest struct {
	UserID         int64   `json:"-"`
	Amount         float64 `json:"amount"`
	OrderType      string  `json:"order_type"`
	PlanID         int64   `json:"plan_id"`
	SubscriptionID int64   `json:"subscription_id"`
	CouponCode     string  `json:"coupon_code"`
	PaymentType    string  `json:"payment_type"`
}

type CouponInfo struct {
	Code           string  `json:"code"`
	Type           string  `json:"type"`
	Value          float64 `json:"value"`
	DiscountAmount float64 `json:"discount_amount"`
}

type PreviewPriceResponse struct {
	BaseAmount           float64       `json:"base_amount"`
	ThresholdDiscount    float64       `json:"threshold_discount"`
	CouponDiscount       float64       `json:"coupon_discount"`
	AfterDiscount        float64       `json:"after_discount"`
	Fee                  float64       `json:"fee"`
	PayAmount            float64       `json:"pay_amount"`
	FeeRate              float64       `json:"fee_rate"`
	AppliedThresholdRule *DiscountRule `json:"applied_threshold_rule,omitempty"`
	CouponInfo           *CouponInfo   `json:"coupon_info,omitempty"`
}

type OrderListParams struct {
	Page        int
	PageSize    int
	Status      string
	OrderType   string
	PaymentType string
	Keyword     string
}

type RefundPlan struct {
	OrderID         int64
	Order           *dbent.PaymentOrder
	RefundAmount    float64
	GatewayAmount   float64
	Reason          string
	Force           bool
	DeductBalance   bool
	DeductionType   string
	BalanceToDeduct float64
	SubDaysToDeduct int
	SubscriptionID  int64
}

type RefundResult struct {
	Success         bool    `json:"success"`
	Warning         string  `json:"warning,omitempty"`
	RequireForce    bool    `json:"require_force,omitempty"`
	BalanceDeducted float64 `json:"balance_deducted,omitempty"`
	SubDaysDeducted int     `json:"subscription_days_deducted,omitempty"`
}

type RefundPreview struct {
	RefundAmount            float64 `json:"refund_amount"`
	MaxRefundableAmount     float64 `json:"max_refundable_amount"`
	CalculatedAutomatically bool    `json:"calculated_automatically"`
}

type DashboardStats struct {
	TodayAmount   CurrencyAmounts `json:"today_amount"`
	TotalAmount   CurrencyAmounts `json:"total_amount"`
	TodayCount    int             `json:"today_count"`
	TotalCount    int             `json:"total_count"`
	AvgAmount     CurrencyAmounts `json:"avg_amount"`
	PendingOrders int             `json:"pending_orders"`

	DailySeries    []DailyStats        `json:"daily_series"`
	PaymentMethods []PaymentMethodStat `json:"payment_methods"`
	TopUsers       TopUsersByCurrency  `json:"top_users"`
}

// CurrencyAmounts holds payment amounts keyed by their ISO 4217 currency.
// Amounts in different currencies must never be added together.
type CurrencyAmounts map[string]float64

type DailyStats struct {
	Date   string          `json:"date"`
	Amount CurrencyAmounts `json:"amount"`
	Count  int             `json:"count"`
}

type PaymentMethodStat struct {
	Type   string          `json:"type"`
	Amount CurrencyAmounts `json:"amount"`
	Count  int             `json:"count"`
}

type TopUserStat struct {
	UserID int64   `json:"user_id"`
	Email  string  `json:"email"`
	Amount float64 `json:"amount"`
}

// TopUsersByCurrency contains an independent ranked user list for each
// currency. A single cross-currency leaderboard would be misleading.
type TopUsersByCurrency map[string][]TopUserStat

// --- Service ---

type PaymentService struct {
	providerMu               sync.Mutex
	providersLoaded          bool
	entClient                *dbent.Client
	registry                 *payment.Registry
	loadBalancer             payment.LoadBalancer
	redeemService            *RedeemService
	subscriptionSvc          *SubscriptionService
	configService            *PaymentConfigService
	userRepo                 UserRepository
	groupRepo                GroupRepository
	userGroupRateRepo        UserGroupRateRepository
	resumeService            *PaymentResumeService
	affiliateService         *AffiliateService
	discountService          *DiscountService
	couponService            *CouponService
	notificationEmailService *NotificationEmailService
}

func (s *PaymentService) GetEntClient() *dbent.Client {
	return s.entClient
}

func NewPaymentService(entClient *dbent.Client, registry *payment.Registry, loadBalancer payment.LoadBalancer, redeemService *RedeemService, subscriptionSvc *SubscriptionService, configService *PaymentConfigService, userRepo UserRepository, groupRepo GroupRepository, userGroupRateRepo UserGroupRateRepository, affiliateService *AffiliateService) *PaymentService {
	svc := &PaymentService{entClient: entClient, registry: registry, loadBalancer: newVisibleMethodLoadBalancer(loadBalancer, configService), redeemService: redeemService, subscriptionSvc: subscriptionSvc, configService: configService, userRepo: userRepo, groupRepo: groupRepo, userGroupRateRepo: userGroupRateRepo, affiliateService: affiliateService}
	svc.resumeService = psNewPaymentResumeService(configService)
	svc.discountService = NewDiscountService()
	return svc
}

func (s *PaymentService) SetCouponService(couponService *CouponService) {
	s.couponService = couponService
}

func (s *PaymentService) SetDiscountService(discountService *DiscountService) {
	s.discountService = discountService
}

func (s *PaymentService) SetNotificationEmailService(notificationEmailService *NotificationEmailService) {
	s.notificationEmailService = notificationEmailService
}

// --- Provider Registry ---

// EnsureProviders lazily initializes the provider registry on first call.
func (s *PaymentService) EnsureProviders(ctx context.Context) {
	s.providerMu.Lock()
	defer s.providerMu.Unlock()
	if !s.providersLoaded {
		s.loadProviders(ctx)
		s.providersLoaded = true
	}
}

// RefreshProviders clears and re-registers all providers from the database.
func (s *PaymentService) RefreshProviders(ctx context.Context) {
	s.providerMu.Lock()
	defer s.providerMu.Unlock()
	s.registry.Clear()
	s.loadProviders(ctx)
	s.providersLoaded = true
}

func (s *PaymentService) loadProviders(ctx context.Context) {
	instances, err := s.entClient.PaymentProviderInstance.Query().
		Where(paymentproviderinstance.EnabledEQ(true)).
		All(ctx)
	if err != nil {
		slog.Error("[PaymentService] failed to query provider instances", "error", err)
		return
	}
	for _, inst := range instances {
		cfg, err := s.loadBalancer.GetInstanceConfig(ctx, int64(inst.ID))
		if err != nil {
			slog.Warn("[PaymentService] failed to decrypt config for instance", "instanceID", inst.ID, "error", err)
			continue
		}
		if inst.PaymentMode != "" {
			cfg["paymentMode"] = inst.PaymentMode
		}
		instID := fmt.Sprintf("%d", inst.ID)
		p, err := provider.CreateProvider(inst.ProviderKey, instID, cfg)
		if err != nil {
			slog.Warn("[PaymentService] failed to create provider for instance", "instanceID", inst.ID, "key", inst.ProviderKey, "error", err)
			continue
		}
		s.registry.Register(p)
	}
}

// --- Helpers ---

func psIsRefundStatus(s string) bool {
	switch s {
	case OrderStatusRefundRequested, OrderStatusRefunding, OrderStatusRefundPending, OrderStatusPartiallyRefunded, OrderStatusRefunded, OrderStatusRefundFailed:
		return true
	}
	return false
}

func psErrMsg(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func psNilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (s *PaymentService) paymentResume() *PaymentResumeService {
	if s.resumeService != nil {
		return s.resumeService
	}
	return psNewPaymentResumeService(s.configService)
}

func NewLegacyAwarePaymentResumeService(legacyKey []byte) *PaymentResumeService {
	return newLegacyAwarePaymentResumeService(legacyKey)
}

func psNewPaymentResumeService(configService *PaymentConfigService) *PaymentResumeService {
	return newLegacyAwarePaymentResumeService(psResumeLegacyVerificationKey(configService))
}

func newLegacyAwarePaymentResumeService(legacyKey []byte) *PaymentResumeService {
	signingKey, verifyFallbacks := resolvePaymentResumeSigningKeys(legacyKey)
	return NewPaymentResumeService(signingKey, verifyFallbacks...)
}

func psResumeLegacyVerificationKey(configService *PaymentConfigService) []byte {
	if configService == nil {
		return nil
	}
	return configService.encryptionKey
}

func resolvePaymentResumeSigningKeys(legacyKey []byte) ([]byte, [][]byte) {
	signingKey := parsePaymentResumeSigningKey(os.Getenv(paymentResumeSigningKeyEnv))
	if len(signingKey) == 0 {
		if len(legacyKey) == 0 {
			return nil, nil
		}
		return legacyKey, nil
	}
	if len(legacyKey) == 0 || bytes.Equal(legacyKey, signingKey) {
		return signingKey, nil
	}
	return signingKey, [][]byte{legacyKey}
}

func parsePaymentResumeSigningKey(raw string) []byte {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if len(raw) >= 64 && len(raw)%2 == 0 {
		if decoded, err := hex.DecodeString(raw); err == nil && len(decoded) > 0 {
			return decoded
		}
	}
	return []byte(raw)
}

func psSliceContains(sl []string, s string) bool {
	for _, v := range sl {
		if v == s {
			return true
		}
	}
	return false
}

// Subscription validity period unit constants.
const (
	validityUnitWeek   = "week"
	validityUnitWeeks  = "weeks"
	validityUnitMonth  = "month"
	validityUnitMonths = "months"
)

func psComputeValidityDays(days int, unit string) int {
	switch unit {
	case validityUnitWeek, validityUnitWeeks:
		return days * 7
	case validityUnitMonth, validityUnitMonths:
		return days * 30
	default:
		return days
	}
}

func psPlanSubscriptionDays(plan *dbent.SubscriptionPlan, now time.Time) int {
	if plan == nil {
		return 0
	}
	if plan.ExpiresAt != nil {
		if !plan.ExpiresAt.After(now) {
			return 0
		}
		days := int(math.Ceil(plan.ExpiresAt.Sub(now).Hours() / 24))
		if days < 1 {
			return 1
		}
		return days
	}
	return psComputeValidityDays(plan.ValidityDays, plan.ValidityUnit)
}

func psStartOfDayUTC(t time.Time) time.Time {
	return timezone.StartOfDay(t)
}

func applyPagination(pageSize, page int) (size, pg int) {
	size = pageSize
	if size <= 0 {
		size = defaultPageSize
	}
	if size > maxPageSize {
		size = maxPageSize
	}
	pg = page
	if pg < 1 {
		pg = 1
	}
	return size, pg
}
