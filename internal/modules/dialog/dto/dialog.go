// Package dto — структуры запросов и ответов модуля dialog (граница API).
package dto

import (
	"time"

	"github.com/flying-develop/ai-app-go/internal/modules/dialog/model"
)

// CreateDialogRequest — тело запроса на создание диалога.
type CreateDialogRequest struct {
	UserID uint   `json:"user_id" binding:"required"`
	Title  string `json:"title" binding:"max=200"`
}

// UpdateDialogRequest — тело запроса на частичное обновление диалога.
// Поля-указатели: nil означает «не менять».
type UpdateDialogRequest struct {
	Title *string `json:"title" binding:"omitempty,max=200"`
}

// DialogResponse — представление диалога в ответах API.
type DialogResponse struct {
	ID        uint      `json:"id"`
	UserID    uint      `json:"user_id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NewDialogResponse маппит модель в ответ API.
func NewDialogResponse(d *model.Dialog) DialogResponse {
	return DialogResponse{
		ID:        d.ID,
		UserID:    d.UserID,
		Title:     d.Title,
		CreatedAt: d.CreatedAt,
		UpdatedAt: d.UpdatedAt,
	}
}

// NewDialogListResponse маппит срез моделей в срез ответов.
func NewDialogListResponse(dialogs []model.Dialog) []DialogResponse {
	out := make([]DialogResponse, 0, len(dialogs))
	for i := range dialogs {
		out = append(out, NewDialogResponse(&dialogs[i]))
	}
	return out
}
