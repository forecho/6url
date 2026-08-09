package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type AdminAPIToken struct{ ent.Schema }

func (AdminAPIToken) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (AdminAPIToken) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").MaxLen(180),
		field.String("prefix").MaxLen(24).Unique(),
		field.String("token_hash").MaxLen(128).Unique().Sensitive(),
		field.JSON("scopes", []string{}).Default([]string{}),
		field.Time("expires_at").Optional().Nillable(),
		field.Time("last_used_at").Optional().Nillable(),
		field.Time("revoked_at").Optional().Nillable(),
	}
}

type AdminSession struct{ ent.Schema }

func (AdminSession) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (AdminSession) Fields() []ent.Field {
	return []ent.Field{
		field.Int("admin_api_token_id").Positive(), field.String("session_hash").MaxLen(128).Unique().Sensitive(),
		field.String("csrf_secret_hash").MaxLen(128).Sensitive(), field.Time("expires_at"), field.Time("last_seen_at").Default(time.Now),
		field.Time("revoked_at").Optional().Nillable(),
	}
}

type AdminAuditLog struct{ ent.Schema }

func (AdminAuditLog) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (AdminAuditLog) Fields() []ent.Field {
	return []ent.Field{
		field.Int("admin_api_token_id").Optional().Nillable(), field.String("action").MaxLen(180), field.String("resource_type").MaxLen(100),
		field.String("resource_id").MaxLen(100).Optional().Default(""), field.String("request_id").MaxLen(100).Optional().Default(""),
		field.String("idempotency_key").MaxLen(255).Optional().Nillable(), field.Text("diff_summary_json").Optional().Default("{}"),
		field.String("reason").MaxLen(500).Optional().Default(""), field.String("ip_hash").MaxLen(128).Optional().Default(""),
	}
}
func (AdminAuditLog) Indexes() []ent.Index {
	return []ent.Index{index.Fields("resource_type", "resource_id"), index.Fields("created_at")}
}

type RevalidationJob struct{ ent.Schema }

func (RevalidationJob) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (RevalidationJob) Fields() []ent.Field {
	return []ent.Field{
		field.String("dedupe_key").MaxLen(255).Unique(), field.Enum("status").Values("pending", "processing", "completed", "failed").Default("pending"),
		field.JSON("paths", []string{}).Default([]string{}), field.Int("attempts").Default(0), field.Int("max_attempts").Default(5),
		field.Time("next_retry_at").Default(time.Now), field.Time("claimed_at").Optional().Nillable(), field.Time("completed_at").Optional().Nillable(),
		field.String("last_error").MaxLen(500).Optional().Default(""),
	}
}
func (RevalidationJob) Indexes() []ent.Index {
	return []ent.Index{index.Fields("status", "next_retry_at")}
}

type NotificationDelivery struct{ ent.Schema }

func (NotificationDelivery) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (NotificationDelivery) Fields() []ent.Field {
	return []ent.Field{
		field.String("dedupe_key").MaxLen(255).Unique(), field.Enum("channel").Values("email", "slack"), field.String("kind").MaxLen(100),
		field.String("recipient").MaxLen(500).Sensitive(), field.String("locale").MaxLen(16).Optional().Default("zh-CN"),
		field.String("template_key").MaxLen(100), field.Int("template_version").Default(1), field.Text("payload_encrypted").Sensitive(),
		field.Enum("status").Values("pending", "sending", "sent", "failed").Default("pending"), field.Int("attempts").Default(0), field.Int("max_attempts").Default(5),
		field.Time("next_retry_at").Default(time.Now), field.Time("claimed_at").Optional().Nillable(), field.Time("sent_at").Optional().Nillable(),
		field.Time("payload_expires_at").Optional().Nillable(), field.String("last_error").MaxLen(500).Optional().Default(""),
	}
}
func (NotificationDelivery) Indexes() []ent.Index {
	return []ent.Index{index.Fields("status", "next_retry_at")}
}
