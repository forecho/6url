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

	"github.com/forecho/6url/backend/ent/ad"
	"github.com/forecho/6url/backend/ent/adapplication"
	"github.com/forecho/6url/backend/ent/enttest"
	"github.com/forecho/6url/backend/ent/paymentorder"
	"github.com/forecho/6url/backend/internal/admintoken"
	"github.com/forecho/6url/backend/internal/config"
	"github.com/forecho/6url/backend/internal/securepayload"
)

func TestPaidSponsorPublicationIsAtomic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := enttest.Open(t, dialect.SQLite, "file:paid-sponsor-publication?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()
	assetRow := client.Asset.Create().SetProvider("imgant").SetAlbum("6url").SetExternalID("image-1").SetStorageKey("6url/image-1.webp").SetPublicURL("https://cdn.imgant.com/image-1.webp").SetContentHash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa").SetMimeType("image/webp").SetSizeBytes(1024).SetWidth(1200).SetHeight(630).SaveX(ctx)
	application := client.AdApplication.Create().SetPublicTokenHash("paid-ad-token").SetStatus(adapplication.StatusPaid).SetApplicantEmail("advertiser@example.com").SetApplicantName("Advertiser").SetPreferredLocale("en").SetTargetURL("https://example.com/product").SetRequestedLocales([]string{"zh-CN", "en", "ja", "ko"}).SetRawTitle("Example Sponsor").SetRawDescription("Sponsor description").SaveX(ctx)
	offer := client.AdOffer.Create().SetCode("sponsor-standard").SetName("Sponsor").SetAmountMinor(2500).SetCurrency("usd").SetDurationDays(30).SetIncludedLocales([]string{"zh-CN", "en", "ja", "ko"}).SetStripePriceID("price_ad").SetActive(true).SaveX(ctx)
	order := client.PaymentOrder.Create().SetPurpose(paymentorder.PurposeAd).SetAdApplicationID(application.ID).SetAdOfferID(offer.ID).SetProvider(paymentorder.ProviderStripe).SetStatus(paymentorder.StatusPaid).SetAmountMinor(2500).SetCurrency("usd").SetDurationDaysSnapshot(30).SetLocalesSnapshot([]string{"zh-CN", "en", "ja", "ko"}).SetStripePriceIDSnapshot("price_ad").SetPaidAt(time.Now()).SaveX(ctx)
	pepper := "01234567890123456789012345678901"
	rawAdmin, prefix, _ := admintoken.Generate()
	client.AdminAPIToken.Create().SetName("ads").SetPrefix(prefix).SetTokenHash(admintoken.Hash(rawAdmin, pepper)).SetScopes([]string{"ads:write"}).SaveX(ctx)
	sealer, _ := securepayload.New(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	router := New(Dependencies{Config: config.Config{GinMode: gin.TestMode, CORSAllowedOrigins: []string{"https://6url.net"}, AdminTokenPepper: pepper}, Ent: client, PayloadSealer: &sealer})
	body, _ := json.Marshal(map[string]any{"image_asset_id": assetRow.ID, "sort_order": 20, "localizations": sponsorLocalizations()})
	request := httptest.NewRequest(http.MethodPost, "/admin/v1/ad-applications/"+strconv.Itoa(application.ID)+"/publish", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+rawAdmin)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	adRow := client.Ad.Query().OnlyX(ctx)
	if adRow.Status != ad.StatusActive || adRow.Origin != ad.OriginAdvertiser || adRow.PaymentOrderID == nil || *adRow.PaymentOrderID != order.ID || adRow.EndsAt == nil {
		t.Fatalf("unexpected ad: %#v", adRow)
	}
	if client.AdLocalization.Query().CountX(ctx) != 4 || client.AdApplication.GetX(ctx, application.ID).Status != adapplication.StatusPublished {
		t.Fatal("advertisement publication rows are incomplete")
	}
	if client.AdminAuditLog.Query().CountX(ctx) != 1 || client.RevalidationJob.Query().CountX(ctx) != 1 || client.NotificationDelivery.Query().CountX(ctx) != 1 {
		t.Fatal("advertisement publication side effects are incomplete")
	}
}

func TestManualSponsorCommitRequiresMatchingVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := enttest.Open(t, dialect.SQLite, "file:manual-sponsor-commit?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()
	assetRow := client.Asset.Create().SetProvider("imgant").SetAlbum("6url").SetExternalID("image-2").SetStorageKey("6url/image-2.webp").SetPublicURL("https://cdn.imgant.com/image-2.webp").SetContentHash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb").SetMimeType("image/webp").SetSizeBytes(1024).SetWidth(1200).SetHeight(630).SaveX(ctx)
	adRow := client.Ad.Create().SetStatus(ad.StatusDraft).SetTargetURL("https://example.com").SetImageAssetID(assetRow.ID).SetOrigin(ad.OriginAdmin).SaveX(ctx)
	for _, item := range sponsorLocalizations() {
		client.AdLocalization.Create().SetAdID(adRow.ID).SetLocale(item["locale"]).SetTitle(item["title"]).SetDescription(item["description"]).SetCtaLabel(item["cta_label"]).SetAltText(item["alt_text"]).SaveX(ctx)
	}
	pepper := "01234567890123456789012345678901"
	rawAdmin, prefix, _ := admintoken.Generate()
	client.AdminAPIToken.Create().SetName("ads").SetPrefix(prefix).SetTokenHash(admintoken.Hash(rawAdmin, pepper)).SetScopes([]string{"ads:write"}).SaveX(ctx)
	router := New(Dependencies{Config: config.Config{GinMode: gin.TestMode, CORSAllowedOrigins: []string{"https://6url.net"}, AdminTokenPepper: pepper}, Ent: client})
	body := []byte(`{"status":"active","sort_order":5,"version":1}`)
	request := httptest.NewRequest(http.MethodPost, "/admin/v1/ads/"+strconv.Itoa(adRow.ID)+"/commit", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+rawAdmin)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("If-Match", `W/"ad-`+strconv.Itoa(adRow.ID)+`-v1"`)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	updated := client.Ad.GetX(ctx, adRow.ID)
	if updated.Status != ad.StatusActive || updated.Version != 2 || client.RevalidationJob.Query().CountX(ctx) != 1 {
		t.Fatalf("unexpected committed ad: %#v", updated)
	}
}

func sponsorLocalizations() []map[string]string {
	result := make([]map[string]string, 0, 4)
	for _, localeCode := range []string{"zh-CN", "en", "ja", "ko"} {
		result = append(result, map[string]string{"locale": localeCode, "title": "Sponsor " + localeCode, "description": "Sponsor description", "cta_label": "Visit", "alt_text": "Sponsor image"})
	}
	return result
}
