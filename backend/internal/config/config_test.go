package config

import "testing"

func TestValidateProductionOrigins(t *testing.T) {
	cfg := Config{
		Environment:        "production",
		AppBaseURL:         "https://6url.net",
		APIBaseURL:         "https://api.6url.net",
		DBHost:             "127.0.0.1",
		DBPort:             "3306",
		DBUser:             "root",
		DBName:             "6url",
		AdminTokenPepper:   "01234567890123456789012345678901",
		ImgAntAlbum:        "6url",
		CORSAllowedOrigins: []string{"https://6url.net"},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsShortTokenPepper(t *testing.T) {
	cfg := Config{DBHost: "db", DBPort: "3306", DBUser: "root", DBName: "6url", AdminTokenPepper: "short", AppBaseURL: "http://localhost:3000", APIBaseURL: "http://localhost:8087", ImgAntAlbum: "6url"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected an error")
	}
}
