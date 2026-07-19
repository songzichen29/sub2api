package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// InvoiceSetting is the singleton invoice configuration record (ID = 1).
type InvoiceSetting struct {
	ent.Schema
}

func (InvoiceSetting) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "invoice_settings"}}
}

func (InvoiceSetting) Fields() []ent.Field {
	return []ent.Field{
		field.Float("min_amount").SchemaType(map[string]string{dialect.MySQL: "decimal(20,2)"}).Default(300),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.MySQL: "datetime(6)"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.MySQL: "datetime(6)"}),
	}
}
