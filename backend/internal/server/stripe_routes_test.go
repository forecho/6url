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
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"

	"github.com/forecho/6url/backend/ent/enttest"
	"github.com/forecho/6url/backend/ent/paymentorder"
	"github.com/forecho/6url/backend/ent/submission"
	"github.com/forecho/6url/backend/internal/config"
	"github.com/forecho/6url/backend/internal/securepayload"
)

func TestStripeAsyncWebhookMarksMatchingOrderPaid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := enttest.Open(t, dialect.SQLite, "file:stripe-webhook?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()
	application := client.Submission.Create().SetPublicTokenHash("stripe-public-hash").SetType(submission.TypePaid).SetStatus(submission.StatusAwaitingPayment).SetApplicantEmail("owner@example.com").SetApplicantName("Owner").SetPreferredLocale("en").SetRawName("Example").SetRawURL("https://example.com").SetNormalizedURL("https://example.com").SetNormalizedRegistrableDomain("example.com").SetRawDescription("Example").SaveX(ctx)
	plan := client.ListingPlan.Create().SetCode("listing-standard").SetName("Listing").SetAmountMinor(1500).SetCurrency("usd").SetStripePriceID("price_listing").SetActive(true).SaveX(ctx)
	order := client.PaymentOrder.Create().SetPurpose(paymentorder.PurposeListing).SetSubmissionID(application.ID).SetListingPlanID(plan.ID).SetProvider(paymentorder.ProviderStripe).SetStatus(paymentorder.StatusProcessing).SetAmountMinor(1500).SetCurrency("usd").SetLivemode(false).SetStripePriceIDSnapshot("price_listing").SetStripeCheckoutSessionID("cs_test_1").SaveX(ctx)
	sealer, err := securepayload.New(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	secret := "whsec_test"
	router := New(Dependencies{Config: config.Config{GinMode: gin.TestMode, Environment: "test", AppBaseURL: "https://6url.net", CORSAllowedOrigins: []string{"https://6url.net"}, StripeWebhookSecret: secret}, Ent: client, PayloadSealer: &sealer})
	payload := []byte(fmt.Sprintf(`{"id":"evt_paid","type":"checkout.session.async_payment_succeeded","data":{"object":{"id":"cs_test_1","payment_status":"paid","payment_intent":"pi_test_1","amount_total":1500,"currency":"usd","livemode":false,"metadata":{"order_id":"%d"}}}}`, order.ID))
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
	if updated := client.PaymentOrder.GetX(ctx, order.ID); updated.Status != paymentorder.StatusPaid || updated.StripePaymentIntentID == nil || *updated.StripePaymentIntentID != "pi_test_1" {
		t.Fatalf("unexpected payment order: %#v", updated)
	}
	if updated := client.Submission.GetX(ctx, application.ID); updated.Status != submission.StatusPaid {
		t.Fatalf("unexpected submission status: %s", updated.Status)
	}
	if client.PaymentEvent.Query().CountX(ctx) != 1 || client.NotificationDelivery.Query().CountX(ctx) != 1 {
		t.Fatal("webhook side effects are incomplete")
	}
}

func TestStripeExpiredCheckoutCanBeRetried(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := enttest.Open(t, dialect.SQLite, "file:stripe-expired?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()
	application := client.Submission.Create().SetPublicTokenHash("expired-public-hash").SetType(submission.TypePaid).SetStatus(submission.StatusAwaitingPayment).SetApplicantEmail("owner@example.com").SetApplicantName("Owner").SetPreferredLocale("en").SetRawName("Example").SetRawURL("https://example.com").SetNormalizedURL("https://example.com").SetNormalizedRegistrableDomain("example.com").SetRawDescription("Example").SaveX(ctx)
	plan := client.ListingPlan.Create().SetCode("expired-plan").SetName("Listing").SetAmountMinor(1500).SetCurrency("usd").SetStripePriceID("price_expired").SetActive(true).SaveX(ctx)
	order := client.PaymentOrder.Create().SetPurpose(paymentorder.PurposeListing).SetSubmissionID(application.ID).SetListingPlanID(plan.ID).SetProvider(paymentorder.ProviderStripe).SetStatus(paymentorder.StatusProcessing).SetAmountMinor(1500).SetCurrency("usd").SetLivemode(false).SetStripePriceIDSnapshot("price_expired").SetStripeCheckoutSessionID("cs_expired_1").SaveX(ctx)
	secret := "whsec_expired"
	router := New(Dependencies{Config: config.Config{GinMode: gin.TestMode, Environment: "test", CORSAllowedOrigins: []string{"https://6url.net"}, StripeWebhookSecret: secret}, Ent: client})
	payload := []byte(fmt.Sprintf(`{"id":"evt_expired","type":"checkout.session.expired","data":{"object":{"id":"cs_expired_1","payment_status":"unpaid","amount_total":1500,"currency":"usd","livemode":false,"metadata":{"order_id":"%d"}}}}`, order.ID))
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
	updated := client.PaymentOrder.GetX(ctx, order.ID)
	if updated.Status != paymentorder.StatusPending || updated.StripeCheckoutSessionID != nil {
		t.Fatalf("expired order was not reset: %#v", updated)
	}
	event := client.PaymentEvent.Query().OnlyX(ctx)
	if event.ProcessedAt == nil {
		t.Fatal("expired checkout event was not marked processed")
	}
}
