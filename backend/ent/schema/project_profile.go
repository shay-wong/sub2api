package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

const (
	projectProfileModeRestricted   = "restricted"
	projectProfileModeUnrestricted = "unrestricted"
)

// ProjectProfile is a project-scoped application profile.
type ProjectProfile struct {
	ent.Schema
}

func (ProjectProfile) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "project_profiles"},
	}
}

func (ProjectProfile) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

func (ProjectProfile) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("project_id"),
		field.String("name").
			MaxLen(100).
			NotEmpty(),
		field.String("description").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("mode").
			MaxLen(20).
			Default(projectProfileModeRestricted).
			Validate(func(v string) error {
				if v == projectProfileModeRestricted || v == projectProfileModeUnrestricted {
					return nil
				}
				return errInvalidEnumValue("project profile mode")
			}),
		field.Bool("is_active").
			Default(false),
	}
}

func (ProjectProfile) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("project", Project.Type).
			Ref("app_profiles").
			Field("project_id").
			Unique().
			Required(),
		edge.To("bindings", ProjectProfileBinding.Type),
	}
}

func (ProjectProfile) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("project_id", "is_active"),
		index.Fields("deleted_at"),
		index.Fields("project_id").
			Unique().
			Annotations(entsql.IndexWhere("is_active = true AND deleted_at IS NULL")),
	}
}
