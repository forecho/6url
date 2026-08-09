package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"

	"github.com/forecho/6url/backend/ent"
	"github.com/forecho/6url/backend/ent/ad"
	"github.com/forecho/6url/backend/ent/adapplication"
	"github.com/forecho/6url/backend/ent/enttest"
	"github.com/forecho/6url/backend/ent/paymentorder"
	"github.com/forecho/6url/backend/ent/placement"
	"github.com/forecho/6url/backend/ent/site"
	"github.com/forecho/6url/backend/ent/submission"
	"github.com/forecho/6url/backend/internal/admintoken"
	"github.com/forecho/6url/backend/internal/config"
	"github.com/forecho/6url/backend/internal/securepayload"
	"github.com/forecho/6url/backend/internal/stripeclient"
)

type staticStripeRefund struct{ refund stripeclient.Refund }

func (s staticStripeRefund) Refund(context.Context, stripeclient.RefundRequest) (stripeclient.Refund, error) {
	return s.refund, nil
}

func TestSuccessfulSponsorRefundRevokesAd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := enttest.Open(t, dialect.SQLite, "file:sponsor-refund?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()
	application, order, adRow := createRefundableSponsor(t, client, paymentorder.StatusPaid, nil)
	pepper := "01234567890123456789012345678901"
	rawAdmin, prefix, _ := admintoken.Generate()
	client.AdminAPIToken.Create().SetName("finance").SetPrefix(prefix).SetTokenHash(admintoken.Hash(rawAdmin, pepper)).SetScopes([]string{"payments:write"}).SaveX(ctx)
	sealer, _ := securepayload.New(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	provider := staticStripeRefund{refund: stripeclient.Refund{ID: "re_success", PaymentIntentID: "pi_refund", Amount: 2500, Currency: "usd", Status: "succeeded"}}
	router := New(Dependencies{Config: config.Config{GinMode: gin.TestMode, CORSAllowedOrigins: []string{"https://6url.net"}, AdminTokenPepper: pepper}, Ent: client, PayloadSealer: &sealer, StripeRefund: provider})
	request := httptest.NewRequest(http.MethodPost, "/admin/v1/orders/"+strconv.Itoa(order.ID)+"/refund", bytes.NewBufferString(`{"reason":"customer request"}`))
	request.Header.Set("Authorization", "Bearer "+rawAdmin)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "refund-sponsor-order-1")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	updatedOrder := client.PaymentOrder.GetX(ctx, order.ID)
	if updatedOrder.Status != paymentorder.StatusRefunded || updatedOrder.RefundedAt == nil || updatedOrder.ExternalRef == nil || *updatedOrder.ExternalRef != "re_success" {
		t.Fatalf("unexpected refunded order: %#v", updatedOrder)
	}
	if updated := client.Ad.GetX(ctx, adRow.ID); updated.Status != ad.StatusCanceled {
		t.Fatalf("refunded ad status=%s", updated.Status)
	}
	if client.AdApplication.GetX(ctx, application.ID).Status != adapplication.StatusPublished || client.RevalidationJob.Query().CountX(ctx) != 1 || client.NotificationDelivery.Query().CountX(ctx) != 1 || client.PaymentEvent.Query().CountX(ctx) != 1 || client.AdminAuditLog.Query().CountX(ctx) != 1 {
		t.Fatal("refund side effects are incomplete")
	}
}

