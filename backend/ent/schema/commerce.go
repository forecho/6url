package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	entschema "entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Asset struct{ ent.Schema }

func (Asset) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (Asset) Fields() []ent.Field {
	return []ent.Field{
		field.String("provider").MaxLen(32).Default("imgant"),
		field.String("album").MaxLen(100).Default("6url"),
		field.String("external_id").MaxLen(255),
		field.String("storage_key").MaxLen(500),
		field.String("public_url").MaxLen(2048),
		field.String("content_hash").MaxLen(128).Unique(),
		field.String("mime_type").MaxLen(100),
		field.Int64("size_bytes").NonNegative(),
		field.Int("width").NonNegative(),
		field.Int("height").NonNegative(),
		field.Enum("status").Values("healthy", "unhealthy", "pending_delete", "deleted").Default("healthy"),
		field.Int("created_by_token_id").Optional().Nillable(),
	}
}
func (Asset) Indexes() []ent.Index {
	return []ent.Index{index.Fields("provider", "external_id").Unique()}
}
func (Asset) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("site_logos", Site.Type),
		edge.To("ad_images", Ad.Type),
	}
}

type Placement struct{ ent.Schema }

func (Placement) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (Placement) Fields() []ent.Field {
	return []ent.Field{
		field.Int("site_id").Positive(),
		field.Enum("kind").Values("editorial", "exchange", "paid", "affiliate", "owned"),
		field.Enum("origin").Values("submission", "admin", "migration"),
		field.Enum("status").Values("pending", "active", "suspended", "expired", "canceled").Default("pending"),
		field.Int("submission_id").Optional().Nillable(),
		field.Int("payment_order_id").Optional().Nillable(),
		field.String("outbound_url").MaxLen(2048).Optional().Nillable(),
		field.String("outbound_rel").MaxLen(100).Optional().Default("noopener"),
		field.String("disclosure_key").MaxLen(100).Optional().Default(""),
		field.Time("starts_at").Default(time.Now),
		field.Time("ends_at").Optional().Nillable(),
		field.String("backlink_url").MaxLen(2048).Optional().Nillable(),
		field.Int("consecutive_failures").Default(0),
		field.String("last_check_status").MaxLen(100).Optional().Nillable(),
		field.Time("last_checked_at").Optional().Nillable(),
		field.String("suspended_reason").MaxLen(500).Optional().Default(""),
	}
}
func (Placement) Indexes() []ent.Index {
	return []ent.Index{index.Fields("site_id", "status"), index.Fields("kind", "status")}
}
func (Placement) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("site", Site.Type).Ref("placements").Field("site_id").Unique().Required(),
		edge.From("submission", Submission.Type).Ref("placements").Field("submission_id").Unique(),
		edge.From("payment_order", PaymentOrder.Type).Ref("placements").Field("payment_order_id").Unique(),
		edge.To("locales", PlacementLocale.Type),
		edge.To("backlink_checks", BacklinkCheck.Type),
	}
}
func (Placement) Annotations() []entschema.Annotation {
	return []entschema.Annotation{entsql.Checks(map[string]string{
		"placement_origin_subject": "((origin = 'submission' AND submission_id IS NOT NULL) OR (origin IN ('admin', 'migration') AND submission_id IS NULL))",
		"placement_payment_kind":   "((kind = 'paid' AND payment_order_id IS NOT NULL) OR (kind IN ('editorial', 'exchange', 'affiliate', 'owned') AND payment_order_id IS NULL))",
	})}
}

type PlacementLocale struct{ ent.Schema }

func (PlacementLocale) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (PlacementLocale) Fields() []ent.Field {
	return []ent.Field{field.Int("placement_id").Positive(), field.String("locale").MaxLen(16)}
}
func (PlacementLocale) Indexes() []ent.Index {
	return []ent.Index{index.Fields("placement_id", "locale").Unique()}
}
func (PlacementLocale) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("placement", Placement.Type).Ref("locales").Field("placement_id").Unique().Required(),
	}
}

type Ad struct{ ent.Schema }

