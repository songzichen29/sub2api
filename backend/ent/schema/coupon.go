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

// Coupon holds payment coupon definitions.
//
// 支付优惠券：用户创建支付订单时使用，可抵扣订单金额。
// 删除策略：不硬删除已使用优惠券，管理端删除应转为 disabled/archived。
type Coupon struct {
	ent.Schema
}

func (Coupon) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "coupons"},
	}
}

func (Coupon) Fields() []ent.Field {
	return []ent.Field{
		field.String("code").
			MaxLen(32).
			NotEmpty().
			Unique().
			Comment("优惠券码"),
		field.String("type").
			MaxLen(20).
			Comment("类型: fixed, percent"),
		field.Float("value").
			SchemaType(map[string]string{dialect.MySQL: "decimal(20,8)"}).
			Comment("固定金额或减免比例"),
		field.Float("min_amount").
			SchemaType(map[string]string{dialect.MySQL: "decimal(20,2)"}).
			Default(0).
			Comment("最低消费门槛"),
		field.Float("max_discount").
			SchemaType(map[string]string{dialect.MySQL: "decimal(20,2)"}).
			Default(0).
			Comment("百分比类型最大减免金额，0表示不限制"),
		field.String("scope").
			MaxLen(20).
			Default("all").
			Comment("适用范围: balance, subscription, all"),
		field.Int("max_uses").
			Default(0).
			Comment("总使用次数限制，0表示无限制"),
		field.Int("used_count").
			Default(0).
			Comment("已使用次数"),
		field.Int("per_user_limit").
			Default(1).
			Comment("每用户限用次数，0表示无限制"),
		field.String("status").
			MaxLen(20).
			Default("active").
			Comment("状态: active, disabled, archived"),
		field.Time("starts_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "datetime(6)"}).
			Comment("生效时间，null表示立即生效"),
		field.Time("expires_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "datetime(6)"}).
			Comment("过期时间，null表示永不过期"),
		field.String("notes").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "longtext"}).
			Comment("备注"),
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

func (Coupon) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("usage_records", CouponUsage.Type),
	}
}

func (Coupon) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("scope"),
		index.Fields("expires_at"),
	}
}
