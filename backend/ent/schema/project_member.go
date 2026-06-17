package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ProjectMember maps users to a project-scoped role.
type ProjectMember struct {
	ent.Schema
}

func (ProjectMember) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "project_members"},
	}
}

func (ProjectMember) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (ProjectMember) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("project_id"),
		field.Int64("user_id"),
		field.String("role").
			MaxLen(20).
			Default("user"),
		field.JSON("scopes", []string{}).
			Default([]string{}),
		field.Bool("is_owner").
			Default(false),
	}
}

func (ProjectMember) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("project", Project.Type).
			Ref("members").
			Field("project_id").
			Unique().
			Required(),
		edge.From("user", User.Type).
			Ref("project_members").
			Field("user_id").
			Unique().
			Required(),
	}
}

func (ProjectMember) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("project_id", "user_id").Unique(),
		index.Fields("user_id"),
		index.Fields("project_id", "role"),
	}
}
