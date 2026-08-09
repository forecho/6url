package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	HTTPPort           string
	GinMode            string
	Environment        string
	AppBaseURL         string
	APIBaseURL         string
	CORSAllowedOrigins []string

	DBHost         string
	DBPort         string
	DBUser         string
	DBPassword     string
	DBName         string
	DBMaxOpenConns int
	DBMaxIdleConns int
	RedisAddr      string
	RedisPassword  string
	RedisDB        int
	RedisKeyPrefix string

	AdminTokenPepper       string
	ApplicationTokenPepper string
	TrustedProxyCIDRs      []string
	TurnstileSecretKey     string

	StripeSecretKey      string
	StripeWebhookSecret  string
	StripeListingPriceID string
	StripeAdPriceID      string
	StripeListingAmount  int
	StripeListingDays    int
	StripeAdAmount       int
	StripeAdDays         int

	EmailProvider   string
	EmailAPIKey     string
	EmailFrom       string
	EmailAPIBaseURL string

	AdminNotifyChannels []string
	SlackBotToken       string
	SlackChannelID      string

	ImgAntAllowedHosts        []string
	ImgAntAlbum               string
	NotificationEncryptionKey string
	RevalidationWebhookURL    string
	RevalidationSecret        string
}

func Load() Config {
	_ = godotenv.Load()
	return Config{
		HTTPPort:                  env("HTTP_PORT", "8087"),
		GinMode:                   env("GIN_MODE", "debug"),
		Environment:               env("ENVIRONMENT", "development"),
		AppBaseURL:                env("APP_BASE_URL", "http://localhost:3000"),
		APIBaseURL:                env("API_BASE_URL", "http://localhost:8087"),
		CORSAllowedOrigins:        csvEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000"),
		DBHost:                    env("DB_HOST", "127.0.0.1"),
		DBPort:                    env("DB_PORT", "3306"),
		DBUser:                    env("DB_USER", "root"),
		DBPassword:                env("DB_PASSWORD", ""),
		DBName:                    env("DB_NAME", "6url"),
		DBMaxOpenConns:            intEnv("DB_MAX_OPEN_CONNS", 25),
		DBMaxIdleConns:            intEnv("DB_MAX_IDLE_CONNS", 5),
		RedisAddr:                 env("REDIS_ADDR", "127.0.0.1:6379"),
		RedisPassword:             env("REDIS_PASSWORD", ""),
		RedisDB:                   intEnv("REDIS_DB", 0),
		RedisKeyPrefix:            env("REDIS_KEY_PREFIX", "6url:"),
		AdminTokenPepper:          env("ADMIN_TOKEN_PEPPER", ""),
		ApplicationTokenPepper:    env("APPLICATION_TOKEN_PEPPER", ""),
		TrustedProxyCIDRs:         csvEnv("TRUSTED_PROXY_CIDRS", "127.0.0.1,::1"),
		TurnstileSecretKey:        env("TURNSTILE_SECRET_KEY", ""),
		StripeSecretKey:           env("STRIPE_SECRET_KEY", ""),
		StripeWebhookSecret:       env("STRIPE_WEBHOOK_SECRET", ""),
		StripeListingPriceID:      env("STRIPE_LISTING_PRICE_ID", ""),
		StripeAdPriceID:           env("STRIPE_AD_PRICE_ID", ""),
		StripeListingAmount:       intEnv("STRIPE_LISTING_AMOUNT_MINOR", 0),
		StripeListingDays:         intEnv("STRIPE_LISTING_DURATION_DAYS", 0),
		StripeAdAmount:            intEnv("STRIPE_AD_AMOUNT_MINOR", 0),
		StripeAdDays:              intEnv("STRIPE_AD_DURATION_DAYS", 0),
		EmailProvider:             strings.ToLower(env("EMAIL_PROVIDER", "unosend")),
		EmailAPIKey:               env("EMAIL_API_KEY", ""),
		EmailFrom:                 env("EMAIL_FROM_ADDRESS", ""),
		EmailAPIBaseURL:           env("EMAIL_API_BASE_URL", ""),
		AdminNotifyChannels:       csvEnv("ADMIN_NOTIFY_CHANNELS", "slack"),
		SlackBotToken:             env("SLACK_BOT_TOKEN", ""),
		SlackChannelID:            env("SLACK_CHANNEL_ID", ""),
		ImgAntAllowedHosts:        csvEnv("IMGANT_ALLOWED_HOSTS", "cdn.imgant.com"),
		ImgAntAlbum:               env("IMGANT_ALBUM", "6url"),
		NotificationEncryptionKey: env("NOTIFICATION_ENCRYPTION_KEY", ""),
		RevalidationWebhookURL:    env("REVALIDATION_WEBHOOK_URL", ""),
		RevalidationSecret:        env("REVALIDATION_SECRET", ""),
	}
}

