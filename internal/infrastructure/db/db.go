// Package db инкапсулирует подключение к PostgreSQL через GORM:
// создание engine, настройку пула соединений, адаптер логирования на
// log/slog и хелпер транзакций.
//
// Прикладной код обращается к БД только через репозитории модулей
// (см. .ai-factory/rules/base.md) — этот пакет лишь предоставляет
// готовый *gorm.DB.
package db

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/flying-develop/ai-app-go/internal/infrastructure/config"
)

// Параметры пула соединений. Значения консервативные — уточняются под
// нагрузку на поздних вехах.
const (
	maxOpenConns    = 10
	maxIdleConns    = 5
	connMaxLifetime = time.Hour
)

// Open открывает подключение к БД, настраивает пул и проверяет
// доступность (ping). Возвращает готовый *gorm.DB.
func Open(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*gorm.DB, error) {
	if logger == nil {
		logger = slog.Default()
	}

	gormDB, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{
		Logger:                 newSlogLogger(logger),
		SkipDefaultTransaction: true,
	})
	if err != nil {
		return nil, fmt.Errorf("db: открытие подключения: %w", err)
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("db: доступ к *sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetMaxIdleConns(maxIdleConns)
	sqlDB.SetConnMaxLifetime(connMaxLifetime)

	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("db: ping: %w", err)
	}

	logger.Info("database connection established",
		slog.Int("max_open_conns", maxOpenConns),
		slog.Int("max_idle_conns", maxIdleConns),
		slog.Duration("conn_max_lifetime", connMaxLifetime),
	)

	return gormDB, nil
}

// Close закрывает пул соединений. Безопасно вызывать через defer.
func Close(gormDB *gorm.DB) error {
	if gormDB == nil {
		return nil
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
