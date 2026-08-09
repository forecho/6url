package legacyimport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode"

	"github.com/forecho/6url/backend/ent"
	"github.com/forecho/6url/backend/ent/category"
	"github.com/forecho/6url/backend/ent/categorycontentrevision"
	"github.com/forecho/6url/backend/ent/categorylocalepublication"
	"github.com/forecho/6url/backend/ent/placement"
	"github.com/forecho/6url/backend/ent/site"
	"github.com/forecho/6url/backend/ent/sitecategory"
	"github.com/forecho/6url/backend/ent/sitecontentrevision"
	"github.com/forecho/6url/backend/ent/sitelocalepublication"
	"github.com/forecho/6url/backend/internal/siteurl"
)

type Resource struct {
	Title string         `json:"title"`
	Icon  string         `json:"icon"`
	Sites []ResourceSite `json:"site"`
}

type ResourceSite struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Logo        string `json:"logo"`
}

type Result struct {
	CategoriesCreated int `json:"categories_created"`
	SitesCreated      int `json:"sites_created"`
	SitesReused       int `json:"sites_reused"`
}

type Importer struct {
	Client *ent.Client
}

func LoadResources(path string) ([]Resource, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read resources: %w", err)
	}
	var resources []Resource
	if err := json.Unmarshal(data, &resources); err != nil {
		return nil, fmt.Errorf("decode resources: %w", err)
	}
	if len(resources) == 0 {
		return nil, errors.New("resources file is empty")
	}
	return resources, nil
}

func (i Importer) Import(ctx context.Context, resources []Resource) (Result, error) {
	var result Result
	for categoryOrder, resource := range resources {
		categoryRow, created, err := i.ensureCategory(ctx, resource, categoryOrder)
		if err != nil {
			return result, err
		}
		if created {
			result.CategoriesCreated++
		}
		for siteOrder, item := range resource.Sites {
			created, err := i.ensureSite(ctx, categoryRow.ID, item, siteOrder)
			if err != nil {
				return result, fmt.Errorf("import %s/%s: %w", resource.Title, item.Title, err)
			}
			if created {
				result.SitesCreated++
			} else {
				result.SitesReused++
			}
		}
	}
	return result, nil
}

func (i Importer) ensureCategory(ctx context.Context, resource Resource, sortOrder int) (*ent.Category, bool, error) {
	slug := slugify(resource.Title, "category")
	row, err := i.Client.Category.Query().Where(category.SlugEQ(slug)).Only(ctx)
	if err == nil {
		return row, false, nil
	}
	if !ent.IsNotFound(err) {
		return nil, false, err
	}
	tx, err := i.Client.Tx(ctx)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()
	row, err = tx.Category.Create().SetSlug(slug).SetStatus(category.StatusDraft).SetSortOrder(sortOrder * 10).Save(ctx)
	if err != nil {
		return nil, false, err
	}
	revision, err := tx.CategoryContentRevision.Create().SetCategoryID(row.ID).SetLocale("zh-CN").SetRevision(1).SetStatus(categorycontentrevision.StatusDraft).SetTitle(strings.TrimSpace(resource.Title)).SetDescription("待编辑补充分类介绍。").SetSeoTitle(strings.TrimSpace(resource.Title) + "网站导航 | 6URL").SetSeoDescription("6URL 收录的" + strings.TrimSpace(resource.Title) + "网站，内容正在进行编辑审核。").SetSourceLocale("zh-CN").Save(ctx)
	if err != nil {
		return nil, false, err
	}
	if _, err := tx.CategoryLocalePublication.Create().SetCategoryID(row.ID).SetLocale("zh-CN").SetDraftRevisionID(revision.ID).SetTranslationStatus(categorylocalepublication.TranslationStatusDraft).Save(ctx); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return row.Unwrap(), true, nil
}

