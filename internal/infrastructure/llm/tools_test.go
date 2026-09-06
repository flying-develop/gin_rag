package llm_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/llms"

	"github.com/flying-develop/ai-app-go/internal/infrastructure/llm"
	"github.com/flying-develop/ai-app-go/internal/infrastructure/llm/llmtest"
)

// fakeTool — инструмент-заглушка: запоминает полученные аргументы, отдаёт
// заранее заданный результат или ошибку.
type fakeTool struct {
	name    string
	result  string
	err     error
	gotArgs string
	calls   int
}

func (f *fakeTool) Name() string { return f.name }

func (f *fakeTool) Definition() llms.Tool {
	return llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        f.name,
			Description: "тестовый инструмент",
			Parameters:  map[string]any{"type": "object"},
		},
	}
}

func (f *fakeTool) Execute(_ context.Context, argumentsJSON string) (string, error) {
	f.calls++
	f.gotArgs = argumentsJSON
	if f.err != nil {
		return "", f.err
	}
	return f.result, nil
}

func toolCall(id, name, args string) llms.ToolCall {
	return llms.ToolCall{
		ID:           id,
		Type:         "function",
		FunctionCall: &llms.FunctionCall{Name: name, Arguments: args},
	}
}

// toolResponses собирает все ToolCallResponse из среза сообщений.
func toolResponses(msgs []llms.MessageContent) []llms.ToolCallResponse {
	var out []llms.ToolCallResponse
	for _, m := range msgs {
		for _, p := range m.Parts {
			if tr, ok := p.(llms.ToolCallResponse); ok {
				out = append(out, tr)
			}
		}
	}
	return out
}

func TestGenerateWithTools_NoToolCalls_SingleCall(t *testing.T) {
	fake := &llmtest.Fake{Responses: []llmtest.Response{{Content: "привет"}}}
	tool := &fakeTool{name: "noop"}
	input := []llms.MessageContent{llms.TextParts(llms.ChatMessageTypeHuman, "hi")}

	resp, err := llm.GenerateWithTools(context.Background(), fake, []llm.Tool{tool}, input, nil)
	require.NoError(t, err)
	require.Equal(t, "привет", resp.Choices[0].Content)
	require.Equal(t, 1, fake.Calls)
	require.Zero(t, tool.calls)
}

func TestGenerateWithTools_NilTools_PassesThrough(t *testing.T) {
	fake := &llmtest.Fake{Answer: "без инструментов"}
	input := []llms.MessageContent{llms.TextParts(llms.ChatMessageTypeHuman, "hi")}

	resp, err := llm.GenerateWithTools(context.Background(), fake, nil, input, nil)
	require.NoError(t, err)
	require.Equal(t, "без инструментов", resp.Choices[0].Content)
	require.Equal(t, 1, fake.Calls)
}

func TestGenerateWithTools_ExecutesToolAndReturnsFinal(t *testing.T) {
	fake := &llmtest.Fake{Responses: []llmtest.Response{
		{ToolCalls: []llms.ToolCall{toolCall("c1", "get_time", `{"timezone":"UTC"}`)}},
		{Content: "сейчас 12:00"},
	}}
	tool := &fakeTool{name: "get_time", result: "2026-09-06T12:00:00Z"}
	input := []llms.MessageContent{llms.TextParts(llms.ChatMessageTypeHuman, "который час?")}

	resp, err := llm.GenerateWithTools(context.Background(), fake, []llm.Tool{tool}, input, nil)
	require.NoError(t, err)
	require.Equal(t, "сейчас 12:00", resp.Choices[0].Content)
	require.Equal(t, 2, fake.Calls)
	require.Equal(t, `{"timezone":"UTC"}`, tool.gotArgs)

	// Второй вызов модели получил результат инструмента.
	require.Len(t, fake.Prompts, 2)
	trs := toolResponses(fake.Prompts[1])
	require.Len(t, trs, 1)
	require.Equal(t, "c1", trs[0].ToolCallID)
	require.Equal(t, "2026-09-06T12:00:00Z", trs[0].Content)
}

func TestGenerateWithTools_UnknownTool_Graceful(t *testing.T) {
	fake := &llmtest.Fake{Responses: []llmtest.Response{
		{ToolCalls: []llms.ToolCall{toolCall("c1", "nope", `{}`)}},
		{Content: "ладно"},
	}}
	tool := &fakeTool{name: "get_time"}
	input := []llms.MessageContent{llms.TextParts(llms.ChatMessageTypeHuman, "?")}

	resp, err := llm.GenerateWithTools(context.Background(), fake, []llm.Tool{tool}, input, nil)
	require.NoError(t, err)
	require.Equal(t, "ладно", resp.Choices[0].Content)
	require.Zero(t, tool.calls)

	trs := toolResponses(fake.Prompts[1])
	require.Len(t, trs, 1)
	require.Contains(t, trs[0].Content, "unknown tool")
}

func TestGenerateWithTools_ToolError_Graceful(t *testing.T) {
	fake := &llmtest.Fake{Responses: []llmtest.Response{
		{ToolCalls: []llms.ToolCall{toolCall("c1", "get_time", `{}`)}},
		{Content: "ок"},
	}}
	tool := &fakeTool{name: "get_time", err: errors.New("boom")}
	input := []llms.MessageContent{llms.TextParts(llms.ChatMessageTypeHuman, "?")}

	resp, err := llm.GenerateWithTools(context.Background(), fake, []llm.Tool{tool}, input, nil)
	require.NoError(t, err, "ошибка инструмента не должна просачиваться наружу")
	require.Equal(t, "ок", resp.Choices[0].Content)

	trs := toolResponses(fake.Prompts[1])
	require.Len(t, trs, 1)
	require.Contains(t, trs[0].Content, "failed")
	require.Contains(t, trs[0].Content, "boom")
}

func TestGenerateWithTools_DoesNotMutateInput(t *testing.T) {
	fake := &llmtest.Fake{Responses: []llmtest.Response{
		{ToolCalls: []llms.ToolCall{toolCall("c1", "get_time", `{}`)}},
		{Content: "финал"},
	}}
	tool := &fakeTool{name: "get_time", result: "ok"}
	input := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeSystem, "system"),
		llms.TextParts(llms.ChatMessageTypeHuman, "который час?"),
	}

	_, err := llm.GenerateWithTools(context.Background(), fake, []llm.Tool{tool}, input, nil)
	require.NoError(t, err)

	require.Len(t, input, 2, "входной срез не расширен")
	require.Equal(t, llms.ChatMessageTypeSystem, input[0].Role)
	require.Equal(t, llms.ChatMessageTypeHuman, input[1].Role)
}

func TestGenerateWithTools_PrimaryCallError_Wrapped(t *testing.T) {
	fake := &llmtest.Fake{Responses: []llmtest.Response{{Err: errors.New("openai down")}}}
	tool := &fakeTool{name: "get_time"}
	input := []llms.MessageContent{llms.TextParts(llms.ChatMessageTypeHuman, "?")}

	_, err := llm.GenerateWithTools(context.Background(), fake, []llm.Tool{tool}, input, nil)
	require.ErrorContains(t, err, "openai down")
}
