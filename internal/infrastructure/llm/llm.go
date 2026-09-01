// Package llm инициализирует chat-модель для langchaingo.
//
// Вызывающий код работает с интерфейсом llms.Model и не знает, какой
// провайдер за ним стоит — смена OpenAI на Anthropic/Gemini не требует
// правок в сервисах.
package llm

import (
	"fmt"
	"log/slog"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"

	"github.com/flying-develop/ai-app-go/internal/infrastructure/config"
)

// New создаёт chat-модель OpenAI по конфигу.
//
// Пустой OPENAI_API_KEY — ошибка: эндпоинты диалога с LLM без ключа
// работать не могут.
func New(cfg *config.Config, logger *slog.Logger) (llms.Model, error) {
	if cfg.OpenAIAPIKey == "" {
		return nil, fmt.Errorf("llm: OPENAI_API_KEY не задан")
	}

	client, err := openai.New(
		openai.WithToken(cfg.OpenAIAPIKey),
		openai.WithModel(cfg.OpenAIModel),
	)
	if err != nil {
		return nil, fmt.Errorf("llm: инициализация клиента OpenAI: %w", err)
	}

	logger.Info("llm client initialized", slog.String("model", cfg.OpenAIModel))
	return client, nil
}