func (Ad) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (Ad) Fields() []ent.Field {
	return []ent.Field{
		field.Int("site_id").Optional().Nillable(),
		field.Int("ad_application_id").Optional().Nillable().Unique(),
		field.Int("payment_order_id").Optional().Nillable().Unique(),
		field.Enum("status").Values("draft", "active", "paused", "expired", "canceled").Default("draft"),
		field.String("target_url").MaxLen(2048),
		field.Int("image_asset_id").Positive(),
		field.Time("starts_at").Optional().Nillable(),
		field.Time("ends_at").Optional().Nillable(),
		field.Int("sort_order").Default(0),
		field.Enum("origin").Values("admin", "advertiser", "migration"),
		field.Int("created_by_token_id").Optional().Nillable(),
		field.Int("version").Default(1),
	}
}
func (Ad) Indexes() []ent.Index {
	return []ent.Index{index.Fields("status", "sort_order")}
}
func (Ad) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("site", Site.Type).Ref("ads").Field("site_id").Unique(),
		edge.From("application", AdApplication.Type).Ref("ad").Field("ad_application_id").Unique(),
		edge.From("payment_order", PaymentOrder.Type).Ref("ad").Field("payment_order_id").Unique(),
		edge.From("image_asset", Asset.Type).Ref("ad_images").Field("image_asset_id").Unique().Required(),
		edge.To("localizations", AdLocalization.Type),
	}
}
func (Ad) Annotations() []entschema.Annotation {
	return []entschema.Annotation{entsql.Checks(map[string]string{
		"ad_origin_application_order": "((origin IN ('admin', 'migration') AND ad_application_id IS NULL AND payment_order_id IS NULL) OR (origin = 'advertiser' AND ad_application_id IS NOT NULL AND payment_order_id IS NOT NULL))",
	})}
}

type AdLocalization struct{ ent.Schema }

func (AdLocalization) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (AdLocalization) Fields() []ent.Field {
	return []ent.Field{
		field.Int("ad_id").Positive(),
		field.String("locale").MaxLen(16),
		field.String("title").MaxLen(180),
		field.String("description").MaxLen(500),
		field.String("cta_label").MaxLen(80),
		field.String("alt_text").MaxLen(300),
		field.Int("version").Default(1),
	}
}
func (AdLocalization) Indexes() []ent.Index {
	return []ent.Index{index.Fields("ad_id", "locale").Unique()}
}
func (AdLocalization) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("ad", Ad.Type).Ref("localizations").Field("ad_id").Unique().Required(),
	}
}

type Submission struct{ ent.Schema }

func (Submission) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (Submission) Fields() []ent.Field {
	return []ent.Field{
		field.String("public_token_hash").MaxLen(128).Unique().Sensitive(),
		field.Enum("type").Values("exchange", "paid"),
		field.Enum("status").Values("received", "needs_changes", "rejected", "preapproved", "exchange_verification", "awaiting_payment", "paid", "published", "canceled").Default("received"),
		field.String("applicant_email").MaxLen(320),
		field.String("applicant_name").MaxLen(180),
		field.String("preferred_locale").MaxLen(16),
		field.String("raw_name").MaxLen(180),
		field.String("raw_url").MaxLen(2048),
		field.String("normalized_url").MaxLen(2048),
		field.String("normalized_registrable_domain").MaxLen(255),
		field.Text("raw_description"),
		field.Int("requested_category_id").Optional().Nillable(),
		field.String("backlink_url").MaxLen(2048).Optional().Nillable(),
		field.Int("duplicate_site_id").Optional().Nillable(),
		field.Int("reviewer_token_id").Optional().Nillable(),
		field.Text("review_note").Optional().Default(""),
		field.Time("submitted_at").Default(time.Now),
		field.Time("reviewed_at").Optional().Nillable(),
	}
}
func (Submission) Indexes() []ent.Index {
	return []ent.Index{index.Fields("normalized_registrable_domain"), index.Fields("status", "submitted_at")}
}
func (Submission) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("application_sessions", ApplicationSession.Type),
		edge.To("payment_orders", PaymentOrder.Type),
		edge.To("placements", Placement.Type),
	}
}

type AdApplication struct{ ent.Schema }

