package config

import (
	"errors"
	"fmt"
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

	AdminTokenPepper string

	StripeSecretKey      string
	StripeWebhookSecret  string
	StripeListingPriceID string
	StripeAdPriceID      string

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
		AdminTokenPepper:          env("ADMIN_TOKEN_PEPPER", ""),
		StripeSecretKey:           env("STRIPE_SECRET_KEY", ""),
		StripeWebhookSecret:       env("STRIPE_WEBHOOK_SECRET", ""),
		StripeListingPriceID:      env("STRIPE_LISTING_PRICE_ID", ""),
		StripeAdPriceID:           env("STRIPE_AD_PRICE_ID", ""),
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
	}
}

func (c Config) Validate() error {
	if c.DBHost == "" || c.DBPort == "" || c.DBUser == "" || c.DBName == "" {
		return errors.New("database configuration is incomplete")
	}
	if len(c.AdminTokenPepper) < 32 {
		return errors.New("ADMIN_TOKEN_PEPPER must contain at least 32 characters")
	}
	if c.AppBaseURL == "" || c.APIBaseURL == "" {
		return errors.New("APP_BASE_URL and API_BASE_URL are required")
	}
	if c.ImgAntAlbum != "6url" {
		return errors.New("IMGANT_ALBUM must be 6url")
	}
	if c.Environment == "production" {
		if c.AppBaseURL != "https://6url.net" {
			return errors.New("production APP_BASE_URL must be https://6url.net")
		}
		if c.APIBaseURL != "https://api.6url.net" {
			return errors.New("production API_BASE_URL must be https://api.6url.net")
		}
		if len(c.CORSAllowedOrigins) != 1 || c.CORSAllowedOrigins[0] != "https://6url.net" {
			return errors.New("production CORS_ALLOWED_ORIGINS must be exactly https://6url.net")
		}
		if contains(c.AdminNotifyChannels, "slack") && (c.SlackBotToken == "" || c.SlackChannelID == "") {
			return errors.New("Slack notification configuration is incomplete")
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
