package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/tmc/langchaingo/llms"
	"gorm.io/gorm"

	"github.com/flying-develop/ai-app-go/internal/apperr"
	"github.com/flying-develop/ai-app-go/internal/infrastructure/db"
	"github.com/flying-develop/ai-app-go/internal/modules/dialog/model"
)

// historyLimit — сколько последних сообщений диалога передаётся в LLM.
const historyLimit = 100

// ChatService реализует базовый чат: сообщение пользователя → вызов LLM
// с историей диалога → сохранение обоих сообщений.
type ChatService struct {
	repo DialogRepository
	db   *gorm.DB
	llm  llms.Model
	log  *slog.Logger
}

// NewChatService собирает сервис чата.
func NewChatService(repo DialogRepository, database *gorm.DB, model llms.Model) *ChatService {
	return &ChatService{
		repo: repo,
		db:   database,
		llm:  model,
		log:  slog.Default().With(slog.String("component", "dialog.chat")),
	}
}

// SendMessage отправляет сообщение пользователя в диалог и возвращает
// ответ ассистента.
//
// Сообщения сохраняются в БД только после успешного ответа LLM (атомарно):
// при сбое вызова возвращается ошибка категории Upstream (HTTP 502), в БД
// ничего не пишется.
func (s *ChatService) SendMessage(ctx context.Context, dialogID uint, text string) (*model.DialogMessage, error) {
	if s.llm == nil {
		return nil, apperr.Upstream("llm not configured", nil)
	}

	dialog, err := s.repo.FindByID(ctx, dialogID)
	if err != nil {
		return nil, fmt.Errorf("load dialog %d: %w", dialogID, err)
	}
	if dialog == nil {
		return nil, ErrDialogNotFound
	}

	history, err := s.repo.ListMessages(ctx, dialogID, historyLimit)
	if err != nil {
		return nil, fmt.Errorf("load history for dialog %d: %w", dialogID, err)
	}

	prompt := buildPrompt(history, text)

	started := time.Now()
	resp, err := s.llm.GenerateContent(ctx, prompt)
	if err != nil {
		s.log.ErrorContext(ctx, "llm request failed",
			slog.Uint64("dialog_id", uint64(dialogID)),
			slog.String("error", err.Error()),
		)
		return nil, apperr.Upstream("llm request failed", err)
	}

	answer := firstChoice(resp)

	userMsg := &model.DialogMessage{DialogID: dialogID, Role: model.RoleUser, Content: text}
	assistantMsg := &model.DialogMessage{DialogID: dialogID, Role: model.RoleAssistant, Content: answer}

	if err := db.WithinTx(ctx, s.db, func(txCtx context.Context) error {
		return s.repo.AppendMessages(txCtx, userMsg, assistantMsg)
	}); err != nil {
		return nil, fmt.Errorf("persist messages for dialog %d: %w", dialogID, err)
	}

	s.log.InfoContext(ctx, "chat message sent",
		slog.Uint64("dialog_id", uint64(dialogID)),
		slog.Int("history_len", len(history)),
		slog.Int64("elapsed_ms", time.Since(started).Milliseconds()),
	)
	return assistantMsg, nil
}

// buildPrompt собирает историю и новое сообщение пользователя в формат
// langchaingo.
func buildPrompt(history []model.DialogMessage, userText string) []llms.MessageContent {
	msgs := make([]llms.MessageContent, 0, len(history)+1)
	for _, m := range history {
		msgs = append(msgs, llms.TextParts(chatRole(m.Role), m.Content))
	}
	msgs = append(msgs, llms.TextParts(llms.ChatMessageTypeHuman, userText))
	return msgs
}

// chatRole переводит роль из модели в тип langchaingo.
func chatRole(role string) llms.ChatMessageType {
	switch role {
	case model.RoleAssistant:
		return llms.ChatMessageTypeAI
	case model.RoleSystem:
		return llms.ChatMessageTypeSystem
	default:
		return llms.ChatMessageTypeHuman
	}
}

// firstChoice достаёт текст первого варианта ответа LLM.
func firstChoice(resp *llms.ContentResponse) string {
	if resp == nil || len(resp.Choices) == 0 {
		return ""
	}
	return resp.Choices[0].Content
}
