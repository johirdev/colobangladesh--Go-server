// Package database connects to PostgreSQL and runs versioned SQL migrations.
package database

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"time"

	"cholobangladesh-server/internal/config"

	"github.com/pressly/goose/v3"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

//go:embed migrations/*.sql
var migrations embed.FS

func Connect(cfg config.DBConfig, production bool) (*gorm.DB, error) {
	level := logger.Warn
	if production {
		level = logger.Error
	}
	db, err := gorm.Open(postgres.Open(cfg.URL), &gorm.Config{
		Logger:                 logger.Default.LogMode(level),
		TranslateError:         true, // unique violations -> gorm.ErrDuplicatedKey
		SkipDefaultTransaction: true,
		NowFunc:                func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	slog.Info("postgres connected")
	return db, nil
}

// Migrate applies all pending migrations in migrations/ (embedded in the binary).
func Migrate(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	goose.SetBaseFS(migrations)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	if err := goose.Up(sqlDB, "migrations"); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	v, _ := goose.GetDBVersion(sqlDB)
	slog.Info("migrations applied", "version", v)
	return nil
}
