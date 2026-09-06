package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/tmc/langchaingo/llms"

	"github.com/flying-develop/ai-app-go/internal/infrastructure/llm"
)

// maxDialogSteps — предохранитель против бесконечного цикла agent ⇄ tools
// (модель раз за разом запрашивает инструменты). Превышение → ошибка,
// которая в ChatService превращается в 502.
const maxDialogSteps = 25

// dialogState — состояние машины: накопленные сообщения диалога.
type dialogState struct {
	messages []llms.MessageContent
}

// stateName — имя состояния машины.
type stateName string

const (
	// stateAgent — «спросить модель» (с доступом к инструментам).
	stateAgent stateName = "agent"
	// stateTools — «выполнить инструменты, запрошенные моделью».
	stateTools stateName = "tools"
	// stateEnd — терминальное состояние: машина останавливается.
	stateEnd stateName = ""
)

// transition — результат шага состояния: что добавить в состояние и куда перейти.
type transition struct {
	messages []llms.MessageContent
	next     stateName
}

// stateFunc — реализация одного состояния машины.
type stateFunc func(ctx context.Context, st *dialogState) (transition, error)

// dialogMachine — явная машина состояний диалога.
//
// Состояния agent ⇄ tools с условным переходом: agent спрашивает модель и,
// если та запросила инструменты, переходит в tools (выполнить и вернуться в
// agent), иначе — в терминальное состояние. Цикл повторяется, пока модель
// запрашивает инструменты (не более maxDialogSteps шагов).
type dialogMachine struct {
	start  stateName
	states map[stateName]stateFunc
	log    *slog.Logger
}

// newDialogMachine строит машину состояний диалога.
func newDialogMachine(model llms.Model, tools []llm.Tool, logger *slog.Logger) *dialogMachine {
	if logger == nil {
		logger = slog.Default()
	}
	m := &dialogMachine{
		start: stateAgent,
		log:   logger.With(slog.String("component", "dialog.machine")),
	}

	defs := llm.ToolDefinitions(tools)
	var callOpts []llms.CallOption
	if len(defs) > 0 {
		callOpts = []llms.CallOption{llms.WithTools(defs)}
	}

	m.states = map[stateName]stateFunc{
		stateAgent: func(ctx context.Context, st *dialogState) (transition, error) {
			m.log.InfoContext(ctx, "state enter",
				slog.String("state", string(stateAgent)),
				slog.Int("messages", len(st.messages)),
			)

			resp, err := model.GenerateContent(ctx, st.messages, callOpts...)
			if err != nil {
				return transition{}, err
			}
			choice := firstChoiceOf(resp)
			msg := llm.AssistantToolCallMessage(choice)

			next := stateEnd
			toolCalls := 0
			if choice != nil && len(choice.ToolCalls) > 0 {
				next = stateTools
				toolCalls = len(choice.ToolCalls)
			}

			m.log.InfoContext(ctx, "state exit",
				slog.String("state", string(stateAgent)),
				slog.Int("tool_calls", toolCalls),
			)
			return transition{messages: []llms.MessageContent{msg}, next: next}, nil
		},

		stateTools: func(ctx context.Context, st *dialogState) (transition, error) {
			calls := llm.ToolCallsFromMessage(st.messages[len(st.messages)-1])
			m.log.InfoContext(ctx, "state enter",
				slog.String("state", string(stateTools)),
				slog.Int("tool_calls", len(calls)),
			)

			toolMsgs := llm.ExecuteToolCalls(ctx, tools, calls, m.log)

			m.log.InfoContext(ctx, "state exit",
				slog.String("state", string(stateTools)),
				slog.Int("results", len(toolMsgs)),
			)
			return transition{messages: toolMsgs, next: stateAgent}, nil
		},
	}
	return m
}

// Run прогоняет машину от стартового состояния до терминального, накапливая
// сообщения в состоянии. Входной срез initial не мутируется.
func (m *dialogMachine) Run(ctx context.Context, initial []llms.MessageContent) (*dialogState, error) {
	st := &dialogState{messages: append([]llms.MessageContent(nil), initial...)}

	steps := 0
	for current := m.start; current != stateEnd; {
		steps++
		if steps > maxDialogSteps {
			return nil, fmt.Errorf("dialog machine: превышен лимит шагов (%d)", maxDialogSteps)
		}

		fn, ok := m.states[current]
		if !ok {
			return nil, fmt.Errorf("dialog machine: неизвестное состояние %q", current)
		}
		tr, err := fn(ctx, st)
		if err != nil {
			return nil, fmt.Errorf("dialog machine: состояние %q: %w", current, err)
		}
		st.messages = append(st.messages, tr.messages...)
		current = tr.next
	}
	return st, nil
}

// firstChoiceOf достаёт первый вариант ответа модели (или nil).
func firstChoiceOf(resp *llms.ContentResponse) *llms.ContentChoice {
	if resp == nil || len(resp.Choices) == 0 {
		return nil
	}
	return resp.Choices[0]
}

// lastMessageText собирает текст из всех llms.TextContent-частей последнего
// сообщения состояния — финальный ответ ассистента.
func lastMessageText(msgs []llms.MessageContent) string {
	if len(msgs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, p := range msgs[len(msgs)-1].Parts {
		if tc, ok := p.(llms.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}
