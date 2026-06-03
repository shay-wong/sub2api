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

// UserGroupRateLimitWindow stores per-user, per-group 5-hour USD usage windows.
type UserGroupRateLimitWindow struct {
	ent.Schema
}

func (UserGroupRateLimitWindow) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "user_group_rate_limit_windows"},
	}
}

func (UserGroupRateLimitWindow) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

func (UserGroupRateLimitWindow) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.Int64("group_id"),
		field.Float("usage_5h_usd").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).
			Default(0).
			Comment("Used USD amount in the current 5-hour window"),
		field.Time("window_5h_start").
			Optional().
			Nillable().
			Comment("Start time of the current 5-hour window"),
	}
}

func (UserGroupRateLimitWindow) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("group_rate_limit_windows").
			Field("user_id").
			Unique().
			Required(),
		edge.From("group", Group.Type).
			Ref("user_rate_limit_windows").
			Field("group_id").
			Unique().
			Required(),
	}
}

func (UserGroupRateLimitWindow) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "group_id").
			Unique().
			Annotations(entsql.IndexWhere("deleted_at IS NULL")),
		index.Fields("user_id"),
		index.Fields("group_id"),
		index.Fields("deleted_at"),
	}
}
