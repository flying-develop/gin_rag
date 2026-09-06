package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tmc/langchaingo/llms"

	"github.com/flying-develop/ai-app-go/internal/infrastructure/llm"
)

// DialogTools — инструменты, доступные ассистенту в диалоге. Список передаётся
// в ChatService; чтобы добавить новый инструмент, достаточно реализовать
// llm.Tool и дописать его сюда.
var DialogTools = []llm.Tool{currentTimeTool{}}

// currentTimeTool — пример инструмента: возвращает текущее время в таймзоне.
// Самодостаточен, без внешних зависимостей — служит образцом для будущих
// инструментов (RAG-поиск, запросы к БД, внешние API).
type currentTimeTool struct{}

// Name реализует llm.Tool.
func (currentTimeTool) Name() string { return "get_current_time" }

// Definition реализует llm.Tool.
func (currentTimeTool) Definition() llms.Tool {
	return llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        "get_current_time",
			Description: "Возвращает текущие дату и время в указанной таймзоне (формат RFC 3339).",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"timezone": map[string]any{
						"type":        "string",
						"description": "IANA-имя таймзоны, например Europe/Moscow. По умолчанию UTC.",
					},
				},
			},
		},
	}
}

// Execute реализует llm.Tool. Неизвестная таймзона деградирует в текстовое
// «Error: ...» (модель ответит сама), а не в ошибку выполнения.
func (currentTimeTool) Execute(_ context.Context, argumentsJSON string) (string, error) {
	var args struct {
		Timezone string `json:"timezone"`
	}
	if argumentsJSON != "" {
		if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
			return "", fmt.Errorf("разбор аргументов: %w", err)
		}
	}

	tz := args.Timezone
	if tz == "" {
		tz = "UTC"
	}

	loc, err := time.LoadLocation(tz)
	if err != nil {
		return fmt.Sprintf("Error: unknown timezone %q", tz), nil
	}

	return time.Now().In(loc).Format(time.RFC3339), nil
}