func (i Importer) ensureSite(ctx context.Context, categoryID int, item ResourceSite, sortOrder int) (bool, error) {
	normalized, err := siteurl.Normalize(item.URL)
	if err != nil {
		return false, err
	}
	row, err := i.Client.Site.Query().Where(site.CanonicalURLHashEQ(normalized.Hash)).Only(ctx)
	if err == nil {
		if err := i.ensureSiteCategory(ctx, row.ID, categoryID, sortOrder); err != nil {
			return false, err
		}
		return false, nil
	}
	if !ent.IsNotFound(err) {
		return false, err
	}
	tx, err := i.Client.Tx(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	slug := slugify(item.Title, "site")
	if exists, err := tx.Site.Query().Where(site.SlugEQ(slug)).Exist(ctx); err != nil {
		return false, err
	} else if exists {
		slug += "-" + shortHash(normalized.URL)
	}
	row, err = tx.Site.Create().SetCanonicalURL(normalized.URL).SetCanonicalURLHash(normalized.Hash).SetRegistrableDomain(normalized.RegistrableDomain).SetSlug(slug).SetStatus(site.StatusDraft).SetCreatedVia(site.CreatedViaMigration).Save(ctx)
	if err != nil {
		return false, err
	}
	description := strings.TrimSpace(item.Description)
	if description == "" {
		description = "待编辑补充网站介绍。"
	}
	revision, err := tx.SiteContentRevision.Create().SetSiteID(row.ID).SetLocale("zh-CN").SetRevision(1).SetStatus(sitecontentrevision.StatusDraft).SetName(strings.TrimSpace(item.Title)).SetTagline(description).SetSummary(description).SetDescription(description).SetSeoTitle(strings.TrimSpace(item.Title) + "：介绍、功能与适用场景 | 6URL").SetSeoDescription(description).SetSourceLocale("zh-CN").Save(ctx)
	if err != nil {
		return false, err
	}
	if _, err := tx.SiteLocalePublication.Create().SetSiteID(row.ID).SetLocale("zh-CN").SetDraftRevisionID(revision.ID).SetTranslationStatus(sitelocalepublication.TranslationStatusDraft).Save(ctx); err != nil {
		return false, err
	}
	if _, err := tx.SiteCategory.Create().SetSiteID(row.ID).SetCategoryID(categoryID).SetIsPrimary(true).SetSortOrder(sortOrder * 10).Save(ctx); err != nil {
		return false, err
	}
	placementRow, err := tx.Placement.Create().SetSiteID(row.ID).SetKind(placement.KindEditorial).SetOrigin(placement.OriginMigration).SetStatus(placement.StatusActive).SetOutboundURL(normalized.URL).SetOutboundRel("noopener").SetDisclosureKey("editorial").Save(ctx)
	if err != nil {
		return false, err
	}
	if _, err := tx.PlacementLocale.Create().SetPlacementID(placementRow.ID).SetLocale("zh-CN").Save(ctx); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (i Importer) ensureSiteCategory(ctx context.Context, siteID, categoryID, sortOrder int) error {
	exists, err := i.Client.SiteCategory.Query().Where(sitecategory.SiteIDEQ(siteID), sitecategory.CategoryIDEQ(categoryID)).Exist(ctx)
	if err != nil || exists {
		return err
	}
	_, err = i.Client.SiteCategory.Create().SetSiteID(siteID).SetCategoryID(categoryID).SetIsPrimary(false).SetSortOrder(sortOrder * 10).Save(ctx)
	return err
}

var repeatedHyphen = regexp.MustCompile(`-+`)

func slugify(value, fallbackPrefix string) string {
	var builder strings.Builder
	lastHyphen := false
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if r <= unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			builder.WriteRune(r)
			lastHyphen = false
			continue
		}
		if (unicode.IsSpace(r) || r == '-' || r == '_') && !lastHyphen && builder.Len() > 0 {
			builder.WriteByte('-')
			lastHyphen = true
		}
	}
	slug := strings.Trim(repeatedHyphen.ReplaceAllString(builder.String(), "-"), "-")
	if slug == "" {
		return fallbackPrefix + "-" + shortHash(value)
	}
	return slug
}

func shortHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:6])
}
