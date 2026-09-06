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
// реализовав этот интерфейс и передав его в машину состояний диалога или в
// GenerateWithTools — сам паттерн вызова не меняется.
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

// ToolDefinitions собирает описания инструментов для llms.WithTools.
func ToolDefinitions(tools []Tool) []llms.Tool {
	defs := make([]llms.Tool, 0, len(tools))
	for _, t := range tools {
		defs = append(defs, t.Definition())
	}
	return defs
}

// AssistantToolCallMessage восстанавливает сообщение ассистента (текст плюс
// запросы инструментов) из варианта ответа модели — модель ожидает его в
// истории перед результатами инструментов. nil-choice → пустое AI-сообщение.
func AssistantToolCallMessage(choice *llms.ContentChoice) llms.MessageContent {
	if choice == nil {
		return llms.MessageContent{Role: llms.ChatMessageTypeAI}
	}
	parts := make([]llms.ContentPart, 0, len(choice.ToolCalls)+1)
	if choice.Content != "" {
		parts = append(parts, llms.TextContent{Text: choice.Content})
	}
	for _, tc := range choice.ToolCalls {
		parts = append(parts, tc)
	}
	return llms.MessageContent{Role: llms.ChatMessageTypeAI, Parts: parts}
}

// ToolCallsFromMessage извлекает запросы инструментов из сообщения.
func ToolCallsFromMessage(msg llms.MessageContent) []llms.ToolCall {
	var calls []llms.ToolCall
	for _, p := range msg.Parts {
		if tc, ok := p.(llms.ToolCall); ok {
			calls = append(calls, tc)
		}
	}
	return calls
}

// ExecuteToolCalls выполняет запрошенные моделью инструменты и возвращает по
// одному tool-сообщению на каждый вызов, в том же порядке.
//
// Любая проблема (неизвестный инструмент, ошибка выполнения, паника,
// некорректный запрос) деградирует в текстовое «Error: ...» — модель должна
// суметь на это ответить, а не ронять запрос 5xx.
func ExecuteToolCalls(ctx context.Context, tools []Tool, calls []llms.ToolCall, logger *slog.Logger) []llms.MessageContent {
	if logger == nil {
		logger = slog.Default()
	}
	log := logger.With(slog.String("component", "llm.tools"))

	byName := make(map[string]Tool, len(tools))
	for _, t := range tools {
		byName[t.Name()] = t
	}

	msgs := make([]llms.MessageContent, 0, len(calls))
	for _, tc := range calls {
		msgs = append(msgs, llms.MessageContent{
			Role: llms.ChatMessageTypeTool,
			Parts: []llms.ContentPart{llms.ToolCallResponse{
				ToolCallID: tc.ID,
				Name:       toolCallName(tc),
				Content:    runToolCall(ctx, log, byName, tc),
			}},
		})
	}
	return msgs
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
// результата) реализованы машиной состояний диалога, а не здесь; в этой
// функции такой случай логируется как WARN, поведение не меняется.
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

	callOpts := append([]llms.CallOption{llms.WithTools(ToolDefinitions(tools))}, opts...)

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
	extended = append(extended, AssistantToolCallMessage(choice))
	extended = append(extended, ExecuteToolCalls(ctx, tools, choice.ToolCalls, logger)...)

	final, err := model.GenerateContent(ctx, extended, callOpts...)
	if err != nil {
		return nil, fmt.Errorf("llm: финальный вызов после инструментов: %w", err)
	}

	if len(final.Choices) > 0 && len(final.Choices[0].ToolCalls) > 0 {
		names := make([]string, 0, len(final.Choices[0].ToolCalls))
		for _, tc := range final.Choices[0].ToolCalls {
			names = append(names, toolCallName(tc))
		}
		log.WarnContext(ctx, "final response still requests tools; multi-round tool calling not supported here",
			slog.Any("tool_names", names),
		)
	}

	return final, nil
}

// runToolCall находит и выполняет один инструмент, возвращая текст результата
// для модели.
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

// toolCallName безопасно достаёт имя функции из запроса инструмента.
func toolCallName(tc llms.ToolCall) string {
	if tc.FunctionCall == nil {
		return ""
	}
	return tc.FunctionCall.Name
}
