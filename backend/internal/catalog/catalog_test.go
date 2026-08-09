package catalog

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	_ "github.com/mattn/go-sqlite3"

	"github.com/forecho/6url/backend/ent/ad"
	"github.com/forecho/6url/backend/ent/category"
	"github.com/forecho/6url/backend/ent/categorycontentrevision"
	"github.com/forecho/6url/backend/ent/enttest"
	"github.com/forecho/6url/backend/ent/placement"
	"github.com/forecho/6url/backend/ent/site"
	"github.com/forecho/6url/backend/ent/sitecontentrevision"
	"github.com/forecho/6url/backend/ent/submission"
)

func TestLoadSeparatesPublishedCatalogAndLocalizedAds(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:catalog?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()
	now := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	categoryRow := client.Category.Create().SetSlug("tools").SetStatus(category.StatusPublished).SaveX(ctx)
	categoryRevision := client.CategoryContentRevision.Create().SetCategoryID(categoryRow.ID).SetLocale("zh-CN").SetRevision(1).SetStatus(categorycontentrevision.StatusApproved).SetTitle("工具").SetDescription("投资工具").SetSeoTitle("工具").SetSeoDescription("工具").SetSourceLocale("zh-CN").SaveX(ctx)
	client.CategoryLocalePublication.Create().SetCategoryID(categoryRow.ID).SetLocale("zh-CN").SetPublishedRevisionID(categoryRevision.ID).SetTranslationStatus("published").SaveX(ctx)

	siteRow := client.Site.Create().SetCanonicalURL("https://example.com").SetCanonicalURLHash("100680ad546ce6a577f42f52df33b4cf1c2c8e39a3095b0dbbb6e5b19093c1f0").SetRegistrableDomain("example.com").SetSlug("example").SetStatus(site.StatusPublished).SetCreatedVia(site.CreatedViaAdminManual).SaveX(ctx)
	siteRevision := client.SiteContentRevision.Create().SetSiteID(siteRow.ID).SetLocale("zh-CN").SetRevision(1).SetStatus(sitecontentrevision.StatusApproved).SetName("Example").SetTagline("示例工具").SetSummary("摘要").SetDescription("介绍").SetSeoTitle("Example").SetSeoDescription("Example").SetSourceLocale("zh-CN").SaveX(ctx)
	client.SiteLocalePublication.Create().SetSiteID(siteRow.ID).SetLocale("zh-CN").SetPublishedRevisionID(siteRevision.ID).SetTranslationStatus("published").SaveX(ctx)
	client.SiteCategory.Create().SetSiteID(siteRow.ID).SetCategoryID(categoryRow.ID).SetIsPrimary(true).SaveX(ctx)
	editorial := client.Placement.Create().SetSiteID(siteRow.ID).SetKind(placement.KindEditorial).SetOrigin(placement.OriginAdmin).SetStatus(placement.StatusActive).SetStartsAt(now.Add(-time.Hour)).SetOutboundURL("https://example.com").SetDisclosureKey("editorial").SaveX(ctx)
	client.PlacementLocale.Create().SetPlacementID(editorial.ID).SetLocale("zh-CN").SaveX(ctx)
	paidSubmission := client.Submission.Create().SetPublicTokenHash("submission-token-hash").SetType(submission.TypePaid).SetApplicantEmail("owner@example.com").SetApplicantName("Owner").SetPreferredLocale("zh-CN").SetRawName("Example").SetRawURL("https://example.com").SetNormalizedURL("https://example.com").SetNormalizedRegistrableDomain("example.com").SetRawDescription("Example").SaveX(ctx)
	listingPlan := client.ListingPlan.Create().SetCode("standard").SetName("Standard").SetAmountMinor(1000).SetCurrency("usd").SaveX(ctx)
	order := client.PaymentOrder.Create().SetPurpose("listing").SetSubmissionID(paidSubmission.ID).SetSiteID(siteRow.ID).SetListingPlanID(listingPlan.ID).SetProvider("stripe").SetAmountMinor(1000).SetCurrency("usd").SaveX(ctx)
	paid := client.Placement.Create().SetSiteID(siteRow.ID).SetKind(placement.KindPaid).SetOrigin(placement.OriginSubmission).SetSubmissionID(paidSubmission.ID).SetPaymentOrderID(order.ID).SetStatus(placement.StatusActive).SetStartsAt(now.Add(-time.Hour)).SetOutboundURL("https://example.com/paid").SetDisclosureKey("paid").SaveX(ctx)
	client.PlacementLocale.Create().SetPlacementID(paid.ID).SetLocale("zh-CN").SaveX(ctx)

	image := client.Asset.Create().SetExternalID("image-1").SetStorageKey("image-1").SetPublicURL("https://cdn.imgant.com/image-1.png").SetContentHash("hash-1").SetMimeType("image/png").SetSizeBytes(100).SetWidth(32).SetHeight(32).SaveX(ctx)
	adRow := client.Ad.Create().SetStatus(ad.StatusActive).SetTargetURL("https://advertiser.example").SetImageAssetID(image.ID).SetOrigin(ad.OriginAdmin).SetSortOrder(10).SaveX(ctx)
	client.AdLocalization.Create().SetAdID(adRow.ID).SetLocale("zh-CN").SetTitle("广告").SetDescription("广告说明").SetCtaLabel("查看").SetAltText("广告图片").SaveX(ctx)

	result, err := (Service{Client: client, Now: func() time.Time { return now }}).Load(ctx, "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Categories) != 1 || len(result.Categories[0].Sites) != 1 {
		t.Fatalf("unexpected catalog: %#v", result)
	}
	if result.Categories[0].Sites[0].OutboundRel != "sponsored noopener" {
		t.Fatalf("paid relation was not resolved: %#v", result.Categories[0].Sites[0])
	}
	if len(result.Ads) != 1 || result.Ads[0].Rel != "sponsored noopener" || result.Ads[0].Label == "" {
		t.Fatalf("unexpected ads: %#v", result.Ads)
	}

	koResult, err := (Service{Client: client, Now: func() time.Time { return now }}).Load(ctx, "ko")
	if err != nil {
		t.Fatal(err)
	}
	if len(koResult.Ads) != 0 {
		t.Fatalf("ad localization must not fall back: %#v", koResult.Ads)
	}
}
