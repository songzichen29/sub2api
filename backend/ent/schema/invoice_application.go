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

// InvoiceApplication represents an offline ordinary-invoice request.
type InvoiceApplication struct {
	ent.Schema
}

func (InvoiceApplication) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "invoice_applications"}}
}

func (InvoiceApplication) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("status").MaxLen(20).Default("PENDING"),
		field.String("invoice_type").MaxLen(20).Default("ordinary"),
		field.String("header_type").MaxLen(20),
		field.String("header_title").MaxLen(255),
		field.String("header_tax_number").MaxLen(64).Default(""),
		field.String("header_email").MaxLen(255).Default(""),
		field.String("header_phone").MaxLen(32).Default(""),
		field.String("header_address").Optional().Nillable().SchemaType(map[string]string{dialect.MySQL: "longtext"}),
		field.Float("total_amount").SchemaType(map[string]string{dialect.MySQL: "decimal(20,2)"}),
		field.Int64("handled_by").Optional().Nillable(),
		field.String("rejection_reason").Optional().Nillable().SchemaType(map[string]string{dialect.MySQL: "longtext"}),
		field.String("admin_note").Optional().Nillable().SchemaType(map[string]string{dialect.MySQL: "longtext"}),
		field.String("invoice_number").MaxLen(128).Default(""),
		field.Time("processed_at").Optional().Nillable().SchemaType(map[string]string{dialect.MySQL: "datetime(6)"}),
		field.Time("invoiced_at").Optional().Nillable().SchemaType(map[string]string{dialect.MySQL: "datetime(6)"}),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.MySQL: "datetime(6)"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.MySQL: "datetime(6)"}),
	}
}

func (InvoiceApplication) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("invoice_applications").Field("user_id").Unique().Required(),
		edge.To("orders", InvoiceApplicationOrder.Type),
	}
}

func (InvoiceApplication) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "status"),
		index.Fields("created_at"),
	}
}
