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

// InvoiceHeader stores a reusable ordinary-invoice header owned by a user.
type InvoiceHeader struct {
	ent.Schema
}

func (InvoiceHeader) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "invoice_headers"}}
}

func (InvoiceHeader) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("title_type").MaxLen(20).Default("personal"),
		field.String("title").MaxLen(255),
		field.String("tax_number").MaxLen(64).Default(""),
		field.String("email").MaxLen(255).Default(""),
		field.String("phone").MaxLen(32).Default(""),
		field.String("address").Optional().Nillable().SchemaType(map[string]string{dialect.MySQL: "longtext"}),
		field.Bool("is_default").Default(false),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.MySQL: "datetime(6)"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.MySQL: "datetime(6)"}),
	}
}

func (InvoiceHeader) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("invoice_headers").Field("user_id").Unique().Required(),
	}
}

func (InvoiceHeader) Indexes() []ent.Index {
	return []ent.Index{index.Fields("user_id")}
}
