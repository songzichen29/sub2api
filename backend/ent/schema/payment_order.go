package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PaymentOrder holds the schema definition for the PaymentOrder entity.
//
// 删除策略：硬删除
// PaymentOrder 使用硬删除而非软删除，原因如下：
//   - 订单通过 status 字段追踪完整生命周期，无需依赖软删除
//   - 订单审计通过 PaymentAuditLog 表记录，删除前可归档
//   - 减少查询复杂度，避免软删除过滤开销
type PaymentOrder struct {
	ent.Schema
}

func (PaymentOrder) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "payment_orders"},
	}
}

func (PaymentOrder) Fields() []ent.Field {
	return []ent.Field{
		// 用户信息（冗余存储，避免关联查询）
		field.Int64("user_id"),
		field.String("user_email").
			MaxLen(255),
		field.String("user_name").
			MaxLen(100),
		field.String("user_notes").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "longtext"}),

		// 金额信息
		field.Float("amount").
			SchemaType(map[string]string{dialect.MySQL: "decimal(20,2)"}),
		field.Float("pay_amount").
			SchemaType(map[string]string{dialect.MySQL: "decimal(20,2)"}),
		field.Float("fee_rate").
			SchemaType(map[string]string{dialect.MySQL: "decimal(10,4)"}).
			Default(0),
		field.Float("discount_amount").
			SchemaType(map[string]string{dialect.MySQL: "decimal(20,2)"}).
			Default(0).
			Comment("满减折扣金额"),
		field.String("coupon_code").
			MaxLen(32).
			Default("").
			Comment("支付优惠券码"),
		field.Float("coupon_discount_amount").
			SchemaType(map[string]string{dialect.MySQL: "decimal(20,2)"}).
			Default(0).
			Comment("优惠券抵扣金额"),
		field.String("recharge_code").
			MaxLen(64),

		// 支付信息
		field.String("out_trade_no").
			MaxLen(64).
			Default(""),
		field.String("payment_type").
			MaxLen(30),
		field.String("payment_trade_no").
			MaxLen(128),
		field.String("pay_url").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "longtext"}),
		field.String("qr_code").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "longtext"}),
		field.String("qr_code_img").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "longtext"}),

		// 订单类型 & 订阅关联
		field.String("order_type").
			MaxLen(20).
			Default("balance"),
		field.Int64("plan_id").
			Optional().
			Nillable(),
		field.Int64("subscription_group_id").
			Optional().
			Nillable(),
		field.Int64("subscription_id").
			Optional().
			Nillable(),
		field.Int("subscription_days").
			Optional().
			Nillable(),
		field.Float("subscription_quota_usd").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "decimal(20,8)"}).
			Comment("Subscription quota snapshot frozen at order creation"),
		field.String("subscription_validity_unit").
			Optional().
			Nillable().
			MaxLen(10).
			Comment("Original subscription plan validity unit at order creation"),
		field.Time("subscription_plan_expires_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "datetime(6)"}).
			Comment("Fixed subscription plan end time frozen at order creation"),
		field.String("provider_instance_id").
			Optional().
			Nillable().
			MaxLen(64),
		field.String("provider_key").
			Optional().
			Nillable().
			MaxLen(30),
		field.JSON("provider_snapshot", map[string]any{}).
			Optional().
			SchemaType(map[string]string{dialect.MySQL: "json"}),

		// 状态
		field.String("status").
			MaxLen(30).
			Default("PENDING"),
		field.String("invoice_status").
			MaxLen(20).
			Default("UNAPPLIED"),
		field.Int64("invoice_application_id").
			Optional().
			Nillable(),

		// 退款信息
		field.Float("refund_amount").
			SchemaType(map[string]string{dialect.MySQL: "decimal(20,2)"}).
			Default(0),
		field.String("refund_reason").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "longtext"}),
		field.Time("refund_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "datetime(6)"}),
		field.Bool("force_refund").
			Default(false),
		field.Time("refund_requested_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "datetime(6)"}),
		field.String("refund_request_reason").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "longtext"}),
		field.String("refund_requested_by").
			Optional().
			Nillable().
			MaxLen(20),

		// 时间节点
		field.Time("expires_at").
			SchemaType(map[string]string{dialect.MySQL: "datetime(6)"}),
		field.Time("paid_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "datetime(6)"}),
		field.Time("completed_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "datetime(6)"}),
		field.Time("failed_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "datetime(6)"}),
		field.String("failed_reason").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "longtext"}),

		// 来源信息
		field.String("client_ip").
			MaxLen(50),
		field.String("src_host").
			MaxLen(255),
		field.String("src_url").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "longtext"}),

		// 时间戳
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.MySQL: "datetime(6)"}),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			SchemaType(map[string]string{dialect.MySQL: "datetime(6)"}),
	}
}

func (PaymentOrder) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("payment_orders").
			Field("user_id").
			Unique().
			Required(),
		edge.To("coupon_usages", CouponUsage.Type),
		edge.To("invoice_application_orders", InvoiceApplicationOrder.Type),
	}
}

func (PaymentOrder) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("out_trade_no").
			Unique().
			Annotations(entsql.IndexWhere("out_trade_no <> ''")),
		index.Fields("user_id"),
		index.Fields("status"),
		index.Fields("expires_at"),
		index.Fields("created_at"),
		index.Fields("paid_at"),
		index.Fields("payment_type", "paid_at"),
		index.Fields("order_type"),
		index.Fields("subscription_id"),
		index.Fields("coupon_code"),
		index.Fields("invoice_status"),
		index.Fields("invoice_application_id"),
	}
}
