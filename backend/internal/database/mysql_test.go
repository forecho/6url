package database

import (
	"context"
	"os"
	"testing"

	_ "github.com/go-sql-driver/mysql"

	"github.com/forecho/6url/backend/ent"
	"github.com/forecho/6url/backend/internal/config"
)

func TestMySQLSchemaMigrationSmoke(t *testing.T) {
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN is not configured")
	}
	client, err := ent.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := MigrateAndSeed(context.Background(), client, config.Config{StripeAdDays: 30}); err != nil {
		t.Fatal(err)
	}
	if got := client.Locale.Query().CountX(context.Background()); got != 4 {
		t.Fatalf("locale count = %d", got)
	}
}
