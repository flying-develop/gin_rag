// Package llmtest предоставляет подставную реализацию llms.Model для тестов —
// без реальных сетевых вызовов к OpenAI.
package llmtest

import (
	"context"

	"github.com/tmc/langchaingo/llms"
)

// Fake — подставная chat-модель. Возвращает Answer (или Err, если задана)
// и запоминает последний переданный запрос в LastMessages.
type Fake struct {
	Answer       string
	Err          error
	LastMessages []llms.MessageContent
	Calls        int
}

// GenerateContent реализует llms.Model.
func (f *Fake) GenerateContent(_ context.Context, messages []llms.MessageContent, _ ...llms.CallOption) (*llms.ContentResponse, error) {
	f.Calls++
	f.LastMessages = messages
	if f.Err != nil {
		return nil, f.Err
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
