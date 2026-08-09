package turnstile

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const verifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

type Verifier interface {
	Verify(context.Context, string, string) error
}

type Client struct {
	SecretKey string
	HTTP      *http.Client
}

func (c Client) Verify(ctx context.Context, response, remoteIP string) error {
	if c.SecretKey == "" || strings.TrimSpace(response) == "" {
		return errors.New("turnstile verification failed")
	}
	form := url.Values{"secret": {c.SecretKey}, "response": {strings.TrimSpace(response)}}
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, verifyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return errors.New("turnstile verification unavailable")
	}
	defer res.Body.Close()
	var result struct {
		Success bool `json:"success"`
	}
	if res.StatusCode != http.StatusOK || json.NewDecoder(res.Body).Decode(&result) != nil || !result.Success {
		return errors.New("turnstile verification failed")
	}
	return nil
}
