package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Locale struct{ ent.Schema }

func (Locale) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (Locale) Fields() []ent.Field {
	return []ent.Field{
		field.String("code").MaxLen(16).Unique(),
		field.String("name").MaxLen(64),
		field.String("native_name").MaxLen(64),
		field.Bool("enabled").Default(true),
		field.Bool("is_default").Default(false),
		field.Int("sort_order").Default(0),
	}
}

type Category struct{ ent.Schema }

func (Category) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (Category) Fields() []ent.Field {
	return []ent.Field{
		field.String("slug").MaxLen(160).Unique(),
		field.Enum("status").Values("draft", "published", "archived").Default("draft"),
		field.Int("sort_order").Default(0),
		field.Int("version").Default(1),
	}
}
func (Category) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("content_revisions", CategoryContentRevision.Type),
		edge.To("locale_publications", CategoryLocalePublication.Type),
		edge.To("site_links", SiteCategory.Type),
	}
}

type CategoryContentRevision struct{ ent.Schema }

func (CategoryContentRevision) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (CategoryContentRevision) Fields() []ent.Field {
	return []ent.Field{
		field.Int("category_id").Positive(),
		field.String("locale").MaxLen(16),
		field.Int("revision").Positive(),
		field.Enum("status").Values("draft", "review", "approved").Default("draft"),
		field.String("title").MaxLen(180),
		field.Text("description"),
		field.String("seo_title").MaxLen(180),
		field.String("seo_description").MaxLen(320),
		field.String("source_locale").MaxLen(16),
		field.Int("source_revision_id").Optional().Nillable(),
		field.Int("facts_version").Default(1),
		field.Int("created_by_token_id").Optional().Nillable(),
	}
}
func (CategoryContentRevision) Indexes() []ent.Index {
	return []ent.Index{index.Fields("category_id", "locale", "revision").Unique()}
}
func (CategoryContentRevision) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("category", Category.Type).Ref("content_revisions").Field("category_id").Unique().Required(),
		edge.To("draft_for", CategoryLocalePublication.Type).Unique(),
		edge.To("published_for", CategoryLocalePublication.Type).Unique(),
	}
}

type CategoryLocalePublication struct{ ent.Schema }

func (CategoryLocalePublication) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (CategoryLocalePublication) Fields() []ent.Field {
	return []ent.Field{
		field.Int("category_id").Positive(),
		field.String("locale").MaxLen(16),
		field.Int("draft_revision_id").Optional().Nillable(),
		field.Int("published_revision_id").Optional().Nillable(),
		field.Enum("translation_status").Values("missing", "draft", "review", "published", "source_changed").Default("missing"),
		field.Int("version").Default(1),
	}
}
func (CategoryLocalePublication) Indexes() []ent.Index {
	return []ent.Index{index.Fields("category_id", "locale").Unique()}
}
func (CategoryLocalePublication) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("category", Category.Type).Ref("locale_publications").Field("category_id").Unique().Required(),
		edge.From("draft_revision", CategoryContentRevision.Type).Ref("draft_for").Field("draft_revision_id").Unique(),
		edge.From("published_revision", CategoryContentRevision.Type).Ref("published_for").Field("published_revision_id").Unique(),
	}
}

type Site struct{ ent.Schema }

func (Site) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (Site) Fields() []ent.Field {
	return []ent.Field{
		field.String("canonical_url").MaxLen(2048),
		field.String("canonical_url_hash").MaxLen(64).Unique(),
		field.String("registrable_domain").MaxLen(255),
		field.String("slug").MaxLen(180).Unique(),
		field.Enum("status").Values("draft", "published", "suspended", "archived", "removed").Default("draft"),
		field.Enum("created_via").Values("migration", "submission", "admin_manual").Default("admin_manual"),
		field.Int("logo_asset_id").Optional().Nillable(),
		field.Int("facts_version").Default(1),
		field.Int("version").Default(1),
	}
}
func (Site) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("category_links", SiteCategory.Type),
		edge.To("content_revisions", SiteContentRevision.Type),
		edge.To("locale_publications", SiteLocalePublication.Type),
		edge.To("placements", Placement.Type),
		edge.To("payment_orders", PaymentOrder.Type),
		edge.To("ads", Ad.Type),
		edge.From("logo_asset", Asset.Type).Ref("site_logos").Field("logo_asset_id").Unique(),
	}
}

type SiteCategory struct{ ent.Schema }

func (SiteCategory) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (SiteCategory) Fields() []ent.Field {
	return []ent.Field{
		field.Int("site_id").Positive(),
		field.Int("category_id").Positive(),
		field.Bool("is_primary").Default(false),
		field.Int("sort_order").Default(0),
	}
}
func (SiteCategory) Indexes() []ent.Index {
	return []ent.Index{index.Fields("site_id", "category_id").Unique()}
}
func (SiteCategory) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("site", Site.Type).Ref("category_links").Field("site_id").Unique().Required(),
		edge.From("category", Category.Type).Ref("site_links").Field("category_id").Unique().Required(),
	}
}

type SiteContentRevision struct{ ent.Schema }

func (SiteContentRevision) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (SiteContentRevision) Fields() []ent.Field {
	return []ent.Field{
		field.Int("site_id").Positive(),
		field.String("locale").MaxLen(16),
		field.Int("revision").Positive(),
		field.Enum("status").Values("draft", "review", "approved").Default("draft"),
		field.String("name").MaxLen(180),
		field.String("tagline").MaxLen(280),
		field.Text("summary"),
		field.Text("description"),
		field.JSON("features", []string{}).Default([]string{}),
		field.JSON("use_cases", []string{}).Default([]string{}),
		field.JSON("pros", []string{}).Default([]string{}),
		field.JSON("limitations", []string{}).Default([]string{}),
		field.String("pricing_summary").MaxLen(500).Optional().Default(""),
		field.String("seo_title").MaxLen(180),
		field.String("seo_description").MaxLen(320),
		field.String("source_locale").MaxLen(16),
		field.Int("source_revision_id").Optional().Nillable(),
		field.Int("facts_version").Default(1),
		field.Int("created_by_token_id").Optional().Nillable(),
	}
}
func (SiteContentRevision) Indexes() []ent.Index {
	return []ent.Index{index.Fields("site_id", "locale", "revision").Unique()}
}
func (SiteContentRevision) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("site", Site.Type).Ref("content_revisions").Field("site_id").Unique().Required(),
		edge.To("draft_for", SiteLocalePublication.Type).Unique(),
		edge.To("published_for", SiteLocalePublication.Type).Unique(),
	}
}

type SiteLocalePublication struct{ ent.Schema }

func (SiteLocalePublication) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (SiteLocalePublication) Fields() []ent.Field {
	return []ent.Field{
		field.Int("site_id").Positive(),
		field.String("locale").MaxLen(16),
		field.Int("draft_revision_id").Optional().Nillable(),
		field.Int("published_revision_id").Optional().Nillable(),
		field.Enum("translation_status").Values("missing", "draft", "review", "published", "source_changed").Default("missing"),
		field.Int("version").Default(1),
	}
}
func (SiteLocalePublication) Indexes() []ent.Index {
	return []ent.Index{index.Fields("site_id", "locale").Unique()}
}
func (SiteLocalePublication) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("site", Site.Type).Ref("locale_publications").Field("site_id").Unique().Required(),
		edge.From("draft_revision", SiteContentRevision.Type).Ref("draft_for").Field("draft_revision_id").Unique(),
		edge.From("published_revision", SiteContentRevision.Type).Ref("published_for").Field("published_revision_id").Unique(),
	}
}
