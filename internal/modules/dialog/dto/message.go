package dto

import (
	"time"

	"github.com/flying-develop/ai-app-go/internal/modules/dialog/model"
)

// SendMessageRequest — тело запроса на отправку сообщения в диалог.
type SendMessageRequest struct {
	Text string `json:"text" binding:"required,max=8000"`
}

// MessageResponse — представление сообщения диалога в ответах API.
type MessageResponse struct {
	ID        uint      `json:"id"`
	DialogID  uint      `json:"dialog_id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// NewMessageResponse маппит модель в ответ API.
func NewMessageResponse(m *model.DialogMessage) MessageResponse {
	return MessageResponse{
		ID:        m.ID,
		DialogID:  m.DialogID,
		Role:      m.Role,
		Content:   m.Content,
		CreatedAt: m.CreatedAt,
	}
}

// NewMessageListResponse маппит срез моделей в срез ответов.
func NewMessageListResponse(messages []model.DialogMessage) []MessageResponse {
	out := make([]MessageResponse, 0, len(messages))
	for i := range messages {
		out = append(out, NewMessageResponse(&messages[i]))
	}
	return out
}
