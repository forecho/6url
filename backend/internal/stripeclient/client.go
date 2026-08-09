package stripeclient

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	SecretKey string
	BaseURL   string
	HTTP      *http.Client
}

type CheckoutRequest struct {
	OrderID       int
	PriceID       string
	CustomerEmail string
	SuccessURL    string
	CancelURL     string
}

type CheckoutSession struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

type RefundRequest struct {
	OrderID         int
	PaymentIntentID string
	AmountMinor     int64
	IdempotencyKey  string
}

type Refund struct {
	ID              string `json:"id"`
	PaymentIntentID string `json:"payment_intent"`
	Amount          int64  `json:"amount"`
	Currency        string `json:"currency"`
	Status          string `json:"status"`
}

func (c Client) CreateCheckout(ctx context.Context, input CheckoutRequest) (CheckoutSession, error) {
	if c.SecretKey == "" || input.OrderID <= 0 || input.PriceID == "" {
		return CheckoutSession{}, errors.New("Stripe checkout configuration is incomplete")
	}
	baseURL := strings.TrimRight(c.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.stripe.com"
	}
	form := url.Values{
		"mode":                                    {"payment"},
		"line_items[0][price]":                    {input.PriceID},
		"line_items[0][quantity]":                 {"1"},
		"client_reference_id":                     {strconv.Itoa(input.OrderID)},
		"metadata[order_id]":                      {strconv.Itoa(input.OrderID)},
		"payment_intent_data[metadata][order_id]": {strconv.Itoa(input.OrderID)},
		"customer_email":                          {input.CustomerEmail},
		"success_url":                             {input.SuccessURL},
		"cancel_url":                              {input.CancelURL},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/checkout/sessions", strings.NewReader(form.Encode()))
	if err != nil {
		return CheckoutSession{}, err
	}
	req.SetBasicAuth(c.SecretKey, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Idempotency-Key", fmt.Sprintf("6url-order-%d", input.OrderID))
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	response, err := httpClient.Do(req)
	if err != nil {
		return CheckoutSession{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	if err != nil {
		return CheckoutSession{}, err
	}
	if response.StatusCode >= http.StatusBadRequest {
		return CheckoutSession{}, fmt.Errorf("Stripe API returned %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var session CheckoutSession
	if err := json.Unmarshal(body, &session); err != nil || session.ID == "" || session.URL == "" {
		return CheckoutSession{}, errors.New("Stripe returned an invalid checkout session")
	}
	return session, nil
}

func (c Client) Refund(ctx context.Context, input RefundRequest) (Refund, error) {
	if c.SecretKey == "" || input.OrderID <= 0 || input.PaymentIntentID == "" || input.AmountMinor <= 0 || input.IdempotencyKey == "" {
		return Refund{}, errors.New("Stripe refund configuration is incomplete")
	}
	baseURL := strings.TrimRight(c.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.stripe.com"
	}
	form := url.Values{
		"payment_intent":         {input.PaymentIntentID},
		"amount":                 {strconv.FormatInt(input.AmountMinor, 10)},
		"metadata[order_id]":     {strconv.Itoa(input.OrderID)},
		"metadata[refund_scope]": {"full"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/refunds", strings.NewReader(form.Encode()))
	if err != nil {
		return Refund{}, err
	}
	req.SetBasicAuth(c.SecretKey, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Idempotency-Key", input.IdempotencyKey)
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	response, err := httpClient.Do(req)
	if err != nil {
		return Refund{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	if err != nil {
		return Refund{}, err
	}
	if response.StatusCode >= http.StatusBadRequest {
		return Refund{}, fmt.Errorf("Stripe API returned %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var refund Refund
	if err := json.Unmarshal(body, &refund); err != nil || refund.ID == "" || refund.PaymentIntentID != input.PaymentIntentID || refund.Amount != input.AmountMinor || refund.Currency == "" || refund.Status == "" {
		return Refund{}, errors.New("Stripe returned an invalid refund")
	}
	return refund, nil
}

func VerifyWebhook(payload []byte, signatureHeader, secret string, now time.Time) error {
	if secret == "" {
		return errors.New("Stripe webhook secret is missing")
	}
	var timestamp int64
	var signatures []string
	for _, part := range strings.Split(signatureHeader, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch key {
		case "t":
			timestamp, _ = strconv.ParseInt(value, 10, 64)
		case "v1":
			signatures = append(signatures, value)
		}
	}
	if timestamp == 0 || len(signatures) == 0 || now.Sub(time.Unix(timestamp, 0)).Abs() > 5*time.Minute {
		return errors.New("invalid Stripe webhook timestamp")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = fmt.Fprintf(mac, "%d.", timestamp)
	_, _ = mac.Write(payload)
	expected := mac.Sum(nil)
	for _, signature := range signatures {
		decoded, err := hex.DecodeString(signature)
		if err == nil && hmac.Equal(expected, decoded) {
			return nil
		}
	}
	return errors.New("invalid Stripe webhook signature")
}
