package routes

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/handler/admin"
	ratelimit "github.com/Wei-Shaw/sub2api/internal/middleware"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// publicOrderVerifyRateLimit caps anonymous legacy out_trade_no lookups per
// client IP. The payment result page polls at most a handful of times per
// order, so this leaves ample headroom for real users while making
// out_trade_no enumeration impractical.
const (
	publicOrderVerifyRateLimit       = 20
	publicOrderVerifyRateLimitWindow = time.Minute
)

// RegisterPaymentRoutes registers all payment-related routes:
// user-facing endpoints, webhook endpoints, and admin endpoints.
func RegisterPaymentRoutes(v1 *gin.RouterGroup, paymentHandler *handler.PaymentHandler, args ...any) {
	var invoiceHandler *handler.InvoiceHandler
	var webhookHandler *handler.PaymentWebhookHandler
	var adminPaymentHandler *admin.PaymentHandler
	var adminInvoiceHandler *admin.InvoiceHandler
	var jwtAuth middleware.JWTAuthMiddleware
	var adminAuth middleware.AdminAuthMiddleware
	var auditLog middleware.AuditLogMiddleware
	var settingService *service.SettingService
	var panelRateLimiter *middleware.PanelRateLimiter
	var redisClient *redis.Client
	if len(args) >= 10 {
		invoiceHandler, _ = args[0].(*handler.InvoiceHandler)
		webhookHandler, _ = args[1].(*handler.PaymentWebhookHandler)
		adminPaymentHandler, _ = args[2].(*admin.PaymentHandler)
		adminInvoiceHandler, _ = args[3].(*admin.InvoiceHandler)
		jwtAuth, _ = args[4].(middleware.JWTAuthMiddleware)
		adminAuth, _ = args[5].(middleware.AdminAuthMiddleware)
		auditLog, _ = args[6].(middleware.AuditLogMiddleware)
		settingService, _ = args[7].(*service.SettingService)
		panelRateLimiter, _ = args[8].(*middleware.PanelRateLimiter)
		redisClient, _ = args[9].(*redis.Client)
	} else if len(args) >= 8 {
		webhookHandler, _ = args[0].(*handler.PaymentWebhookHandler)
		adminPaymentHandler, _ = args[1].(*admin.PaymentHandler)
		jwtAuth, _ = args[2].(middleware.JWTAuthMiddleware)
		adminAuth, _ = args[3].(middleware.AdminAuthMiddleware)
		auditLog, _ = args[4].(middleware.AuditLogMiddleware)
		settingService, _ = args[5].(*service.SettingService)
		panelRateLimiter, _ = args[6].(*middleware.PanelRateLimiter)
		redisClient, _ = args[7].(*redis.Client)
	}
	// --- User-facing payment endpoints (authenticated) ---
	authenticated := v1.Group("/payment")
	authenticated.Use(gin.HandlerFunc(jwtAuth))
	authenticated.Use(middleware.BackendModeUserGuard(settingService))
	// 面板全局按用户限流
	authenticated.Use(panelRateLimiter.Global())
	{
		authenticated.GET("/config", paymentHandler.GetPaymentConfig)
		authenticated.GET("/checkout-info", paymentHandler.GetCheckoutInfo)
		authenticated.GET("/plans", paymentHandler.GetPlans)
		authenticated.GET("/limits", paymentHandler.GetLimits)
		authenticated.GET("/discount-rules", paymentHandler.GetDiscountRules)
		authenticated.POST("/preview-price", paymentHandler.PreviewPrice)

		invoiceHeaders := authenticated.Group("/invoice-headers")
		{
			invoiceHeaders.GET("", invoiceHandler.ListInvoiceHeaders)
			invoiceHeaders.POST("", invoiceHandler.CreateInvoiceHeader)
			invoiceHeaders.PUT("/:id", invoiceHandler.UpdateInvoiceHeader)
			invoiceHeaders.DELETE("/:id", invoiceHandler.DeleteInvoiceHeader)
		}
		invoices := authenticated.Group("/invoices")
		{
			invoices.GET("/eligible-orders", invoiceHandler.GetInvoiceApplicationData)
			invoices.POST("", invoiceHandler.CreateInvoiceApplication)
			invoices.GET("", invoiceHandler.ListInvoiceApplications)
			invoices.GET("/:id", invoiceHandler.GetInvoiceApplication)
		}

		orders := authenticated.Group("/orders")
		{
			orders.POST("", paymentHandler.CreateOrder)
			orders.POST("/verify", paymentHandler.VerifyOrder)
			orders.GET("/my", paymentHandler.GetMyOrders)
			orders.GET("/:id", paymentHandler.GetOrder)
			orders.POST("/:id/cancel", paymentHandler.CancelOrder)
			orders.POST("/:id/refund-request", paymentHandler.RequestRefund)
			orders.GET("/refund-eligible-providers", paymentHandler.GetRefundEligibleProviders)
		}
	}

	// --- Public payment endpoints (no auth) ---
	// Signed resume-token recovery is the preferred public lookup path.
	// The legacy anonymous out_trade_no verify endpoint remains available as a
	// persisted-state compatibility path for staggered upgrades.
	// The anonymous verify endpoint is IP rate-limited to prevent out_trade_no
	// enumeration. It fails open on Redis errors so an outage never blocks a
	// user who is mid-payment from seeing their result.
	publicRateLimiter := ratelimit.NewRateLimiter(redisClient)
	public := v1.Group("/payment/public")
	{
		public.POST("/orders/verify",
			publicRateLimiter.Limit("payment-public-order-verify", publicOrderVerifyRateLimit, publicOrderVerifyRateLimitWindow),
			paymentHandler.VerifyOrderPublic)
		public.POST("/orders/resolve", paymentHandler.ResolveOrderPublicByResumeToken)
	}

	// --- Webhook endpoints (no auth) ---
	webhook := v1.Group("/payment/webhook")
	{
		// EasyPay sends GET callbacks with query params
		webhook.GET("/easypay", webhookHandler.EasyPayNotify)
		webhook.POST("/easypay", webhookHandler.EasyPayNotify)
		webhook.POST("/alipay", webhookHandler.AlipayNotify)
		webhook.POST("/wxpay", webhookHandler.WxpayNotify)
		webhook.POST("/stripe", webhookHandler.StripeWebhook)
		webhook.POST("/airwallex", webhookHandler.AirwallexWebhook)
	}

	// --- Admin payment endpoints (admin auth) ---
	adminGroup := v1.Group("/admin/payment")
	adminGroup.Use(gin.HandlerFunc(adminAuth))
	adminGroup.Use(gin.HandlerFunc(auditLog))
	adminGroup.Use(middleware.AdminComplianceGuard(settingService))
	{
		// Dashboard
		adminGroup.GET("/dashboard", adminPaymentHandler.GetDashboard)
		adminGroup.GET("/daily-stats", adminPaymentHandler.GetDailyStats)

		// Config
		adminGroup.GET("/config", adminPaymentHandler.GetConfig)
		adminGroup.PUT("/config", adminPaymentHandler.UpdateConfig)

		invoices := adminGroup.Group("/invoices")
		{
			invoices.GET("/config", adminInvoiceHandler.GetSettings)
			invoices.PUT("/config", adminInvoiceHandler.UpdateSettings)
			invoices.GET("/export", adminInvoiceHandler.ExportApplications)
			invoices.GET("", adminInvoiceHandler.ListApplications)
			invoices.GET("/:id", adminInvoiceHandler.GetApplication)
			invoices.PUT("/:id", adminInvoiceHandler.UpdateApplication)
		}

		// Orders
		adminOrders := adminGroup.Group("/orders")
		{
			adminOrders.GET("", adminPaymentHandler.ListOrders)
			adminOrders.GET("/:id", adminPaymentHandler.GetOrderDetail)
			adminOrders.POST("/:id/cancel", adminPaymentHandler.CancelOrder)
			adminOrders.POST("/:id/retry", adminPaymentHandler.RetryFulfillment)
			adminOrders.POST("/:id/refund-preview", adminPaymentHandler.PreviewRefund)
			adminOrders.POST("/:id/refund", adminPaymentHandler.ProcessRefund)
			adminOrders.POST("/:id/refund/query", adminPaymentHandler.QueryAndFinalizeRefund)
		}

		// Subscription Plans
		plans := adminGroup.Group("/plans")
		{
			plans.GET("", adminPaymentHandler.ListPlans)
			plans.POST("", adminPaymentHandler.CreatePlan)
			plans.PUT("/:id", adminPaymentHandler.UpdatePlan)
			plans.DELETE("/:id", adminPaymentHandler.DeletePlan)
		}

		// Provider Instances
		providers := adminGroup.Group("/providers")
		{
			providers.GET("", adminPaymentHandler.ListProviders)
			providers.POST("", adminPaymentHandler.CreateProvider)
			providers.PUT("/:id", adminPaymentHandler.UpdateProvider)
			providers.DELETE("/:id", adminPaymentHandler.DeleteProvider)
		}

		// Payment coupons
		coupons := adminGroup.Group("/coupons")
		{
			coupons.GET("", adminPaymentHandler.ListCoupons)
			coupons.GET("/:id", adminPaymentHandler.GetCoupon)
			coupons.POST("", adminPaymentHandler.CreateCoupon)
			coupons.PUT("/:id", adminPaymentHandler.UpdateCoupon)
			coupons.DELETE("/:id", adminPaymentHandler.DeleteCoupon)
			coupons.GET("/:id/usages", adminPaymentHandler.ListCouponUsages)
		}
	}
}
