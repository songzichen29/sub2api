package schema

import (
	"time"

	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/domain"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// UserSubscription holds the schema definition for the UserSubscription entity.
type UserSubscription struct {
	ent.Schema
}

func (UserSubscription) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "user_subscriptions"},
	}
}

func (UserSubscription) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

func (UserSubscription) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.Int64("group_id"),

		field.Time("starts_at").
			SchemaType(map[string]string{dialect.MySQL: "datetime(6)"}),
		field.Time("expires_at").
			SchemaType(map[string]string{dialect.MySQL: "datetime(6)"}),
		field.String("status").
			MaxLen(20).
			Default(domain.SubscriptionStatusActive),
		field.String("validity_unit").
			MaxLen(10).
			Default("day").
			Comment("Original validity unit for overdraft accounting: day/week/month"),

		field.Time("daily_window_start").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "datetime(6)"}),
		field.Time("weekly_window_start").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "datetime(6)"}),
		field.Time("monthly_window_start").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "datetime(6)"}),

		field.Float("daily_usage_usd").
			SchemaType(map[string]string{dialect.MySQL: "decimal(20,10)"}).
			Default(0),
		field.Float("weekly_usage_usd").
			SchemaType(map[string]string{dialect.MySQL: "decimal(20,10)"}).
			Default(0),
		field.Float("monthly_usage_usd").
			SchemaType(map[string]string{dialect.MySQL: "decimal(20,10)"}).
			Default(0),
		field.Float("quota_limit_usd").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "decimal(20,8)"}).
			Comment("Total USD quota available during this subscription period"),
		field.Float("quota_used_usd").
			SchemaType(map[string]string{dialect.MySQL: "decimal(20,10)"}).
			Default(0).
			Comment("Total USD quota used during this subscription period"),
		field.Bool("allow_daily_overdraft").
			Default(false).
			Comment("Whether this user subscription has enabled daily quota overdraft"),
		field.Bool("skip_weekends").
			Default(false).
			Comment("Whether this subscription is unavailable on weekends and expiry is compensated"),
		field.Time("weekend_skip_user_changed_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "datetime(6)"}).
			Comment("When the user consumed the one-time weekend skip change opportunity"),
		field.Time("weekend_skip_original_expires_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "datetime(6)"}).
			Comment("Original expires_at before weekend skip first compensated the subscription"),
		field.Time("weekend_skip_admin_updated_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "datetime(6)"}).
			Comment("When an administrator last changed weekend skip state"),
		field.Int64("weekend_skip_admin_updated_by").
			Optional().
			Nillable().
			Comment("Administrator ID that last changed weekend skip state"),

		field.Int64("assigned_by").
			Optional().
			Nillable(),
		field.Time("assigned_at").
			Default(time.Now).
			SchemaType(map[string]string{dialect.MySQL: "datetime(6)"}),
		field.String("notes").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.MySQL: "longtext"}),
		// 订阅来源标记，决定是否允许管理员重置配额：
		// admin   = 管理员手动分配（按窗口层级规则可部分重置）
		// redeem  = 用户用兑换码兑换（同上）
		// payment = 用户付费购买（永久禁止重置）
		field.String("source").
			MaxLen(20).
			Default(domain.SubscriptionSourceAdmin),
	}
}

func (UserSubscription) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("subscriptions").
			Field("user_id").
			Unique().
			Required(),
		edge.From("group", Group.Type).
			Ref("subscriptions").
			Field("group_id").
			Unique().
			Required(),
		edge.From("assigned_by_user", User.Type).
			Ref("assigned_subscriptions").
			Field("assigned_by").
			Unique(),
		edge.To("usage_logs", UsageLog.Type),
	}
}

func (UserSubscription) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("group_id"),
		index.Fields("status"),
		index.Fields("expires_at"),
		// 活跃订阅查询复合索引（线上由 SQL 迁移创建部分索引，schema 仅用于模型可读性对齐）
		index.Fields("user_id", "status", "expires_at"),
		index.Fields("assigned_by"),
		// 唯一约束通过部分索引实现（WHERE deleted_at IS NULL），支持软删除后重新订阅
		// 见迁移文件 016_soft_delete_partial_unique_indexes.sql
		index.Fields("user_id", "group_id"),
		index.Fields("deleted_at"),
	}
}
