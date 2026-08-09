package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"

	"github.com/forecho/6url/backend/ent/category"
	"github.com/forecho/6url/backend/ent/enttest"
	"github.com/forecho/6url/backend/ent/paymentorder"
	"github.com/forecho/6url/backend/ent/placement"
	"github.com/forecho/6url/backend/ent/site"
	"github.com/forecho/6url/backend/ent/submission"
	"github.com/forecho/6url/backend/internal/admintoken"
	"github.com/forecho/6url/backend/internal/backlink"
	"github.com/forecho/6url/backend/internal/config"
	"github.com/forecho/6url/backend/internal/securepayload"
)

type staticBacklinkChecker struct{ result backlink.Result }

func (s staticBacklinkChecker) Check(context.Context, string, string) backlink.Result {
	return s.result
}

func TestPaidSubmissionPublicationIsAtomic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := enttest.Open(t, dialect.SQLite, "file:paid-publication?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()
	categoryRow := client.Category.Create().SetSlug("tools").SetStatus(category.StatusPublished).SaveX(ctx)
	application := client.Submission.Create().SetPublicTokenHash("publish-token-hash").SetType(submission.TypePaid).SetStatus(submission.StatusPaid).SetApplicantEmail("owner@example.com").SetApplicantName("Owner").SetPreferredLocale("en").SetRawName("Example").SetRawURL("https://example.com").SetNormalizedURL("https://example.com").SetNormalizedRegistrableDomain("example.com").SetRawDescription("Example").SetRequestedCategoryID(categoryRow.ID).SaveX(ctx)
	plan := client.ListingPlan.Create().SetCode("listing-standard").SetName("Listing").SetAmountMinor(1500).SetCurrency("usd").SetStripePriceID("price_listing").SetActive(true).SaveX(ctx)
	order := client.PaymentOrder.Create().SetPurpose(paymentorder.PurposeListing).SetSubmissionID(application.ID).SetListingPlanID(plan.ID).SetProvider(paymentorder.ProviderStripe).SetStatus(paymentorder.StatusPaid).SetAmountMinor(1500).SetCurrency("usd").SetLocalesSnapshot([]string{"zh-CN", "en", "ja", "ko"}).SetStripePriceIDSnapshot("price_listing").SetPaidAt(time.Now()).SaveX(ctx)
	pepper := "01234567890123456789012345678901"
	rawAdmin, prefix, err := admintoken.Generate()
	if err != nil {
		t.Fatal(err)
	}
	client.AdminAPIToken.Create().SetName("publisher").SetPrefix(prefix).SetTokenHash(admintoken.Hash(rawAdmin, pepper)).SetScopes([]string{"content:publish"}).SaveX(ctx)
	sealer, err := securepayload.New(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	router := New(Dependencies{Config: config.Config{GinMode: gin.TestMode, AppBaseURL: "https://6url.net", CORSAllowedOrigins: []string{"https://6url.net"}, AdminTokenPepper: pepper}, Ent: client, PayloadSealer: &sealer})
	requestBody, _ := json.Marshal(map[string]any{"slug": "example", "localizations": publicationLocalizations()})
	request := httptest.NewRequest(http.MethodPost, "/admin/v1/submissions/"+strconv.Itoa(application.ID)+"/publish", bytes.NewReader(requestBody))
	request.Header.Set("Authorization", "Bearer "+rawAdmin)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if updated := client.Submission.GetX(ctx, application.ID); updated.Status != submission.StatusPublished {
		t.Fatalf("submission status=%s", updated.Status)
	}
	siteRow := client.Site.Query().OnlyX(ctx)
	if siteRow.Status != site.StatusPublished || siteRow.Slug != "example" {
		t.Fatalf("unexpected site: %#v", siteRow)
	}
	placementRow := client.Placement.Query().OnlyX(ctx)
	if placementRow.Kind != placement.KindPaid || placementRow.PaymentOrderID == nil || *placementRow.PaymentOrderID != order.ID {
		t.Fatalf("unexpected placement: %#v", placementRow)
	}
	if client.SiteContentRevision.Query().CountX(ctx) != 4 || client.SiteLocalePublication.Query().CountX(ctx) != 4 || client.PlacementLocale.Query().CountX(ctx) != 4 {
		t.Fatal("four-locale publication rows are incomplete")
	}
	if updatedOrder := client.PaymentOrder.GetX(ctx, order.ID); updatedOrder.SiteID == nil || *updatedOrder.SiteID != siteRow.ID {
		t.Fatalf("order site snapshot was not linked: %#v", updatedOrder)
	}
	if client.AdminAuditLog.Query().CountX(ctx) != 1 || client.RevalidationJob.Query().CountX(ctx) != 1 || client.NotificationDelivery.Query().CountX(ctx) != 1 {
		t.Fatal("publication outbox or audit side effects are incomplete")
	}
}

func TestExchangePublicationStoresInitialBacklinkCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := enttest.Open(t, dialect.SQLite, "file:exchange-publication?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()
	categoryRow := client.Category.Create().SetSlug("tools").SetStatus(category.StatusPublished).SaveX(ctx)
	backlinkURL := "https://example.com/links"
	application := client.Submission.Create().SetPublicTokenHash("exchange-publish-token").SetType(submission.TypeExchange).SetStatus(submission.StatusExchangeVerification).SetApplicantEmail("owner@example.com").SetApplicantName("Owner").SetPreferredLocale("en").SetRawName("Example").SetRawURL("https://example.com").SetNormalizedURL("https://example.com").SetNormalizedRegistrableDomain("example.com").SetRawDescription("Example").SetRequestedCategoryID(categoryRow.ID).SetBacklinkURL(backlinkURL).SaveX(ctx)
	pepper := "01234567890123456789012345678901"
	rawAdmin, prefix, _ := admintoken.Generate()
	client.AdminAPIToken.Create().SetName("publisher").SetPrefix(prefix).SetTokenHash(admintoken.Hash(rawAdmin, pepper)).SetScopes([]string{"content:publish"}).SaveX(ctx)
	sealer, _ := securepayload.New(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	checkedAt := time.Now().UTC()
	checker := staticBacklinkChecker{result: backlink.Result{Status: "passed", RequestedURL: backlinkURL, FinalURL: backlinkURL, MatchedHref: "https://6url.net/", MatchedRel: "nofollow", RedirectChain: []string{backlinkURL}, CheckedAt: checkedAt}}
	router := New(Dependencies{Config: config.Config{GinMode: gin.TestMode, AppBaseURL: "https://6url.net", CORSAllowedOrigins: []string{"https://6url.net"}, AdminTokenPepper: pepper}, Ent: client, PayloadSealer: &sealer, Backlinks: checker})
	requestBody, _ := json.Marshal(map[string]any{"slug": "example", "localizations": publicationLocalizations()})
	request := httptest.NewRequest(http.MethodPost, "/admin/v1/submissions/"+strconv.Itoa(application.ID)+"/publish", bytes.NewReader(requestBody))
	request.Header.Set("Authorization", "Bearer "+rawAdmin)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	placementRow := client.Placement.Query().OnlyX(ctx)
	if placementRow.Kind != placement.KindExchange || placementRow.LastCheckStatus == nil || *placementRow.LastCheckStatus != "passed" || client.BacklinkCheck.Query().CountX(ctx) != 1 {
		t.Fatalf("initial backlink check was not persisted: %#v", placementRow)
	}
}

func publicationLocalizations() []map[string]string {
	result := make([]map[string]string, 0, 4)
	for _, localeCode := range []string{"zh-CN", "en", "ja", "ko"} {
		result = append(result, map[string]string{"locale": localeCode, "name": "Example " + localeCode, "tagline": "Useful tool", "summary": "A useful directory tool.", "description": "A complete editorial description for Example.", "seo_title": "Example Tool", "seo_description": "Discover Example and its primary features."})
	}
	return result
}