func TestRefundUpdatedWebhookCompletesPendingRefund(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := enttest.Open(t, dialect.SQLite, "file:sponsor-refund-webhook?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()
	refundID := "re_pending"
	_, order, adRow := createRefundableSponsor(t, client, paymentorder.StatusRefundPending, &refundID)
	sealer, _ := securepayload.New(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	secret := "whsec_refund"
	router := New(Dependencies{Config: config.Config{GinMode: gin.TestMode, CORSAllowedOrigins: []string{"https://6url.net"}, StripeWebhookSecret: secret}, Ent: client, PayloadSealer: &sealer})
	payload := []byte(`{"id":"evt_refund_success","type":"refund.updated","data":{"object":{"id":"re_pending","payment_intent":"pi_refund","amount":2500,"currency":"usd","status":"succeeded"}}}`)
	timestamp := time.Now().Unix()
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = fmt.Fprintf(mac, "%d.", timestamp)
	_, _ = mac.Write(payload)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/stripe", bytes.NewReader(payload))
	request.Header.Set("Stripe-Signature", fmt.Sprintf("t=%d,v1=%s", timestamp, hex.EncodeToString(mac.Sum(nil))))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if client.PaymentOrder.GetX(ctx, order.ID).Status != paymentorder.StatusRefunded || client.Ad.GetX(ctx, adRow.ID).Status != ad.StatusCanceled || client.RevalidationJob.Query().CountX(ctx) != 1 || client.NotificationDelivery.Query().CountX(ctx) != 1 {
		t.Fatal("pending refund was not completed atomically")
	}
}

func TestSuccessfulListingRefundEndsPlacementAndKeepsSite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := enttest.Open(t, dialect.SQLite, "file:listing-refund?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()
	application := client.Submission.Create().SetPublicTokenHash("listing-refund-token").SetType(submission.TypePaid).SetStatus(submission.StatusPublished).SetApplicantEmail("owner@example.com").SetApplicantName("Owner").SetPreferredLocale("en").SetRawName("Example").SetRawURL("https://example.com").SetNormalizedURL("https://example.com").SetNormalizedRegistrableDomain("example.com").SetRawDescription("Description").SaveX(ctx)
	siteRow := client.Site.Create().SetCanonicalURL("https://example.com").SetCanonicalURLHash(stringDigest("https://example.com")).SetRegistrableDomain("example.com").SetSlug("example").SetStatus(site.StatusPublished).SetCreatedVia(site.CreatedViaSubmission).SaveX(ctx)
	plan := client.ListingPlan.Create().SetCode("listing-refund").SetName("Listing").SetAmountMinor(1500).SetCurrency("usd").SaveX(ctx)
	order := client.PaymentOrder.Create().SetPurpose(paymentorder.PurposeListing).SetSubmissionID(application.ID).SetSiteID(siteRow.ID).SetListingPlanID(plan.ID).SetProvider(paymentorder.ProviderStripe).SetStatus(paymentorder.StatusPaid).SetAmountMinor(1500).SetCurrency("usd").SetStripePaymentIntentID("pi_listing").SetPaidAt(time.Now().Add(-24 * time.Hour)).SaveX(ctx)
	placementRow := client.Placement.Create().SetSiteID(siteRow.ID).SetKind(placement.KindPaid).SetOrigin(placement.OriginSubmission).SetStatus(placement.StatusActive).SetSubmissionID(application.ID).SetPaymentOrderID(order.ID).SetOutboundURL("https://example.com").SetOutboundRel("sponsored nofollow noopener").SaveX(ctx)
	client.PlacementLocale.Create().SetPlacementID(placementRow.ID).SetLocale("en").SaveX(ctx)
	pepper := "01234567890123456789012345678901"
	rawAdmin, prefix, _ := admintoken.Generate()
	client.AdminAPIToken.Create().SetName("finance").SetPrefix(prefix).SetTokenHash(admintoken.Hash(rawAdmin, pepper)).SetScopes([]string{"payments:write"}).SaveX(ctx)
	sealer, _ := securepayload.New(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	provider := staticStripeRefund{refund: stripeclient.Refund{ID: "re_listing", PaymentIntentID: "pi_listing", Amount: 1500, Currency: "usd", Status: "succeeded"}}
	router := New(Dependencies{Config: config.Config{GinMode: gin.TestMode, CORSAllowedOrigins: []string{"https://6url.net"}, AdminTokenPepper: pepper}, Ent: client, PayloadSealer: &sealer, StripeRefund: provider})
	request := httptest.NewRequest(http.MethodPost, "/admin/v1/orders/"+strconv.Itoa(order.ID)+"/refund", bytes.NewBufferString(`{"reason":"listing refund"}`))
	request.Header.Set("Authorization", "Bearer "+rawAdmin)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "refund-listing-order-1")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if client.Placement.GetX(ctx, placementRow.ID).Status != placement.StatusCanceled || client.Site.GetX(ctx, siteRow.ID).Status != site.StatusPublished || client.Submission.GetX(ctx, application.ID).Status != submission.StatusPublished {
		t.Fatal("listing refund did not preserve content while ending the paid placement")
	}
	if client.RevalidationJob.Query().CountX(ctx) != 1 || client.NotificationDelivery.Query().CountX(ctx) != 1 {
		t.Fatal("listing refund side effects are incomplete")
	}
}

func createRefundableSponsor(t *testing.T, client *ent.Client, status paymentorder.Status, externalRef *string) (*ent.AdApplication, *ent.PaymentOrder, *ent.Ad) {
	t.Helper()
	ctx := context.Background()
	assetRow := client.Asset.Create().SetProvider("imgant").SetAlbum("6url").SetExternalID("refund-image-" + string(status)).SetStorageKey("6url/refund-" + string(status) + ".webp").SetPublicURL("https://cdn.imgant.com/refund-" + string(status) + ".webp").SetContentHash(stringDigest("refund-image-" + string(status))).SetMimeType("image/webp").SetSizeBytes(1024).SetWidth(1200).SetHeight(630).SaveX(ctx)
	application := client.AdApplication.Create().SetPublicTokenHash(stringDigest("refund-application-" + string(status))).SetStatus(adapplication.StatusPublished).SetApplicantEmail("advertiser@example.com").SetApplicantName("Advertiser").SetPreferredLocale("en").SetTargetURL("https://example.com/sponsor").SetRequestedLocales([]string{"zh-CN", "en", "ja", "ko"}).SetRawTitle("Sponsor").SetRawDescription("Description").SaveX(ctx)
	offer := client.AdOffer.Create().SetCode("refund-offer-" + string(status)).SetName("Sponsor").SetAmountMinor(2500).SetCurrency("usd").SetDurationDays(30).SaveX(ctx)
	orderCreator := client.PaymentOrder.Create().SetPurpose(paymentorder.PurposeAd).SetAdApplicationID(application.ID).SetAdOfferID(offer.ID).SetProvider(paymentorder.ProviderStripe).SetStatus(status).SetAmountMinor(2500).SetCurrency("usd").SetDurationDaysSnapshot(30).SetStripePaymentIntentID("pi_refund").SetPaidAt(time.Now().Add(-24 * time.Hour))
	if externalRef != nil {
		orderCreator.SetExternalRef(*externalRef)
	}
	order := orderCreator.SaveX(ctx)
	adRow := client.Ad.Create().SetAdApplicationID(application.ID).SetPaymentOrderID(order.ID).SetStatus(ad.StatusActive).SetTargetURL(application.TargetURL).SetImageAssetID(assetRow.ID).SetStartsAt(time.Now().Add(-24 * time.Hour)).SetEndsAt(time.Now().Add(29 * 24 * time.Hour)).SetOrigin(ad.OriginAdvertiser).SaveX(ctx)
	return application, order, adRow
}
