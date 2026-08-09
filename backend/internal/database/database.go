package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/go-sql-driver/mysql"

	"github.com/forecho/6url/backend/ent"
	"github.com/forecho/6url/backend/ent/adoffer"
	"github.com/forecho/6url/backend/ent/listingplan"
	"github.com/forecho/6url/backend/ent/locale"
	"github.com/forecho/6url/backend/internal/config"
)

func Open(cfg config.Config) (*ent.Client, *sql.DB, error) {
	sqlDB, err := sql.Open("mysql", cfg.MySQLDSN())
	if err != nil {
		return nil, nil, fmt.Errorf("open mysql: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.DBMaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.DBMaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Hour)
	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, nil, fmt.Errorf("ping mysql: %w", err)
	}
	driver := entsql.OpenDB("mysql", sqlDB)
	return ent.NewClient(ent.Driver(driver)), sqlDB, nil
}

func MigrateAndSeed(ctx context.Context, client *ent.Client, cfg config.Config) error {
	if err := client.Schema.Create(ctx); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	locales := []struct {
		code, name, nativeName string
		isDefault              bool
		sortOrder              int
	}{
		{"zh-CN", "Chinese (Simplified)", "简体中文", true, 10},
		{"en", "English", "English", false, 20},
		{"ja", "Japanese", "日本語", false, 30},
		{"ko", "Korean", "한국어", false, 40},
	}
	for _, item := range locales {
		exists, err := client.Locale.Query().Where(locale.CodeEQ(item.code)).Exist(ctx)
		if err != nil {
			return fmt.Errorf("query locale %s: %w", item.code, err)
		}
		if exists {
			continue
		}
		if _, err := client.Locale.Create().SetCode(item.code).SetName(item.name).SetNativeName(item.nativeName).SetIsDefault(item.isDefault).SetSortOrder(item.sortOrder).Save(ctx); err != nil {
			return fmt.Errorf("seed locale %s: %w", item.code, err)
		}
	}
	if err := seedListingPlan(ctx, client, cfg); err != nil {
		return err
	}
	if err := seedAdOffer(ctx, client, cfg); err != nil {
		return err
	}
	return nil
}

func seedListingPlan(ctx context.Context, client *ent.Client, cfg config.Config) error {
	const code = "listing-standard"
	if row, err := client.ListingPlan.Query().Where(listingplan.CodeEQ(code)).Only(ctx); err == nil {
		updater := row.Update().SetName("Standard paid listing").SetAmountMinor(int64(cfg.StripeListingAmount)).SetCurrency("usd").SetIncludedLocales([]string{"zh-CN", "en", "ja", "ko"}).SetActive(cfg.StripeListingAmount > 0 && cfg.StripeListingPriceID != "")
		if cfg.StripeListingDays > 0 {
			updater.SetDurationDays(cfg.StripeListingDays)
		} else {
			updater.ClearDurationDays()
		}
		if cfg.StripeListingPriceID != "" {
			updater.SetStripePriceID(cfg.StripeListingPriceID)
		} else {
			updater.ClearStripePriceID()
		}
		if _, err := updater.Save(ctx); err != nil {
			return fmt.Errorf("update listing plan: %w", err)
		}
		return nil
	} else if !ent.IsNotFound(err) {
		return fmt.Errorf("query listing plan: %w", err)
	}
	creator := client.ListingPlan.Create().SetCode(code).SetName("Standard paid listing").SetAmountMinor(int64(cfg.StripeListingAmount)).SetCurrency("usd").SetIncludedLocales([]string{"zh-CN", "en", "ja", "ko"}).SetActive(cfg.StripeListingAmount > 0 && cfg.StripeListingPriceID != "")
	if cfg.StripeListingDays > 0 {
		creator.SetDurationDays(cfg.StripeListingDays)
	}
	if cfg.StripeListingPriceID != "" {
		creator.SetStripePriceID(cfg.StripeListingPriceID)
	}
	if _, err := creator.Save(ctx); err != nil {
		return fmt.Errorf("seed listing plan: %w", err)
	}
	return nil
}

func seedAdOffer(ctx context.Context, client *ent.Client, cfg config.Config) error {
	const code = "sponsor-standard"
	if row, err := client.AdOffer.Query().Where(adoffer.CodeEQ(code)).Only(ctx); err == nil {
		updater := row.Update().SetName("Standard sponsor ad").SetAmountMinor(int64(cfg.StripeAdAmount)).SetCurrency("usd").SetDurationDays(max(cfg.StripeAdDays, 1)).SetIncludedLocales([]string{"zh-CN", "en", "ja", "ko"}).SetActive(cfg.StripeAdAmount > 0 && cfg.StripeAdPriceID != "")
		if cfg.StripeAdPriceID != "" {
			updater.SetStripePriceID(cfg.StripeAdPriceID)
		} else {
			updater.ClearStripePriceID()
		}
		if _, err := updater.Save(ctx); err != nil {
			return fmt.Errorf("update ad offer: %w", err)
		}
		return nil
	} else if !ent.IsNotFound(err) {
		return fmt.Errorf("query ad offer: %w", err)
	}
	creator := client.AdOffer.Create().SetCode(code).SetName("Standard sponsor ad").SetAmountMinor(int64(cfg.StripeAdAmount)).SetCurrency("usd").SetDurationDays(max(cfg.StripeAdDays, 1)).SetIncludedLocales([]string{"zh-CN", "en", "ja", "ko"}).SetActive(cfg.StripeAdAmount > 0 && cfg.StripeAdPriceID != "")
	if cfg.StripeAdPriceID != "" {
		creator.SetStripePriceID(cfg.StripeAdPriceID)
	}
	if _, err := creator.Save(ctx); err != nil {
		return fmt.Errorf("seed ad offer: %w", err)
	}
	return nil
}
