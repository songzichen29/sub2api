package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SubscriptionPlan holds the schema definition for the SubscriptionPlan entity.
//
// 删除策略：硬删除
// SubscriptionPlan 使用硬删除而非软删除，原因如下：
//   - 套餐为管理员维护的商品配置，删除即表示下架移除
//   - 通过 for_sale 字段控制是否在售，删除仅用于彻底移除
//   - 已购买的订阅记录保存在 UserSubscription 中，不受套餐删除影响
type SubscriptionPlan struct {
	ent.Schema
}

func (SubscriptionPlan) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "subscription_plans"},
	}
}

func (SubscriptionPlan) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("group_id"),
		field.String("name").
			MaxLen(100).
			NotEmpty(),
		field.String("description").
			SchemaType(map[string]string{dialect.MySQL: "longtext"}).
			Default(""),
		field.Float("price").
			SchemaType(map[string]string{dialect.MySQL: "decimal(20,2)"}),
		field.Float("original_price").
			SchemaType(map[string]string{dialect.MySQL: "decimal(20,2)"}).
			Optional().
			Nillable(),
		field.String("currency").
			MaxLen(3).
			Default(""),
		field.Int("validity_days").
			Default(30),
		field.String("validity_unit").
			MaxLen(10).
			Default("day"),
		field.Float("quota_limit_usd").
			SchemaType(map[string]string{dialect.MySQL: "decimal(20,8)"}).
			Optional().
			Nillable().
			Comment("Total USD quota granted by this plan during the subscription period"),
		field.Time("expires_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "datetime(6)"}).
			Comment("Fixed subscription end time; when set, purchases expire at this timestamp instead of now + validity_days"),
		field.String("features").
			SchemaType(map[string]string{dialect.MySQL: "longtext"}).
			Default(""),
		field.String("product_name").
			MaxLen(100).
			Default(""),
		field.Int("max_buy_count").
			Optional().
			Nillable().
			Comment("Per-user purchase limit; NULL means unlimited"),
		field.Bool("for_sale").
			Default(true),
		field.Int("sort_order").
			Default(0),
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

func (SubscriptionPlan) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("group_id"),
		index.Fields("for_sale"),
	}
}