func (AdApplication) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (AdApplication) Fields() []ent.Field {
	return []ent.Field{
		field.String("public_token_hash").MaxLen(128).Unique().Sensitive(),
		field.Enum("status").Values("received", "needs_changes", "preapproved", "awaiting_payment", "paid", "published", "rejected", "canceled").Default("received"),
		field.String("applicant_email").MaxLen(320),
		field.String("applicant_name").MaxLen(180),
		field.String("preferred_locale").MaxLen(16),
		field.String("target_url").MaxLen(2048),
		field.JSON("requested_locales", []string{}).Default([]string{}),
		field.String("raw_title").MaxLen(180),
		field.Text("raw_description"),
		field.String("source_image_url").MaxLen(2048).Optional().Nillable(),
		field.Int("image_asset_id").Optional().Nillable(),
		field.Int("reviewer_token_id").Optional().Nillable(),
		field.Text("review_note").Optional().Default(""),
	}
}
func (AdApplication) Indexes() []ent.Index {
	return []ent.Index{index.Fields("status", "created_at")}
}
func (AdApplication) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("application_sessions", ApplicationSession.Type),
		edge.To("payment_orders", PaymentOrder.Type),
		edge.To("ad", Ad.Type).Unique(),
	}
}

type ApplicationSession struct{ ent.Schema }

func (ApplicationSession) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (ApplicationSession) Fields() []ent.Field {
	return []ent.Field{
		field.Enum("subject_type").Values("submission", "ad_application"),
		field.Int("submission_id").Optional().Nillable(),
		field.Int("ad_application_id").Optional().Nillable(),
		field.String("session_hash").MaxLen(128).Unique().Sensitive(),
		field.String("csrf_secret_hash").MaxLen(128).Sensitive(),
		field.Time("expires_at"),
		field.Time("last_seen_at").Default(time.Now),
		field.Time("revoked_at").Optional().Nillable(),
	}
}
func (ApplicationSession) Annotations() []entschema.Annotation {
	return []entschema.Annotation{entsql.Checks(map[string]string{
		"application_session_subject": "((subject_type = 'submission' AND submission_id IS NOT NULL AND ad_application_id IS NULL) OR (subject_type = 'ad_application' AND submission_id IS NULL AND ad_application_id IS NOT NULL))",
	})}
}
func (ApplicationSession) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("submission", Submission.Type).Ref("application_sessions").Field("submission_id").Unique(),
		edge.From("ad_application", AdApplication.Type).Ref("application_sessions").Field("ad_application_id").Unique(),
	}
}

type ListingPlan struct{ ent.Schema }

func (ListingPlan) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (ListingPlan) Fields() []ent.Field {
	return []ent.Field{
		field.String("code").MaxLen(100).Unique(), field.String("name").MaxLen(180),
		field.Int64("amount_minor").NonNegative(), field.String("currency").MaxLen(3).Default("usd"),
		field.Int("duration_days").Optional().Nillable(), field.JSON("included_locales", []string{}).Default([]string{}),
		field.String("stripe_price_id").MaxLen(255).Optional().Nillable().Unique(), field.Bool("active").Default(false), field.Int("sort_order").Default(0),
	}
}
func (ListingPlan) Edges() []ent.Edge {
	return []ent.Edge{edge.To("payment_orders", PaymentOrder.Type)}
}

type AdOffer struct{ ent.Schema }

func (AdOffer) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (AdOffer) Fields() []ent.Field {
	return []ent.Field{
		field.String("code").MaxLen(100).Unique(), field.String("name").MaxLen(180),
		field.Int64("amount_minor").NonNegative(), field.String("currency").MaxLen(3).Default("usd"),
		field.Int("duration_days").Positive(), field.JSON("included_locales", []string{}).Default([]string{}),
		field.String("stripe_price_id").MaxLen(255).Optional().Nillable().Unique(), field.Bool("active").Default(false),
	}
}
func (AdOffer) Edges() []ent.Edge {
	return []ent.Edge{edge.To("payment_orders", PaymentOrder.Type)}
}

type PaymentOrder struct{ ent.Schema }

