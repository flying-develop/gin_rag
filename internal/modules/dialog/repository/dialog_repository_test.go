package repository_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/flying-develop/ai-app-go/internal/infrastructure/config"
	"github.com/flying-develop/ai-app-go/internal/infrastructure/db"
	"github.com/flying-develop/ai-app-go/internal/infrastructure/logging"
	"github.com/flying-develop/ai-app-go/internal/modules/dialog/model"
	"github.com/flying-develop/ai-app-go/internal/modules/dialog/repository"
)

func newRepo(t *testing.T) (*repository.DialogRepository, *gorm.DB) {
	t.Helper()
	logging.Setup("DEBUG")

	cfg, err := config.Load()
	require.NoError(t, err)
	require.NoError(t, db.Migrate(cfg, db.DirectionUp), "нужен запущенный postgres")

	gormDB, err := db.Open(context.Background(), cfg, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close(gormDB) })

	require.NoError(t, gormDB.Exec("TRUNCATE dialogs RESTART IDENTITY").Error)
	return repository.NewDialogRepository(gormDB), gormDB
}

func TestDialogRepository_CreateAndFindByID(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	d := &model.Dialog{UserID: 1, Title: "hello"}
	require.NoError(t, repo.Create(ctx, d))
	require.NotZero(t, d.ID)

	got, err := repo.FindByID(ctx, d.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "hello", got.Title)
	require.Equal(t, uint(1), got.UserID)
}

func TestDialogRepository_FindByID_NotFound(t *testing.T) {
	repo, _ := newRepo(t)

	got, err := repo.FindByID(context.Background(), 999)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestDialogRepository_List_FiltersAndOrders(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, &model.Dialog{UserID: 1, Title: "first"}))
	require.NoError(t, repo.Create(ctx, &model.Dialog{UserID: 1, Title: "second"}))
	require.NoError(t, repo.Create(ctx, &model.Dialog{UserID: 2, Title: "other user"}))

	list, err := repo.List(ctx, 1, 0, 0)
	require.NoError(t, err)
	require.Len(t, list, 2)
	require.Equal(t, "second", list[0].Title, "новые первыми (created_at DESC)")

	limited, err := repo.List(ctx, 1, 1, 0)
	require.NoError(t, err)
	require.Len(t, limited, 1)
}

func TestDialogRepository_Update(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	d := &model.Dialog{UserID: 1, Title: "old"}
	require.NoError(t, repo.Create(ctx, d))

	d.Title = "new"
	require.NoError(t, repo.Update(ctx, d))

	got, err := repo.FindByID(ctx, d.ID)
	require.NoError(t, err)
	require.Equal(t, "new", got.Title)
}

func TestDialogRepository_Delete(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	d := &model.Dialog{UserID: 1, Title: "to delete"}
	require.NoError(t, repo.Create(ctx, d))

	deleted, err := repo.Delete(ctx, d.ID)
	require.NoError(t, err)
	require.True(t, deleted)

	deletedAgain, err := repo.Delete(ctx, d.ID)
	require.NoError(t, err)
	require.False(t, deletedAgain)
}
