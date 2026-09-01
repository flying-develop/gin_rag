package db

import (
	"context"
	"fmt"
	"log/slog"

	"gorm.io/gorm"
)

// txContextKey — ключ для хранения активной транзакции в context.Context.
type txContextKey struct{}

// WithinTx выполняет fn внутри одной транзакции БД.
//
// Транзакция коммитится, если fn вернула nil, и откатывается при любой
// ошибке или панике внутри fn. В ctx, переданный в fn, кладётся активная
// транзакция — репозитории достают её через Conn(ctx), поэтому все
// операции внутри fn автоматически идут в этой транзакции.
//
// Инициировать транзакцию положено в сервисном слое.
func WithinTx(ctx context.Context, gormDB *gorm.DB, fn func(ctx context.Context) error) error {
	log := slog.Default().With(slog.String("component", "db.tx"))
	log.DebugContext(ctx, "tx begin")

	err := gormDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(context.WithValue(ctx, txContextKey{}, tx))
	})
	if err != nil {
		log.DebugContext(ctx, "tx rollback", slog.String("error_type", fmt.Sprintf("%T", err)))
		return err
	}

	log.DebugContext(ctx, "tx commit")
	return nil
}

// Conn возвращает активную транзакцию из ctx, если она есть, иначе
// переданное подключение fallback. Репозитории используют его вместо
// прямого обращения к своему *gorm.DB.
func Conn(ctx context.Context, fallback *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(txContextKey{}).(*gorm.DB); ok && tx != nil {
		return tx
	}
	return fallback
}
