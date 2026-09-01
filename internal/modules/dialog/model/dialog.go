// Package model содержит ORM-модели модуля dialog.
package model

import "time"

// Dialog — чат-сессия пользователя с ассистентом. Сообщения (DialogMessage)
// добавляются на вехе «Диалоги с LLM».
type Dialog struct {
	ID        uint      `gorm:"primaryKey"`
	UserID    uint      `gorm:"column:user_id;index"`
	Title     string    `gorm:"column:title"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

// TableName задаёт имя таблицы явно, не полагаясь на конвенцию GORM.
func (Dialog) TableName() string { return "dialogs" }
