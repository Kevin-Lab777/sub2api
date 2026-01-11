package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// APIKey holds the schema definition for the APIKey entity.
type APIKey struct {
	ent.Schema
}

func (APIKey) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "api_keys"},
	}
}

func (APIKey) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

func (APIKey) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("key").
			MaxLen(128).
			NotEmpty().
			Unique(),
		field.String("name").
			MaxLen(100).
			NotEmpty(),
		field.Int64("group_id").
			Optional().
			Nillable(),
		field.String("status").
			MaxLen(20).
			Default(service.StatusActive),
		field.JSON("ip_whitelist", []string{}).
			Optional().
			Comment("Allowed IPs/CIDRs, e.g. [\"192.168.1.100\", \"10.0.0.0/8\"]"),
		field.JSON("ip_blacklist", []string{}).
			Optional().
			Comment("Blocked IPs/CIDRs"),

		// [LITE] 限额字段
		field.Float("daily_limit_usd").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Comment("日限额 (USD)，nil = 使用分组限额"),
		field.Float("weekly_limit_usd").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Comment("周限额 (USD)，nil = 使用分组限额"),
		field.Float("monthly_limit_usd").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Comment("月限额 (USD)，nil = 使用分组限额"),
		field.Float("total_limit_usd").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Comment("总限额 (USD)，nil = 无限制"),

		// [LITE] 用量追踪字段
		field.Float("daily_usage_usd").
			Default(0).
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Comment("今日已用额度"),
		field.Float("weekly_usage_usd").
			Default(0).
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Comment("本周已用额度"),
		field.Float("monthly_usage_usd").
			Default(0).
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Comment("本月已用额度"),
		field.Float("total_usage_usd").
			Default(0).
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Comment("累计已用额度"),

		// [LITE] 用量重置时间字段
		field.Time("usage_reset_daily").
			Optional().
			Nillable().
			Comment("日用量重置时间"),
		field.Time("usage_reset_weekly").
			Optional().
			Nillable().
			Comment("周用量重置时间"),
		field.Time("usage_reset_monthly").
			Optional().
			Nillable().
			Comment("月用量重置时间"),
	}
}

func (APIKey) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("api_keys").
			Field("user_id").
			Unique().
			Required(),
		edge.From("group", Group.Type).
			Ref("api_keys").
			Field("group_id").
			Unique(),
		edge.To("usage_logs", UsageLog.Type),
	}
}

func (APIKey) Indexes() []ent.Index {
	return []ent.Index{
		// key 字段已在 Fields() 中声明 Unique()，无需重复索引
		index.Fields("user_id"),
		index.Fields("group_id"),
		index.Fields("status"),
		index.Fields("deleted_at"),
	}
}
