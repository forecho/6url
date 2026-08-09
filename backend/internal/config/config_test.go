package config

import (
	"encoding/base64"
	"testing"
)

func TestValidateProductionOrigins(t *testing.T) {
	cfg := Config{
		Environment:            "production",
		AppBaseURL:             "https://6url.net",
		APIBaseURL:             "https://api.6url.net",
		DBHost:                 "127.0.0.1",
		DBPort:                 "3306",
		DBUser:                 "root",
		DBName:                 "6url",
		DBMaxOpenConns:         25,
		DBMaxIdleConns:         5,
		AdminTokenPepper:       "01234567890123456789012345678901",
		ApplicationTokenPepper: "abcdefghijklmnopqrstuvwxyz012345",
		ImgAntAlbum:            "6url",
		ImgAntAllowedHosts:     []string{"cdn.imgant.com"},
		CORSAllowedOrigins:     []string{"https://6url.net"},
		TrustedProxyCIDRs:      []string{"127.0.0.1"},
		RedisAddr:              "127.0.0.1:6379", RedisKeyPrefix: "6url:", TurnstileSecretKey: "turnstile",
		NotificationEncryptionKey: base64.StdEncoding.EncodeToString(make([]byte, 32)),
		EmailProvider:             "unosend", EmailAPIKey: "email-key", EmailFrom: "hello@6url.net",
		SlackBotToken: "slack", SlackChannelID: "channel", AdminNotifyChannels: []string{"slack"},
		StripeSecretKey: "stripe", StripeWebhookSecret: "webhook", StripeListingPriceID: "price_listing", StripeAdPriceID: "price_ad",
		StripeListingAmount: 1000, StripeAdAmount: 2000, StripeAdDays: 30,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsShortTokenPepper(t *testing.T) {
	cfg := Config{DBHost: "db", DBPort: "3306", DBUser: "root", DBName: "6url", DBMaxOpenConns: 25, DBMaxIdleConns: 5, AdminTokenPepper: "short", AppBaseURL: "http://localhost:3000", APIBaseURL: "http://localhost:8087", RedisKeyPrefix: "6url:", ImgAntAlbum: "6url", ImgAntAllowedHosts: []string{"cdn.imgant.com"}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected an error")
	}
}
