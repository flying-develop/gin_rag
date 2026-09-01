package repository_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/flying-develop/ai-app-go/internal/modules/dialog/model"
)

func TestDialogRepository_AppendAndListMessages(t *testing.T) {
	repo, _ := newRepo(t)
	ctx := context.Background()

	d := &model.Dialog{UserID: 1, Title: "chat"}
	require.NoError(t, repo.Create(ctx, d))

	require.NoError(t, repo.AppendMessages(ctx,
		&model.DialogMessage{DialogID: d.ID, Role: model.RoleUser, Content: "привет"},
		&model.DialogMessage{DialogID: d.ID, Role: model.RoleAssistant, Content: "здравствуйте"},
	))

	msgs, err := repo.ListMessages(ctx, d.ID, 0)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	require.Equal(t, model.RoleUser, msgs[0].Role)
	require.Equal(t, "привет", msgs[0].Content)
	require.Equal(t, model.RoleAssistant, msgs[1].Role)
	require.Less(t, msgs[0].ID, msgs[1].ID, "порядок по возрастанию id")

	limited, err := repo.ListMessages(ctx, d.ID, 1)
	require.NoError(t, err)
	require.Len(t, limited, 1)
}

func TestDialogRepository_DeleteCascadesMessages(t *testing.T) {
	repo, gormDB := newRepo(t)
	ctx := context.Background()

	d := &model.Dialog{UserID: 1, Title: "chat"}
	require.NoError(t, repo.Create(ctx, d))
	require.NoError(t, repo.AppendMessages(ctx,
		&model.DialogMessage{DialogID: d.ID, Role: model.RoleUser, Content: "x"},
	))

	deleted, err := repo.Delete(ctx, d.ID)
	require.NoError(t, err)
	require.True(t, deleted)

	var count int64
	require.NoError(t, gormDB.Raw("SELECT count(*) FROM dialog_messages WHERE dialog_id = ?", d.ID).Scan(&count).Error)
	require.Equal(t, int64(0), count, "сообщения удаляются каскадом")
}
