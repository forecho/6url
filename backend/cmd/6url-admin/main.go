package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/forecho/6url/backend/internal/admintoken"
	"github.com/forecho/6url/backend/internal/config"
	"github.com/forecho/6url/backend/internal/database"
	"github.com/forecho/6url/backend/internal/legacyimport"
)

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "migrate" {
		runMigrate()
		return
	}
	if len(os.Args) >= 3 && os.Args[1] == "token" && os.Args[2] == "create" {
		runTokenCreate(os.Args[3:])
		return
	}
	if len(os.Args) >= 2 && os.Args[1] == "import-json" {
		runImportJSON(os.Args[2:])
		return
	}
	log.Fatal("usage: 6url-admin migrate | 6url-admin token create ... | 6url-admin import-json --resources PATH")
}

func runMigrate() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}
	client, sqlDB, err := database.Open(cfg)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer client.Close()
	defer sqlDB.Close()
	if err := database.MigrateAndSeed(context.Background(), client, cfg); err != nil {
		log.Fatalf("database initialization failed: %v", err)
	}
	fmt.Println("database schema, locales, and commercial offers are ready")
}

func runTokenCreate(args []string) {
	flags := flag.NewFlagSet("token create", flag.ExitOnError)
	name := flags.String("name", "", "token name")
	scopeCSV := flags.String("scopes", "", "comma-separated scopes")
	_ = flags.Parse(args)
	scopes := parseScopes(*scopeCSV)
	if strings.TrimSpace(*name) == "" {
		log.Fatal("--name is required")
	}
	if err := admintoken.ValidateScopes(scopes); err != nil {
		log.Fatal(err)
	}

	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}
	client, sqlDB, err := database.Open(cfg)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer client.Close()
	defer sqlDB.Close()
	raw, prefix, err := admintoken.Generate()
	if err != nil {
		log.Fatalf("token generation failed: %v", err)
	}
	if _, err := client.AdminAPIToken.Create().SetName(strings.TrimSpace(*name)).SetPrefix(prefix).SetTokenHash(admintoken.Hash(raw, cfg.AdminTokenPepper)).SetScopes(scopes).Save(context.Background()); err != nil {
		log.Fatalf("token storage failed: %v", err)
	}
	fmt.Println(raw)
}

func runImportJSON(args []string) {
	flags := flag.NewFlagSet("import-json", flag.ExitOnError)
	resourcesPath := flags.String("resources", "../data/resources.json", "legacy resources.json path")
	_ = flags.Parse(args)
	resources, err := legacyimport.LoadResources(*resourcesPath)
	if err != nil {
		log.Fatal(err)
	}
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}
	client, sqlDB, err := database.Open(cfg)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer client.Close()
	defer sqlDB.Close()
	result, err := (legacyimport.Importer{Client: client}).Import(context.Background(), resources)
	if err != nil {
		log.Fatalf("legacy import failed: %v", err)
	}
	fmt.Printf("categories_created=%d sites_created=%d sites_reused=%d\n", result.CategoriesCreated, result.SitesCreated, result.SitesReused)
}

func parseScopes(value string) []string {
	seen := make(map[string]struct{})
	for _, item := range strings.Split(value, ",") {
		scope := strings.TrimSpace(item)
		if scope != "" {
			seen[scope] = struct{}{}
		}
	}
	result := make([]string, 0, len(seen))
	for scope := range seen {
		result = append(result, scope)
	}
	sort.Strings(result)
	return result
}
