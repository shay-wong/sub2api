package schema

import (
	"fmt"
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

const (
	projectResourceTypeUser         = "user"
	projectResourceTypeGroup        = "group"
	projectResourceTypeAccount      = "account"
	projectResourceTypeProxy        = "proxy"
	projectResourceTypeSubscription = "subscription"
	projectResourceTypeAPIKey       = "api_key"
)

func errInvalidEnumValue(name string) error {
	return fmt.Errorf("invalid %s", name)
}

// ProjectProfileBinding maps canonical resources into a project profile.
type ProjectProfileBinding struct {
	ent.Schema
}

func (ProjectProfileBinding) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "project_profile_bindings"},
	}
}

func (ProjectProfileBinding) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("project_profile_id"),
		field.String("resource_type").
			MaxLen(30).
			Validate(func(v string) error {
				switch v {
				case projectResourceTypeUser,
					projectResourceTypeGroup,
					projectResourceTypeAccount,
					projectResourceTypeProxy,
					projectResourceTypeSubscription,
					projectResourceTypeAPIKey:
					return nil
				default:
					return errInvalidEnumValue("project profile binding resource type")
				}
			}),
		field.Int64("resource_id"),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.JSON("metadata", map[string]any{}).
			Default(func() map[string]any { return map[string]any{} }).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
	}
}

func (ProjectProfileBinding) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("profile", ProjectProfile.Type).
			Ref("bindings").
			Field("project_profile_id").
			Unique().
			Required(),
	}
}

func (ProjectProfileBinding) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("project_profile_id", "resource_type", "resource_id").Unique(),
		index.Fields("resource_type", "resource_id"),
		index.Fields("project_profile_id", "resource_type"),
	}
}
