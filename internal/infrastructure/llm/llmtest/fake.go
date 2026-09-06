// Package llmtest предоставляет подставную реализацию llms.Model для тестов —
// без реальных сетевых вызовов к OpenAI.
package llmtest

import (
	"context"

	"github.com/tmc/langchaingo/llms"
)

// Response — один заскриптованный ответ модели для Fake.Responses.
// Позволяет тестировать tool calling: первый Response с ToolCalls, следующий —
// финальный текст.
type Response struct {
	Content   string
	ToolCalls []llms.ToolCall
	Err       error
}

// Fake — подставная chat-модель.
//
// Режим по умолчанию: возвращает Answer (или Err, если задана) на каждый вызов.
// Режим скрипта: если задан Responses, каждый вызов GenerateContent берёт
// следующий элемент очереди (для многошаговых сценариев вроде tool calling).
type Fake struct {
	Answer string
	Err    error

	// Responses — очередь заскриптованных ответов; если непусто, имеет
	// приоритет над Answer/Err.
	Responses []Response

	// ToolLoop — если задан (и Responses пуст), каждый вызов возвращает ответ
	// с этими ToolCalls. Для теста лимита шагов машины состояний.
	ToolLoop []llms.ToolCall

	LastMessages []llms.MessageContent
	// Prompts — копия messages каждого вызова GenerateContent по порядку.
	Prompts [][]llms.MessageContent
	Calls   int
}

// GenerateContent реализует llms.Model.
func (f *Fake) GenerateContent(_ context.Context, messages []llms.MessageContent, _ ...llms.CallOption) (*llms.ContentResponse, error) {
	f.Calls++
	f.LastMessages = messages
	f.Prompts = append(f.Prompts, append([]llms.MessageContent(nil), messages...))

	if len(f.Responses) > 0 {
		r := f.Responses[0]
		f.Responses = f.Responses[1:]
		if r.Err != nil {
			return nil, r.Err
		}
		return &llms.ContentResponse{
			Choices: []*llms.ContentChoice{{Content: r.Content, ToolCalls: r.ToolCalls}},
		}, nil
	}

	if f.Err != nil {
		return nil, f.Err
	}
	if len(f.ToolLoop) > 0 {
		return &llms.ContentResponse{
			Choices: []*llms.ContentChoice{{ToolCalls: f.ToolLoop}},
		}, nil
	}
	return &llms.ContentResponse{
		Choices: []*llms.ContentChoice{{Content: f.Answer}},
	}, nil
}

// Call реализует llms.Model (в проекте не используется).
func (f *Fake) Call(_ context.Context, prompt string, _ ...llms.CallOption) (string, error) {
	f.Calls++
	if f.Err != nil {
		return "", f.Err
	}
	return f.Answer, nil
}
