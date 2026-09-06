package llm

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/tmc/langchaingo/llms"
)

// Tool — исполняемый инструмент для LLM: описание для модели плюс реализация.
//
// Будущие модули (RAG-поиск, запросы к БД, внешние API) добавляют инструмент,
// реализовав этот интерфейс и передав его в GenerateWithTools — сам паттерн
// вызова не меняется.
type Tool interface {
	// Name — имя функции; должно совпадать с Definition().Function.Name.
	Name() string
	// Definition возвращает описание инструмента (JSON Schema параметров),
	// которое передаётся модели.
	Definition() llms.Tool
	// Execute выполняет инструмент. argumentsJSON — JSON-строка аргументов
	// от модели (может быть пустой). Возвращаемая строка уходит модели как
	// результат вызова.
	Execute(ctx context.Context, argumentsJSON string) (string, error)
}

// GenerateWithTools выполняет ровно один раунд tool calling:
//
//  1. запрос к модели со списком инструментов;
//  2. если модель не запросила инструменты — ответ возвращается как есть
//     (полная обратная совместимость с обычным GenerateContent);
//  3. иначе — каждый запрошенный инструмент выполняется, результат
//     возвращается модели, и финальный ответ модели возвращается вызывающему.
//
// Многошаговые агентные циклы (модель снова просит инструмент после получения
// результата) вне области этой функции — это веха «Диалог как state machine».
// Такой случай логируется как WARN, поведение не меняется.
//
// Входной срез messages не мутируется.
func GenerateWithTools(
	ctx context.Context,
	model llms.Model,
	tools []Tool,
	messages []llms.MessageContent,
	logger *slog.Logger,
	opts ...llms.CallOption,
) (*llms.ContentResponse, error) {
	if logger == nil {
		logger = slog.Default()
	}
	log := logger.With(slog.String("component", "llm.tools"))

	// Инструментов нет — обычный вызов, ровно один запрос к модели.
	if len(tools) == 0 {
		return model.GenerateContent(ctx, messages, opts...)
	}

	defs := make([]llms.Tool, 0, len(tools))
	byName := make(map[string]Tool, len(tools))
	for _, t := range tools {
		defs = append(defs, t.Definition())
		byName[t.Name()] = t
	}
	callOpts := append([]llms.CallOption{llms.WithTools(defs)}, opts...)

	resp, err := model.GenerateContent(ctx, messages, callOpts...)
	if err != nil {
		return nil, fmt.Errorf("llm: первичный вызов с инструментами: %w", err)
	}

	// Модель не запросила инструменты — возвращаем ответ как есть.
	if len(resp.Choices) == 0 || len(resp.Choices[0].ToolCalls) == 0 {
		return resp, nil
	}
	choice := resp.Choices[0]

	// Собираем новый срез сообщений: история + ответ модели с запросами
	// инструментов + результат каждого инструмента. Входной messages не трогаем.
	extended := make([]llms.MessageContent, 0, len(messages)+1+len(choice.ToolCalls))
	extended = append(extended, messages...)
	extended = append(extended, assistantToolCallMessage(choice))

	for _, tc := range choice.ToolCalls {
		extended = append(extended, llms.MessageContent{
			Role: llms.ChatMessageTypeTool,
			Parts: []llms.ContentPart{llms.ToolCallResponse{
				ToolCallID: tc.ID,
				Name:       toolCallName(tc),
				Content:    runToolCall(ctx, log, byName, tc),
			}},
		})
	}

	final, err := model.GenerateContent(ctx, extended, callOpts...)
	if err != nil {
		return nil, fmt.Errorf("llm: финальный вызов после инструментов: %w", err)
	}

	if len(final.Choices) > 0 && len(final.Choices[0].ToolCalls) > 0 {
		names := make([]string, 0, len(final.Choices[0].ToolCalls))
		for _, tc := range final.Choices[0].ToolCalls {
			names = append(names, toolCallName(tc))
		}
		log.WarnContext(ctx, "final response still requests tools; multi-round tool calling not supported",
			slog.Any("tool_names", names),
		)
	}

	return final, nil
}

// runToolCall находит и выполняет один инструмент, возвращая текст результата
// для модели. Любая проблема (неизвестный инструмент, ошибка выполнения,
// паника) деградирует в текстовое «Error: ...» — модель должна суметь на это
// ответить, а не ронять запрос 5xx.
func runToolCall(ctx context.Context, log *slog.Logger, byName map[string]Tool, tc llms.ToolCall) string {
	if tc.FunctionCall == nil {
		log.WarnContext(ctx, "malformed tool call", slog.String("tool_call_id", tc.ID))
		return "Error: malformed tool call"
	}
	name := tc.FunctionCall.Name

	tool, ok := byName[name]
	if !ok {
		log.WarnContext(ctx, "unknown tool requested", slog.String("tool_name", name))
		return fmt.Sprintf("Error: unknown tool %q", name)
	}

	result, err := safeExecute(ctx, tool, tc.FunctionCall.Arguments)
	if err != nil {
		log.WarnContext(ctx, "tool execution failed",
			slog.String("tool_name", name),
			slog.String("error_type", fmt.Sprintf("%T", err)),
			slog.String("error", err.Error()),
		)
		return fmt.Sprintf("Error: tool %q failed: %v", name, err)
	}

	log.InfoContext(ctx, "tool call executed",
		slog.String("tool_name", name),
		slog.String("tool_call_id", tc.ID),
	)
	return result
}

// safeExecute вызывает Execute, превращая панику инструмента в обычную ошибку —
// последний рубеж вокруг стороннего кода инструмента.
func safeExecute(ctx context.Context, tool Tool, argumentsJSON string) (result string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("паника в инструменте: %v", r)
		}
	}()
	return tool.Execute(ctx, argumentsJSON)
}

// assistantToolCallMessage восстанавливает сообщение ассистента с запросами
// инструментов — модель ожидает его в истории перед результатами инструментов.
func assistantToolCallMessage(choice *llms.ContentChoice) llms.MessageContent {
	parts := make([]llms.ContentPart, 0, len(choice.ToolCalls)+1)
	if choice.Content != "" {
		parts = append(parts, llms.TextContent{Text: choice.Content})
	}
	for _, tc := range choice.ToolCalls {
		parts = append(parts, tc)
	}
	return llms.MessageContent{Role: llms.ChatMessageTypeAI, Parts: parts}
}

// toolCallName безопасно достаёт имя функции из запроса инструмента.
func toolCallName(tc llms.ToolCall) string {
	if tc.FunctionCall == nil {
		return ""
	}
	return tc.FunctionCall.Name
}
