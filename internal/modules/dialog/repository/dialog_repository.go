// Package repository — единственная точка доступа к БД для модуля dialog.
package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/flying-develop/ai-app-go/internal/infrastructure/db"
	"github.com/flying-develop/ai-app-go/internal/modules/dialog/model"
)

// defaultListLimit применяется, когда вызывающий код не задал лимит.
const defaultListLimit = 50

// DialogRepository реализует service.DialogRepository поверх GORM.
type DialogRepository struct {
	db *gorm.DB
}

// NewDialogRepository создаёт репозиторий на переданном подключении.
func NewDialogRepository(database *gorm.DB) *DialogRepository {
	return &DialogRepository{db: database}
}

// conn возвращает активную транзакцию из ctx либо базовое подключение.
func (r *DialogRepository) conn(ctx context.Context) *gorm.DB {
	return db.Conn(ctx, r.db).WithContext(ctx)
}

func (r *DialogRepository) Create(ctx context.Context, d *model.Dialog) error {
	return r.conn(ctx).Create(d).Error
}

func (r *DialogRepository) FindByID(ctx context.Context, id uint) (*model.Dialog, error) {
	var d model.Dialog
	err := r.conn(ctx).First(&d, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *DialogRepository) List(ctx context.Context, userID uint, limit, offset int) ([]model.Dialog, error) {
	if limit <= 0 {
		limit = defaultListLimit
	}
	if offset < 0 {
		offset = 0
	}

	var dialogs []model.Dialog
	err := r.conn(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&dialogs).Error
	if err != nil {
		return nil, err
	}
	return dialogs, nil
}

func (r *DialogRepository) Update(ctx context.Context, d *model.Dialog) error {
	return r.conn(ctx).Save(d).Error
}

func (r *DialogRepository) Delete(ctx context.Context, id uint) (bool, error) {
	res := r.conn(ctx).Delete(&model.Dialog{}, id)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}
