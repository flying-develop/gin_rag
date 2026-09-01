package model

import "time"

// Роли сообщений в диалоге.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
)

// DialogMessage — одно сообщение в рамках диалога (реплика пользователя,
// ассистента или системный prompt).
type DialogMessage struct {
	ID        uint      `gorm:"primaryKey"`
	DialogID  uint      `gorm:"column:dialog_id;index"`
	Role      string    `gorm:"column:role"`
	Content   string    `gorm:"column:content"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

// TableName задаёт имя таблицы явно.
func (DialogMessage) TableName() string { return "dialog_messages" }