func (c Config) Validate() error {
	if c.DBHost == "" || c.DBPort == "" || c.DBUser == "" || c.DBName == "" {
		return errors.New("database configuration is incomplete")
	}
	if len(c.AdminTokenPepper) < 32 {
		return errors.New("ADMIN_TOKEN_PEPPER must contain at least 32 characters")
	}
	if c.DBMaxOpenConns <= 0 || c.DBMaxIdleConns < 0 || c.DBMaxIdleConns > c.DBMaxOpenConns {
		return errors.New("database connection pool configuration is invalid")
	}
	if c.RedisDB < 0 || c.RedisKeyPrefix == "" {
		return errors.New("Redis configuration is invalid")
	}
	for _, value := range c.TrustedProxyCIDRs {
		if net.ParseIP(value) == nil {
			if _, _, err := net.ParseCIDR(value); err != nil {
				return fmt.Errorf("invalid TRUSTED_PROXY_CIDRS entry: %s", value)
			}
		}
	}
	if c.AppBaseURL == "" || c.APIBaseURL == "" {
		return errors.New("APP_BASE_URL and API_BASE_URL are required")
	}
	if c.ImgAntAlbum != "6url" {
		return errors.New("IMGANT_ALBUM must be 6url")
	}
	if len(c.ImgAntAllowedHosts) == 0 {
		return errors.New("IMGANT_ALLOWED_HOSTS must contain at least one host")
	}
	if c.Environment == "production" {
		if len(c.ApplicationTokenPepper) < 32 {
			return errors.New("APPLICATION_TOKEN_PEPPER must contain at least 32 characters in production")
		}
		key, err := base64.StdEncoding.DecodeString(c.NotificationEncryptionKey)
		if err != nil || len(key) != 32 {
			return errors.New("NOTIFICATION_ENCRYPTION_KEY must be base64-encoded 32 bytes in production")
		}
		if _, _, err := net.SplitHostPort(c.RedisAddr); err != nil || c.TurnstileSecretKey == "" {
			return errors.New("REDIS_ADDR and TURNSTILE_SECRET_KEY are required in production")
		}
		if len(c.TrustedProxyCIDRs) == 0 {
			return errors.New("TRUSTED_PROXY_CIDRS must contain the production reverse proxies")
		}
		if c.AppBaseURL != "https://6url.net" {
			return errors.New("production APP_BASE_URL must be https://6url.net")
		}
		if c.APIBaseURL != "https://api.6url.net" {
			return errors.New("production API_BASE_URL must be https://api.6url.net")
		}
		if len(c.CORSAllowedOrigins) != 1 || c.CORSAllowedOrigins[0] != "https://6url.net" {
			return errors.New("production CORS_ALLOWED_ORIGINS must be exactly https://6url.net")
		}
		if !contains(c.AdminNotifyChannels, "slack") || c.SlackBotToken == "" || c.SlackChannelID == "" {
			return errors.New("Slack notification configuration is incomplete")
		}
		address, emailErr := mail.ParseAddress(c.EmailFrom)
		if c.EmailAPIKey == "" || emailErr != nil || address.Address != c.EmailFrom || (c.EmailProvider != "unosend" && c.EmailProvider != "sendflare") {
			return errors.New("email notification configuration is incomplete")
		}
		if c.StripeSecretKey == "" || c.StripeWebhookSecret == "" || c.StripeListingPriceID == "" || c.StripeAdPriceID == "" {
			return errors.New("Stripe configuration is incomplete")
		}
		if c.StripeListingAmount <= 0 || c.StripeAdAmount <= 0 || c.StripeAdDays <= 0 || c.StripeListingDays < 0 {
			return errors.New("Stripe product snapshots are invalid")
		}
		if c.RevalidationWebhookURL != "https://6url.net/api/internal/revalidate" || len(c.RevalidationSecret) < 32 {
			return errors.New("frontend revalidation configuration is incomplete")
		}
	}
	return nil
}

func (c Config) MySQLDSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&loc=UTC", c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
}

func env(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return strings.TrimSpace(value)
	}
	return fallback
}

func intEnv(key string, fallback int) int {
	value := env(key, "")
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func csvEnv(key, fallback string) []string {
	parts := strings.Split(env(key, fallback), ",")
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}
