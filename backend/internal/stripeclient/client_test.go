package stripeclient

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestRefundSendsExplicitFullAmountAndIdempotencyKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/refunds" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		form, _ := url.ParseQuery(string(body))
		if form.Get("payment_intent") != "pi_123" || form.Get("amount") != "2500" || form.Get("metadata[order_id]") != "42" || r.Header.Get("Idempotency-Key") != "refund-key-123" {
			t.Fatalf("unexpected refund request: form=%v key=%q", form, r.Header.Get("Idempotency-Key"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"re_123","payment_intent":"pi_123","amount":2500,"currency":"usd","status":"succeeded"}`))
	}))
	defer server.Close()
	refund, err := (Client{SecretKey: "sk_test", BaseURL: server.URL, HTTP: server.Client()}).Refund(context.Background(), RefundRequest{OrderID: 42, PaymentIntentID: "pi_123", AmountMinor: 2500, IdempotencyKey: "refund-key-123"})
	if err != nil {
		t.Fatal(err)
	}
	if refund.ID != "re_123" || refund.Status != "succeeded" {
		t.Fatalf("unexpected refund: %#v", refund)
	}
}

func TestVerifyWebhook(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	payload := []byte(`{"id":"evt_1"}`)
	mac := hmac.New(sha256.New, []byte("whsec_test"))
	_, _ = fmt.Fprintf(mac, "%d.", now.Unix())
	_, _ = mac.Write(payload)
	header := fmt.Sprintf("t=%d,v1=%s", now.Unix(), hex.EncodeToString(mac.Sum(nil)))
	if err := VerifyWebhook(payload, header, "whsec_test", now); err != nil {
		t.Fatal(err)
	}
	if err := VerifyWebhook(payload, header, "wrong", now); err == nil {
		t.Fatal("wrong secret should fail")
	}
}
