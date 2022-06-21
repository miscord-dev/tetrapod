package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// Route holds the schema definition for the Route entity.
type Route struct {
	ent.Schema
}

// Fields of the Route.
func (Route) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").Unique(),
		field.String("addr").MaxLen(256),
		field.Int("bits"),
	}
}

// Edges of the Route.
func (Route) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("host", Node.Type).
			Ref("routes").
			Unique().
			Required(),
	}
}
