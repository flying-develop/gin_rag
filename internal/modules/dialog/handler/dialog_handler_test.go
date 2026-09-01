package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/flying-develop/ai-app-go/internal/infrastructure/config"
	"github.com/flying-develop/ai-app-go/internal/infrastructure/db"
	"github.com/flying-develop/ai-app-go/internal/infrastructure/httpserver"
	"github.com/flying-develop/ai-app-go/internal/infrastructure/llm/llmtest"
	"github.com/flying-develop/ai-app-go/internal/infrastructure/logging"
	"github.com/flying-develop/ai-app-go/internal/modules/dialog/handler"
	"github.com/flying-develop/ai-app-go/internal/modules/dialog/repository"
	"github.com/flying-develop/ai-app-go/internal/modules/dialog/service"
)

// newRouter собирает реальный движок httpserver.New (с продовым
// обработчиком ошибок) + смонтированный модуль dialog, работающий против
// реального Postgres из docker-compose.
func newRouter(t *testing.T) (*gin.Engine, *llmtest.Fake) {
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
	svc := service.NewDialogService(repo, gormDB)
	fake := &llmtest.Fake{Answer: "ответ ассистента"}
	chat := service.NewChatService(repo, gormDB, fake)

	engine := httpserver.New(cfg, slog.Default())
	handler.NewDialogHandler(svc, chat).Register(engine.Group("/api/v1"))
	return engine, fake
}

func do(t *testing.T, engine *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func TestDialogHandler_CRUDCycle(t *testing.T) {
	engine, _ := newRouter(t)

	rec := do(t, engine, http.MethodPost, "/api/v1/dialogs", gin.H{"user_id": 1, "title": "hi"})
	require.Equal(t, http.StatusCreated, rec.Code)
	var created struct{ ID uint }
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	require.NotZero(t, created.ID)

	rec = do(t, engine, http.MethodGet, "/api/v1/dialogs/1", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	rec = do(t, engine, http.MethodGet, "/api/v1/dialogs?user_id=1", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var list []map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Len(t, list, 1)

	rec = do(t, engine, http.MethodPatch, "/api/v1/dialogs/1", gin.H{"title": "renamed"})
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "renamed")

	rec = do(t, engine, http.MethodDelete, "/api/v1/dialogs/1", nil)
	require.Equal(t, http.StatusNoContent, rec.Code)

	rec = do(t, engine, http.MethodGet, "/api/v1/dialogs/1", nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDialogHandler_Validation(t *testing.T) {
	engine, _ := newRouter(t)

	rec := do(t, engine, http.MethodPost, "/api/v1/dialogs", gin.H{"title": "no user id"})
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	rec = do(t, engine, http.MethodGet, "/api/v1/dialogs", nil)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestDialogHandler_SendMessage(t *testing.T) {
	engine, fake := newRouter(t)

	rec := do(t, engine, http.MethodPost, "/api/v1/dialogs", gin.H{"user_id": 1, "title": "chat"})
	require.Equal(t, http.StatusCreated, rec.Code)
	var created struct{ ID uint }
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	path := fmt.Sprintf("/api/v1/dialogs/%d/messages", created.ID)
	rec = do(t, engine, http.MethodPost, path, gin.H{"text": "привет"})
	require.Equal(t, http.StatusCreated, rec.Code)
	require.Contains(t, rec.Body.String(), "ответ ассистента")
	require.Equal(t, 1, fake.Calls)

	rec = do(t, engine, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var msgs []map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &msgs))
	require.Len(t, msgs, 2)
}

func TestDialogHandler_SendMessage_Errors(t *testing.T) {
	engine, _ := newRouter(t)

	rec := do(t, engine, http.MethodPost, "/api/v1/dialogs/999/messages", gin.H{"text": "hi"})
	require.Equal(t, http.StatusNotFound, rec.Code)

	rec = do(t, engine, http.MethodPost, "/api/v1/dialogs", gin.H{"user_id": 1})
	require.Equal(t, http.StatusCreated, rec.Code)
	var created struct{ ID uint }
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	rec = do(t, engine, http.MethodPost, fmt.Sprintf("/api/v1/dialogs/%d/messages", created.ID), gin.H{})
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}
