package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Address holds the schema definition for the Address entity.
type Address struct {
	ent.Schema
}

// Fields of the Address.
func (Address) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").Unique(),
		field.String("addr").MaxLen(256),
	}
}

// Edges of the Address.
func (Address) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("host", Node.Type).
			Ref("addresses").
			Unique(),
	}
}
