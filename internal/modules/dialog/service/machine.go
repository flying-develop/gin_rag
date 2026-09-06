package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/tmc/langchaingo/llms"

	"github.com/flying-develop/ai-app-go/internal/infrastructure/llm"
)

// dialogState — состояние машины: накопленные сообщения диалога.
type dialogState struct {
	messages []llms.MessageContent
}

// stateName — имя состояния машины.
type stateName string

const (
	// stateAgent — состояние «спросить модель» (с доступом к инструментам).
	stateAgent stateName = "agent"
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
// Пока одно состояние (agent → END): переходный шаг перед разложением на
// состояния agent/tools с циклом (многошаговый tool calling).
type dialogMachine struct {
	start  stateName
	states map[stateName]stateFunc
	log    *slog.Logger
}

// newDialogMachine строит машину с единственным состоянием agent, внутри
// которого вызывается llm.GenerateWithTools — логика вызова модели и
// инструментов не меняется, меняется только оболочка.
func newDialogMachine(model llms.Model, tools []llm.Tool, logger *slog.Logger) *dialogMachine {
	if logger == nil {
		logger = slog.Default()
	}
	m := &dialogMachine{
		start: stateAgent,
		log:   logger.With(slog.String("component", "dialog.machine")),
	}
	m.states = map[stateName]stateFunc{
		stateAgent: func(ctx context.Context, st *dialogState) (transition, error) {
			m.log.InfoContext(ctx, "state enter",
				slog.String("state", string(stateAgent)),
				slog.Int("messages", len(st.messages)),
			)

			resp, err := llm.GenerateWithTools(ctx, model, tools, st.messages, m.log)
			if err != nil {
				return transition{}, err
			}
			answer := firstChoice(resp)

			m.log.InfoContext(ctx, "state exit",
				slog.String("state", string(stateAgent)),
				slog.Int("answer_len", len(answer)),
			)
			return transition{
				messages: []llms.MessageContent{llms.TextParts(llms.ChatMessageTypeAI, answer)},
				next:     stateEnd,
			}, nil
		},
	}
	return m
}

// Run прогоняет машину от стартового состояния до терминального, накапливая
// сообщения в состоянии. Входной срез initial не мутируется.
func (m *dialogMachine) Run(ctx context.Context, initial []llms.MessageContent) (*dialogState, error) {
	st := &dialogState{messages: append([]llms.MessageContent(nil), initial...)}

	for current := m.start; current != stateEnd; {
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
