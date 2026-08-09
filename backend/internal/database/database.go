package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/go-sql-driver/mysql"

	"github.com/forecho/6url/backend/ent"
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

func MigrateAndSeed(ctx context.Context, client *ent.Client) error {
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
	return nil
}
