package db

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres" // регистрирует драйвер "postgres" через init()
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/flying-develop/ai-app-go/internal/infrastructure/config"
	migrationfiles "github.com/flying-develop/ai-app-go/migrations"
)

// DirectionUp и DirectionDown — допустимые направления миграции для Migrate.
const (
	DirectionUp   = "up"
	DirectionDown = "down"
)

// Migrate применяет (up) или откатывает (down) миграции БД.
//
// Использует SQL-файлы, встроенные в бинарь (пакет migrations), и
// подключается к БД по cfg.DatabaseURL. Отсутствие изменений
// (migrate.ErrNoChange) ошибкой не считается.
func Migrate(cfg *config.Config, direction string) error {
	logger := slog.Default().With(slog.String("component", "db.migrate"))

	source, err := iofs.New(migrationfiles.FS, ".")
	if err != nil {
		return fmt.Errorf("db: источник миграций: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", source, migrateDatabaseURL(cfg.DatabaseURL))
	if err != nil {
		return fmt.Errorf("db: инициализация migrate: %w", err)
	}
	defer func() { _, _ = m.Close() }()

	switch direction {
	case DirectionUp:
		err = m.Up()
	case DirectionDown:
		err = m.Down()
	default:
		return fmt.Errorf("db: неизвестное направление миграции %q", direction)
	}

	if errors.Is(err, migrate.ErrNoChange) {
		logger.Info("no migration changes", slog.String("direction", direction))
		return nil
	}
	if err != nil {
		return fmt.Errorf("db: миграция %s: %w", direction, err)
	}

	if direction == DirectionUp {
		logger.Info("migrations applied")
	} else {
		logger.Info("migrations reverted")
	}
	return nil
}

// migrateDatabaseURL приводит строку подключения к схеме, которую
// понимает golang-migrate (`postgres://`). GORM-драйвер принимает и
// `postgres://`, и `postgresql://`, а migrate — только `postgres://`.
func migrateDatabaseURL(url string) string {
	const alt = "postgresql://"
	if strings.HasPrefix(url, alt) {
		return "postgres://" + strings.TrimPrefix(url, alt)
	}
	return url
}
