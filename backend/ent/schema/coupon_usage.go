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

// CouponUsage records payment coupon usage per order.
type CouponUsage struct {
	ent.Schema
}

func (CouponUsage) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "coupon_usages"},
	}
}

func (CouponUsage) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("coupon_id").
			Comment("优惠券ID"),
		field.Int64("user_id").
			Comment("使用用户ID"),
		field.Int64("order_id").
			Comment("支付订单ID"),
		field.Float("discount_amount").
			SchemaType(map[string]string{dialect.MySQL: "decimal(20,2)"}).
			Comment("实际抵扣金额"),
		field.Time("used_at").
			Default(time.Now).
			SchemaType(map[string]string{dialect.MySQL: "datetime(6)"}).
			Comment("使用时间"),
		field.String("status").
			MaxLen(20).
			Default("used").
			Comment("状态: used, refunded"),
	}
}

func (CouponUsage) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("coupon", Coupon.Type).
			Ref("usage_records").
			Field("coupon_id").
			Required().
			Unique(),
		edge.From("user", User.Type).
			Ref("coupon_usages").
			Field("user_id").
			Required().
			Unique(),
		edge.From("payment_order", PaymentOrder.Type).
			Ref("coupon_usages").
			Field("order_id").
			Required().
			Unique(),
	}
}

func (CouponUsage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("coupon_id"),
		index.Fields("user_id"),
		index.Fields("order_id").Unique(),
		index.Fields("coupon_id", "user_id"),
		index.Fields("status"),
	}
}
