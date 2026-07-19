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

// InvoiceApplicationOrder snapshots one full payment order selected for an invoice.
type InvoiceApplicationOrder struct {
	ent.Schema
}

func (InvoiceApplicationOrder) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "invoice_application_orders"}}
}

func (InvoiceApplicationOrder) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("application_id"),
		field.Int64("order_id"),
		field.String("order_no").MaxLen(128).Default(""),
		field.String("order_type").MaxLen(20).Default(""),
		field.Float("amount").SchemaType(map[string]string{dialect.MySQL: "decimal(20,2)"}),
		field.Time("paid_at").Optional().Nillable().SchemaType(map[string]string{dialect.MySQL: "datetime(6)"}),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.MySQL: "datetime(6)"}),
	}
}

func (InvoiceApplicationOrder) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("application", InvoiceApplication.Type).Ref("orders").Field("application_id").Unique().Required(),
		edge.From("payment_order", PaymentOrder.Type).Ref("invoice_application_orders").Field("order_id").Unique().Required(),
	}
}

func (InvoiceApplicationOrder) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("application_id", "order_id").Unique(),
		index.Fields("order_id"),
	}
}
