package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/llms"

	"github.com/flying-develop/ai-app-go/internal/infrastructure/llm/llmtest"
	"github.com/flying-develop/ai-app-go/internal/infrastructure/logging"
)

func newMachine(fake *llmtest.Fake) *dialogMachine {
	logging.Setup("DEBUG")
	return newDialogMachine(fake, DialogTools, nil)
}

func TestDialogMachine_Run_NoToolCalls(t *testing.T) {
	fake := &llmtest.Fake{Answer: "привет"}
	m := newMachine(fake)

	initial := []llms.MessageContent{llms.TextParts(llms.ChatMessageTypeHuman, "хай")}
	st, err := m.Run(context.Background(), initial)
	require.NoError(t, err)
	require.Equal(t, "привет", lastMessageText(st.messages))
	require.Len(t, st.messages, len(initial)+1)
	require.Equal(t, 1, fake.Calls)
}

func TestDialogMachine_Run_ExecutesTool(t *testing.T) {
	fake := &llmtest.Fake{Responses: []llmtest.Response{
		{ToolCalls: []llms.ToolCall{{
			ID:           "c1",
			Type:         "function",
			FunctionCall: &llms.FunctionCall{Name: "get_current_time", Arguments: `{}`},
		}}},
		{Content: "финал"},
	}}
	m := newMachine(fake)

	st, err := m.Run(context.Background(), []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeHuman, "который час?"),
	})
	require.NoError(t, err)
	require.Equal(t, "финал", lastMessageText(st.messages), "последнее сообщение — финальный ответ, не tool-обмен")
	require.Equal(t, 2, fake.Calls)
}

func TestDialogMachine_Run_AccumulatesMessages(t *testing.T) {
	fake := &llmtest.Fake{Answer: "ok"}
	m := newMachine(fake)

	initial := []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeSystem, "system"),
		llms.TextParts(llms.ChatMessageTypeHuman, "hi"),
	}
	st, err := m.Run(context.Background(), initial)
	require.NoError(t, err)
	require.Len(t, st.messages, 3)
	require.Equal(t, llms.ChatMessageTypeSystem, st.messages[0].Role)
	require.Equal(t, llms.ChatMessageTypeHuman, st.messages[1].Role)
	require.Equal(t, llms.ChatMessageTypeAI, st.messages[2].Role)
}

func TestDialogMachine_Run_DoesNotMutateInput(t *testing.T) {
	fake := &llmtest.Fake{Answer: "ok"}
	m := newMachine(fake)

	initial := []llms.MessageContent{llms.TextParts(llms.ChatMessageTypeHuman, "hi")}
	_, err := m.Run(context.Background(), initial)
	require.NoError(t, err)
	require.Len(t, initial, 1, "входной срез не расширен")
	require.Equal(t, llms.ChatMessageTypeHuman, initial[0].Role)
}

func TestDialogMachine_Run_PropagatesError(t *testing.T) {
	fake := &llmtest.Fake{Err: errors.New("openai down")}
	m := newMachine(fake)

	_, err := m.Run(context.Background(), []llms.MessageContent{
		llms.TextParts(llms.ChatMessageTypeHuman, "hi"),
	})
	require.ErrorContains(t, err, "openai down")
	require.ErrorContains(t, err, `состояние "agent"`)
}
