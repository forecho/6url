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
	"strings"
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
	payload := []byte(fmt.Sprintf(`{"id":"evt_expired","type":"checkout.session.expired","data":{"object":{"id":"cs_expired_1","payment_status":"unpaid","amount_total":1500,"currency":"usd","livemode":false,"metadata":{"order_id":"%d","sixurl_app":"6url.net","sixurl_order_id":"%d"}}}}`, order.ID, order.ID))
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

func TestStripeWebhookIgnoresAnotherProductsCheckout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := enttest.Open(t, dialect.SQLite, "file:stripe-other-checkout?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()
	application := client.Submission.Create().SetPublicTokenHash("other-checkout-public-hash").SetType(submission.TypePaid).SetStatus(submission.StatusAwaitingPayment).SetApplicantEmail("owner@example.com").SetApplicantName("Owner").SetPreferredLocale("en").SetRawName("Example").SetRawURL("https://example.com").SetNormalizedURL("https://example.com").SetNormalizedRegistrableDomain("example.com").SetRawDescription("Example").SaveX(ctx)
	plan := client.ListingPlan.Create().SetCode("other-checkout-plan").SetName("Listing").SetAmountMinor(1500).SetCurrency("usd").SetStripePriceID("price_other_checkout").SetActive(true).SaveX(ctx)
	order := client.PaymentOrder.Create().SetPurpose(paymentorder.PurposeListing).SetSubmissionID(application.ID).SetListingPlanID(plan.ID).SetProvider(paymentorder.ProviderStripe).SetStatus(paymentorder.StatusProcessing).SetAmountMinor(1500).SetCurrency("usd").SetLivemode(false).SetStripeCheckoutSessionID("cs_6url").SaveX(ctx)
	secret := "whsec_other_checkout"
	router := New(Dependencies{Config: config.Config{GinMode: gin.TestMode, Environment: "test", CORSAllowedOrigins: []string{"https://6url.net"}, StripeWebhookSecret: secret}, Ent: client})
	payload := []byte(fmt.Sprintf(`{"id":"evt_other_checkout","type":"checkout.session.completed","data":{"object":{"id":"cs_another_product","payment_status":"paid","payment_intent":"pi_another_product","amount_total":1500,"currency":"usd","livemode":false,"metadata":{"order_id":"%d"}}}}`, order.ID))
	response := postSignedStripeWebhook(t, router, secret, payload)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"ignored":true`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if updated := client.PaymentOrder.GetX(ctx, order.ID); updated.Status != paymentorder.StatusProcessing {
		t.Fatalf("another product changed the local order: %#v", updated)
	}
	if client.PaymentEvent.Query().CountX(ctx) != 0 {
		t.Fatal("another product Checkout event was recorded")
	}
}

func TestStripeWebhookIgnoresAnotherProductsRefund(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := enttest.Open(t, dialect.SQLite, "file:stripe-other-refund?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()
	secret := "whsec_other_refund"
	router := New(Dependencies{Config: config.Config{GinMode: gin.TestMode, Environment: "test", CORSAllowedOrigins: []string{"https://6url.net"}, StripeWebhookSecret: secret}, Ent: client})
	payload := []byte(`{"id":"evt_other_refund","type":"refund.updated","data":{"object":{"id":"re_another_product","payment_intent":"pi_another_product","amount":2500,"currency":"usd","status":"succeeded","livemode":false}}}`)
	response := postSignedStripeWebhook(t, router, secret, payload)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"ignored":true`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if client.PaymentEvent.Query().CountX(ctx) != 0 {
		t.Fatal("another product refund event was recorded")
	}
	chargeRefundPayload := []byte(`{"id":"evt_other_charge_refund","type":"charge.refund.updated","data":{"object":{"id":"re_charge_only","amount":2500,"currency":"usd","status":"succeeded","livemode":false}}}`)
	chargeRefundResponse := postSignedStripeWebhook(t, router, secret, chargeRefundPayload)
	if chargeRefundResponse.Code != http.StatusOK || !strings.Contains(chargeRefundResponse.Body.String(), `"ignored":true`) {
		t.Fatalf("status=%d body=%s", chargeRefundResponse.Code, chargeRefundResponse.Body.String())
	}
	if client.PaymentEvent.Query().CountX(ctx) != 0 {
		t.Fatal("another product charge-only refund event was recorded")
	}
}

func TestStripeWebhookIgnoresUnsupportedEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := enttest.Open(t, dialect.SQLite, "file:stripe-unsupported?mode=memory&cache=shared&_fk=1")
	defer client.Close()
	ctx := context.Background()
	secret := "whsec_unsupported"
	router := New(Dependencies{Config: config.Config{GinMode: gin.TestMode, Environment: "test", CORSAllowedOrigins: []string{"https://6url.net"}, StripeWebhookSecret: secret}, Ent: client})
	payload := []byte(`{"id":"evt_customer_created","type":"customer.created","data":{"object":{"id":"cus_another_product"}}}`)
	response := postSignedStripeWebhook(t, router, secret, payload)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"ignored":true`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if client.PaymentEvent.Query().CountX(ctx) != 0 {
		t.Fatal("unsupported Stripe event was recorded")
	}
}

func postSignedStripeWebhook(t *testing.T, router http.Handler, secret string, payload []byte) *httptest.ResponseRecorder {
	t.Helper()
	timestamp := time.Now().Unix()
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = fmt.Fprintf(mac, "%d.", timestamp)
	_, _ = mac.Write(payload)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/stripe", bytes.NewReader(payload))
	request.Header.Set("Stripe-Signature", fmt.Sprintf("t=%d,v1=%s", timestamp, hex.EncodeToString(mac.Sum(nil))))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
