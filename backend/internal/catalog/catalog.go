package catalog

import (
	"context"
	"sort"
	"time"

	"github.com/forecho/6url/backend/ent"
	"github.com/forecho/6url/backend/ent/ad"
	"github.com/forecho/6url/backend/ent/adlocalization"
	"github.com/forecho/6url/backend/ent/asset"
	"github.com/forecho/6url/backend/ent/category"
	"github.com/forecho/6url/backend/ent/categorycontentrevision"
	"github.com/forecho/6url/backend/ent/categorylocalepublication"
	"github.com/forecho/6url/backend/ent/placement"
	"github.com/forecho/6url/backend/ent/placementlocale"
	"github.com/forecho/6url/backend/ent/site"
	"github.com/forecho/6url/backend/ent/sitecategory"
	"github.com/forecho/6url/backend/ent/sitecontentrevision"
	"github.com/forecho/6url/backend/ent/sitelocalepublication"
)

type Catalog struct {
	Locale     string     `json:"locale"`
	Categories []Category `json:"categories"`
	Ads        []Ad       `json:"ads"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type Category struct {
	ID          int       `json:"id"`
	Slug        string    `json:"slug"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Sites       []Site    `json:"sites"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Site struct {
	ID          int       `json:"id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Tagline     string    `json:"tagline"`
	Summary     string    `json:"summary"`
	LogoURL     string    `json:"logo_url,omitempty"`
	OutboundURL string    `json:"outbound_url"`
	OutboundRel string    `json:"outbound_rel"`
	Disclosures []string  `json:"disclosures"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Ad struct {
	ID          int        `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	CTALabel    string     `json:"cta_label"`
	AltText     string     `json:"alt_text"`
	ImageURL    string     `json:"image_url"`
	TargetURL   string     `json:"target_url"`
	Rel         string     `json:"rel"`
	Label       string     `json:"label"`
	EndsAt      *time.Time `json:"ends_at,omitempty"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type Service struct {
	Client *ent.Client
	Now    func() time.Time
}

func (s Service) Load(ctx context.Context, localeCode string) (Catalog, error) {
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	result := Catalog{Locale: localeCode, Categories: []Category{}, Ads: []Ad{}, UpdatedAt: time.Unix(0, 0).UTC()}
	categoryRows, err := s.Client.Category.Query().Where(category.StatusEQ(category.StatusPublished)).Order(ent.Asc(category.FieldSortOrder), ent.Asc(category.FieldID)).All(ctx)
	if err != nil {
		return result, err
	}
	for _, categoryRow := range categoryRows {
		publication, err := s.Client.CategoryLocalePublication.Query().Where(categorylocalepublication.CategoryIDEQ(categoryRow.ID), categorylocalepublication.LocaleEQ(localeCode), categorylocalepublication.PublishedRevisionIDNotNil()).Only(ctx)
		if ent.IsNotFound(err) {
			continue
		}
		if err != nil {
			return result, err
		}
		revision, err := s.Client.CategoryContentRevision.Query().Where(
			categorycontentrevision.IDEQ(*publication.PublishedRevisionID),
			categorycontentrevision.CategoryIDEQ(categoryRow.ID),
			categorycontentrevision.LocaleEQ(localeCode),
			categorycontentrevision.StatusEQ(categorycontentrevision.StatusApproved),
		).Only(ctx)
		if err != nil {
			return result, err
		}
		item := Category{ID: categoryRow.ID, Slug: categoryRow.Slug, Title: revision.Title, Description: revision.Description, Sites: []Site{}, UpdatedAt: latestTime(categoryRow.UpdatedAt, publication.UpdatedAt, revision.UpdatedAt)}
		siteLinks, err := s.Client.SiteCategory.Query().Where(sitecategory.CategoryIDEQ(categoryRow.ID)).Order(ent.Asc(sitecategory.FieldSortOrder), ent.Asc(sitecategory.FieldID)).All(ctx)
		if err != nil {
			return result, err
		}
		for _, link := range siteLinks {
			siteItem, ok, err := s.loadSite(ctx, link.SiteID, localeCode, now)
			if err != nil {
				return result, err
			}
			if ok {
				item.Sites = append(item.Sites, siteItem)
				item.UpdatedAt = latestTime(item.UpdatedAt, siteItem.UpdatedAt)
			}
		}
		result.Categories = append(result.Categories, item)
		result.UpdatedAt = latestTime(result.UpdatedAt, item.UpdatedAt)
	}
	result.Ads, err = s.loadAds(ctx, localeCode, now)
	for _, item := range result.Ads {
		result.UpdatedAt = latestTime(result.UpdatedAt, item.UpdatedAt)
	}
	return result, err
}

func (s Service) loadSite(ctx context.Context, siteID int, localeCode string, now time.Time) (Site, bool, error) {
	row, err := s.Client.Site.Query().Where(site.IDEQ(siteID), site.StatusEQ(site.StatusPublished)).Only(ctx)
	if ent.IsNotFound(err) {
		return Site{}, false, nil
	}
	if err != nil {
		return Site{}, false, err
	}
	publication, err := s.Client.SiteLocalePublication.Query().Where(sitelocalepublication.SiteIDEQ(siteID), sitelocalepublication.LocaleEQ(localeCode), sitelocalepublication.PublishedRevisionIDNotNil()).Only(ctx)
	if ent.IsNotFound(err) {
		return Site{}, false, nil
	}
	if err != nil {
		return Site{}, false, err
	}
	revision, err := s.Client.SiteContentRevision.Query().Where(
		sitecontentrevision.IDEQ(*publication.PublishedRevisionID),
		sitecontentrevision.SiteIDEQ(siteID),
		sitecontentrevision.LocaleEQ(localeCode),
		sitecontentrevision.StatusEQ(sitecontentrevision.StatusApproved),
	).Only(ctx)
	if err != nil {
		return Site{}, false, err
	}
	placements, err := s.Client.Placement.Query().Where(placement.SiteIDEQ(siteID), placement.StatusEQ(placement.StatusActive)).All(ctx)
	if err != nil {
		return Site{}, false, err
	}
	eligible := make([]*ent.Placement, 0, len(placements))
	for _, candidate := range placements {
		if candidate.StartsAt.After(now) || candidate.EndsAt != nil && !candidate.EndsAt.After(now) {
			continue
		}
		covered, err := s.Client.PlacementLocale.Query().Where(placementlocale.PlacementIDEQ(candidate.ID), placementlocale.LocaleEQ(localeCode)).Exist(ctx)
		if err != nil {
			return Site{}, false, err
		}
		if covered {
			eligible = append(eligible, candidate)
		}
	}
	if len(eligible) == 0 {
		return Site{}, false, nil
	}
	selected, disclosures := resolvePlacement(eligible)
	logoURL := ""
	updatedAt := latestTime(row.UpdatedAt, publication.UpdatedAt, revision.UpdatedAt, selected.UpdatedAt)
	if row.LogoAssetID != nil {
		assetRow, assetErr := s.Client.Asset.Query().Where(asset.IDEQ(*row.LogoAssetID), asset.StatusEQ(asset.StatusHealthy)).Only(ctx)
		if assetErr == nil {
			logoURL = assetRow.PublicURL
			updatedAt = latestTime(updatedAt, assetRow.UpdatedAt)
		} else if !ent.IsNotFound(assetErr) {
			return Site{}, false, assetErr
		}
	}
	outboundURL := row.CanonicalURL
	if selected.OutboundURL != nil && *selected.OutboundURL != "" {
		outboundURL = *selected.OutboundURL
	}
	return Site{ID: row.ID, Slug: row.Slug, Name: revision.Name, Tagline: revision.Tagline, Summary: revision.Summary, LogoURL: logoURL, OutboundURL: outboundURL, OutboundRel: selected.OutboundRel, Disclosures: disclosures, UpdatedAt: updatedAt}, true, nil
}

func resolvePlacement(rows []*ent.Placement) (*ent.Placement, []string) {
	priority := map[placement.Kind]int{placement.KindPaid: 0, placement.KindAffiliate: 0, placement.KindExchange: 1, placement.KindEditorial: 2, placement.KindOwned: 2}
	sort.Slice(rows, func(i, j int) bool {
		left, right := rows[i], rows[j]
		if priority[left.Kind] != priority[right.Kind] {
			return priority[left.Kind] < priority[right.Kind]
		}
		if !left.StartsAt.Equal(right.StartsAt) {
			return left.StartsAt.Before(right.StartsAt)
		}
		return left.ID < right.ID
	})
	selected := rows[0]
	disclosures := make([]string, 0, len(rows))
	seen := map[string]struct{}{}
	for _, row := range rows {
		if row.DisclosureKey != "" {
			if _, ok := seen[row.DisclosureKey]; !ok {
				seen[row.DisclosureKey] = struct{}{}
				disclosures = append(disclosures, row.DisclosureKey)
			}
		}
	}
	if priority[selected.Kind] == 0 {
		selected.OutboundRel = "sponsored noopener"
	} else if selected.Kind == placement.KindExchange {
		selected.OutboundRel = "nofollow noopener"
	} else {
		selected.OutboundRel = "noopener"
	}
	return selected, disclosures
}

func (s Service) loadAds(ctx context.Context, localeCode string, now time.Time) ([]Ad, error) {
	rows, err := s.Client.Ad.Query().Where(ad.StatusEQ(ad.StatusActive)).Order(ent.Asc(ad.FieldSortOrder), ent.Asc(ad.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]Ad, 0, len(rows))
	for _, row := range rows {
		if row.StartsAt != nil && row.StartsAt.After(now) || row.EndsAt != nil && !row.EndsAt.After(now) {
			continue
		}
		localization, err := s.Client.AdLocalization.Query().Where(adlocalization.AdIDEQ(row.ID), adlocalization.LocaleEQ(localeCode)).Only(ctx)
		if ent.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		image, err := s.Client.Asset.Query().Where(asset.IDEQ(row.ImageAssetID), asset.StatusEQ(asset.StatusHealthy)).Only(ctx)
		if ent.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		result = append(result, Ad{ID: row.ID, Title: localization.Title, Description: localization.Description, CTALabel: localization.CtaLabel, AltText: localization.AltText, ImageURL: image.PublicURL, TargetURL: row.TargetURL, Rel: "sponsored noopener", Label: "Sponsor / 广告", EndsAt: row.EndsAt, UpdatedAt: latestTime(row.UpdatedAt, localization.UpdatedAt, image.UpdatedAt)})
	}
	return result, nil
}

func latestTime(values ...time.Time) time.Time {
	latest := time.Unix(0, 0).UTC()
	for _, value := range values {
		if value.After(latest) {
			latest = value
		}
	}
	return latest
}
