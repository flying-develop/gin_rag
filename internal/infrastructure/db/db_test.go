package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/flying-develop/ai-app-go/internal/infrastructure/config"
	"github.com/flying-develop/ai-app-go/internal/infrastructure/db"
	"github.com/flying-develop/ai-app-go/internal/infrastructure/logging"
)

// openTestDB поднимает подключение к реальному Postgres из docker-compose.
// DATABASE_URL берётся из окружения (сервис tests передаёт его из .env).
func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	logging.Setup("DEBUG")

	cfg, err := config.Load()
	require.NoError(t, err)

	gormDB, err := db.Open(context.Background(), cfg, nil)
	require.NoError(t, err, "нужен запущенный postgres: docker compose up -d postgres")

	t.Cleanup(func() { _ = db.Close(gormDB) })
	return gormDB
}

func TestOpen_PingAndPool(t *testing.T) {
	gormDB := openTestDB(t)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.PingContext(context.Background()))
	require.Equal(t, 10, sqlDB.Stats().MaxOpenConnections)
}

func TestWithinTx_CommitsOnSuccess(t *testing.T) {
	gormDB := openTestDB(t)
	ctx := context.Background()
	setupTxTable(t, gormDB)

	err := db.WithinTx(ctx, gormDB, func(txCtx context.Context) error {
		return db.Conn(txCtx, gormDB).Exec("INSERT INTO tx_test (marker) VALUES (?)", "commit").Error
	})
	require.NoError(t, err)

	require.Equal(t, int64(1), countMarker(t, gormDB, "commit"))
}

func TestWithinTx_RollsBackOnError(t *testing.T) {
	gormDB := openTestDB(t)
	ctx := context.Background()
	setupTxTable(t, gormDB)

	sentinel := errors.New("boom")
	err := db.WithinTx(ctx, gormDB, func(txCtx context.Context) error {
		if e := db.Conn(txCtx, gormDB).Exec("INSERT INTO tx_test (marker) VALUES (?)", "rollback").Error; e != nil {
			return e
		}
		return sentinel
	})
	require.ErrorIs(t, err, sentinel)

	require.Equal(t, int64(0), countMarker(t, gormDB, "rollback"))
}

func setupTxTable(t *testing.T, gormDB *gorm.DB) {
	t.Helper()
	require.NoError(t, gormDB.Exec(`CREATE TABLE IF NOT EXISTS tx_test (marker text)`).Error)
	require.NoError(t, gormDB.Exec(`TRUNCATE tx_test`).Error)
	t.Cleanup(func() { _ = gormDB.Exec(`DROP TABLE IF EXISTS tx_test`).Error })
}

func countMarker(t *testing.T, gormDB *gorm.DB, marker string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, gormDB.Raw(`SELECT count(*) FROM tx_test WHERE marker = ?`, marker).Scan(&n).Error)
	return n
}
