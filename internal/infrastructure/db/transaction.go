package db

import (
	"context"
	"fmt"
	"log/slog"

	"gorm.io/gorm"
)

// WithinTx выполняет fn внутри одной транзакции БД.
//
// Транзакция коммитится, если fn вернула nil, и откатывается при любой
// ошибке или панике внутри fn. Инициировать транзакцию положено в
// сервисном слое; все операции внутри fn должны идти через репозитории.
func WithinTx(ctx context.Context, gormDB *gorm.DB, fn func(tx *gorm.DB) error) error {
	log := slog.Default().With(slog.String("component", "db.tx"))
	log.DebugContext(ctx, "tx begin")

	err := gormDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(tx)
	})
	if err != nil {
		log.DebugContext(ctx, "tx rollback", slog.String("error_type", fmt.Sprintf("%T", err)))
		return err
	}

	log.DebugContext(ctx, "tx commit")
	return nil
}