func (PaymentOrder) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (PaymentOrder) Fields() []ent.Field {
	return []ent.Field{
		field.Enum("purpose").Values("listing", "ad"),
		field.Int("submission_id").Optional().Nillable(), field.Int("ad_application_id").Optional().Nillable(), field.Int("site_id").Optional().Nillable(),
		field.Int("listing_plan_id").Optional().Nillable(), field.Int("ad_offer_id").Optional().Nillable(),
		field.Enum("status").Values("pending", "processing", "paid", "failed", "expired", "refunded", "pending_manual").Default("pending"),
		field.Enum("provider").Values("stripe", "manual"), field.Int64("amount_minor").NonNegative(), field.String("currency").MaxLen(3).Default("usd"),
		field.Bool("livemode").Default(false), field.Int("duration_days_snapshot").Optional().Nillable(), field.JSON("locales_snapshot", []string{}).Default([]string{}),
		field.String("stripe_price_id_snapshot").MaxLen(255).Optional().Nillable(),
		field.String("stripe_checkout_session_id").MaxLen(255).Optional().Nillable().Unique(),
		field.String("stripe_payment_intent_id").MaxLen(255).Optional().Nillable().Unique(),
		field.String("external_ref").MaxLen(255).Optional().Nillable().Unique(),
		field.Time("paid_at").Optional().Nillable(), field.Time("refunded_at").Optional().Nillable(),
	}
}
func (PaymentOrder) Indexes() []ent.Index {
	return []ent.Index{index.Fields("purpose", "status"), index.Fields("submission_id"), index.Fields("ad_application_id")}
}
func (PaymentOrder) Annotations() []entschema.Annotation {
	return []entschema.Annotation{entsql.Checks(map[string]string{
		"payment_order_purpose_subject": "((purpose = 'listing' AND submission_id IS NOT NULL AND ad_application_id IS NULL AND listing_plan_id IS NOT NULL AND ad_offer_id IS NULL) OR (purpose = 'ad' AND submission_id IS NULL AND ad_application_id IS NOT NULL AND listing_plan_id IS NULL AND ad_offer_id IS NOT NULL))",
	})}
}
func (PaymentOrder) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("submission", Submission.Type).Ref("payment_orders").Field("submission_id").Unique(),
		edge.From("ad_application", AdApplication.Type).Ref("payment_orders").Field("ad_application_id").Unique(),
		edge.From("site", Site.Type).Ref("payment_orders").Field("site_id").Unique(),
		edge.From("listing_plan", ListingPlan.Type).Ref("payment_orders").Field("listing_plan_id").Unique(),
		edge.From("ad_offer", AdOffer.Type).Ref("payment_orders").Field("ad_offer_id").Unique(),
		edge.To("placements", Placement.Type),
		edge.To("ad", Ad.Type).Unique(),
	}
}

type PaymentEvent struct{ ent.Schema }

func (PaymentEvent) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (PaymentEvent) Fields() []ent.Field {
	return []ent.Field{
		field.String("provider").MaxLen(32), field.String("event_id").MaxLen(255).Unique(), field.String("type").MaxLen(255),
		field.Text("payload_encrypted_or_redacted"), field.Time("received_at").Default(time.Now),
		field.Time("processing_started_at").Optional().Nillable(), field.Time("processed_at").Optional().Nillable(), field.String("error_summary").MaxLen(500).Optional().Default(""),
	}
}

type BacklinkCheck struct{ ent.Schema }

func (BacklinkCheck) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (BacklinkCheck) Fields() []ent.Field {
	return []ent.Field{
		field.Int("placement_id").Positive(), field.Enum("status").Values("passed", "missing", "nofollow", "blocked", "network_error", "unsafe", "manual_review"),
		field.String("requested_url").MaxLen(2048), field.String("final_url").MaxLen(2048).Optional().Default(""), field.Int("http_status").Optional().Nillable(),
		field.String("matched_href").MaxLen(2048).Optional().Default(""), field.String("matched_anchor").MaxLen(500).Optional().Default(""), field.String("matched_rel").MaxLen(200).Optional().Default(""),
		field.Text("redirect_chain_json").Optional().Default("[]"), field.String("error_code").MaxLen(100).Optional().Default(""), field.Time("checked_at").Default(time.Now),
	}
}
func (BacklinkCheck) Indexes() []ent.Index {
	return []ent.Index{index.Fields("placement_id", "checked_at")}
}
func (BacklinkCheck) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("placement", Placement.Type).Ref("backlink_checks").Field("placement_id").Unique().Required(),
	}
}
