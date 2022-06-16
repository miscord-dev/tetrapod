package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Node holds the schema definition for the Node entity.
type Node struct {
	ent.Schema
}

// Fields of the Node.
func (Node) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").Unique(),
		field.String("public_key").Unique(),
		field.String("public_disco_key").Unique(),
		field.String("host_name"),
		field.String("os"),
		field.String("goos"),
		field.String("goarch"),
		field.Time("last_updated_at"),
		field.Strings("endpoints"),
		field.Enum("state").Values("online", "offline", "disabled"),
	}
}

// Edges of the Node.
func (Node) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("routes", Route.Type),
		edge.To("addresses", Address.Type),
	}
}
