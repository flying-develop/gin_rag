package service

import (
	"context"

	"github.com/flying-develop/ai-app-go/internal/modules/dialog/model"
)

// DialogRepository — доступ к хранилищу диалогов. Интерфейс объявлен на
// стороне потребителя (сервиса); реализация — в пакете repository.
//
// Транзакции прозрачны: если ctx несёт активную транзакцию (см.
// db.WithinTx), методы репозитория выполняются в ней автоматически.
type DialogRepository interface {
	// Create сохраняет новый диалог и заполняет d.ID.
	Create(ctx context.Context, d *model.Dialog) error
	// FindByID возвращает диалог по id или (nil, nil), если его нет.
	FindByID(ctx context.Context, id uint) (*model.Dialog, error)
	// List возвращает диалоги пользователя, новые первыми.
	List(ctx context.Context, userID uint, limit, offset int) ([]model.Dialog, error)
	// Update сохраняет изменения существующего диалога.
	Update(ctx context.Context, d *model.Dialog) error
	// Delete удаляет диалог; false — если строки с таким id не было.
	Delete(ctx context.Context, id uint) (bool, error)
}
