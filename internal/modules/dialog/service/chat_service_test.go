package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/llms"
	"gorm.io/gorm"

	"github.com/flying-develop/ai-app-go/internal/apperr"
	"github.com/flying-develop/ai-app-go/internal/infrastructure/config"
	"github.com/flying-develop/ai-app-go/internal/infrastructure/db"
	"github.com/flying-develop/ai-app-go/internal/infrastructure/llm/llmtest"
	"github.com/flying-develop/ai-app-go/internal/infrastructure/logging"
	"github.com/flying-develop/ai-app-go/internal/modules/dialog/model"
	"github.com/flying-develop/ai-app-go/internal/modules/dialog/repository"
	"github.com/flying-develop/ai-app-go/internal/modules/dialog/service"
)

func newChat(t *testing.T, fake *llmtest.Fake) (*service.ChatService, *repository.DialogRepository, *gorm.DB) {
	t.Helper()
	logging.Setup("DEBUG")

	cfg, err := config.Load()
	require.NoError(t, err)
	require.NoError(t, db.Migrate(cfg, db.DirectionUp), "нужен запущенный postgres")

	gormDB, err := db.Open(context.Background(), cfg, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close(gormDB) })
	require.NoError(t, gormDB.Exec("TRUNCATE dialogs, dialog_messages RESTART IDENTITY").Error)

	repo := repository.NewDialogRepository(gormDB)
	return service.NewChatService(repo, gormDB, fake), repo, gormDB
}

func countMessages(t *testing.T, gormDB *gorm.DB, dialogID uint) int64 {
	t.Helper()
	var n int64
	require.NoError(t, gormDB.Raw("SELECT count(*) FROM dialog_messages WHERE dialog_id = ?", dialogID).Scan(&n).Error)
	return n
}

func TestChatService_SendMessage_PersistsBothMessages(t *testing.T) {
	fake := &llmtest.Fake{Answer: "ответ"}
	chat, repo, gormDB := newChat(t, fake)
	ctx := context.Background()

	d := &model.Dialog{UserID: 1, Title: "chat"}
	require.NoError(t, repo.Create(ctx, d))
	require.NoError(t, repo.AppendMessages(ctx,
		&model.DialogMessage{DialogID: d.ID, Role: model.RoleUser, Content: "старое сообщение"},
	))

	msg, err := chat.SendMessage(ctx, d.ID, "новое сообщение")
	require.NoError(t, err)
	require.Equal(t, model.RoleAssistant, msg.Role)
	require.Equal(t, "ответ", msg.Content)

	require.Equal(t, int64(3), countMessages(t, gormDB, d.ID), "старое + user + assistant")

	// fake получил историю (1) + новое сообщение пользователя
	require.Len(t, fake.LastMessages, 2)
	require.Equal(t, llms.ChatMessageTypeHuman, fake.LastMessages[1].Role)
}

func TestChatService_SendMessage_DialogNotFound(t *testing.T) {
	chat, _, _ := newChat(t, &llmtest.Fake{Answer: "x"})

	_, err := chat.SendMessage(context.Background(), 999, "hi")
	require.ErrorIs(t, err, service.ErrDialogNotFound)
}

func TestChatService_SendMessage_LLMFailure_NothingPersisted(t *testing.T) {
	fake := &llmtest.Fake{Err: errors.New("openai down")}
	chat, repo, gormDB := newChat(t, fake)
	ctx := context.Background()

	d := &model.Dialog{UserID: 1, Title: "chat"}
	require.NoError(t, repo.Create(ctx, d))

	_, err := chat.SendMessage(ctx, d.ID, "hi")
	require.Error(t, err)
	require.Equal(t, apperr.KindUpstream, apperr.KindOf(err))

	require.Equal(t, int64(0), countMessages(t, gormDB, d.ID), "при сбое LLM в БД ничего")
}
