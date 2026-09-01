// Package service содержит бизнес-логику модуля dialog (Application Services).
package service

import (
	"context"
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	"github.com/flying-develop/ai-app-go/internal/infrastructure/db"
	"github.com/flying-develop/ai-app-go/internal/modules/dialog/dto"
	"github.com/flying-develop/ai-app-go/internal/modules/dialog/model"
)

// DialogService — use cases по диалогам. Транзакции открываются здесь,
// операции с БД идут через repo.
type DialogService struct {
	repo DialogRepository
	db   *gorm.DB
	log  *slog.Logger
}

// NewDialogService собирает сервис из репозитория и подключения к БД.
func NewDialogService(repo DialogRepository, database *gorm.DB) *DialogService {
	return &DialogService{
		repo: repo,
		db:   database,
		log:  slog.Default().With(slog.String("component", "dialog.service")),
	}
}

// Create создаёт новый диалог.
func (s *DialogService) Create(ctx context.Context, req dto.CreateDialogRequest) (*model.Dialog, error) {
	d := &model.Dialog{UserID: req.UserID, Title: req.Title}
	if err := s.repo.Create(ctx, d); err != nil {
		return nil, fmt.Errorf("create dialog: %w", err)
	}
	s.log.InfoContext(ctx, "dialog created", slog.Uint64("dialog_id", uint64(d.ID)), slog.Uint64("user_id", uint64(d.UserID)))
	return d, nil
}

// GetByID возвращает диалог или ErrDialogNotFound.
func (s *DialogService) GetByID(ctx context.Context, id uint) (*model.Dialog, error) {
	d, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get dialog %d: %w", id, err)
	}
	if d == nil {
		return nil, ErrDialogNotFound
	}
	return d, nil
}

// List возвращает диалоги пользователя.
func (s *DialogService) List(ctx context.Context, userID uint, limit, offset int) ([]model.Dialog, error) {
	dialogs, err := s.repo.List(ctx, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list dialogs for user %d: %w", userID, err)
	}
	return dialogs, nil
}

// ListMessages возвращает историю сообщений диалога (по возрастанию id).
func (s *DialogService) ListMessages(ctx context.Context, dialogID uint) ([]model.DialogMessage, error) {
	messages, err := s.repo.ListMessages(ctx, dialogID, 0)
	if err != nil {
		return nil, fmt.Errorf("list messages for dialog %d: %w", dialogID, err)
	}
	return messages, nil
}

// Update применяет частичное изменение диалога внутри транзакции.
func (s *DialogService) Update(ctx context.Context, id uint, req dto.UpdateDialogRequest) (*model.Dialog, error) {
	var updated *model.Dialog

	err := db.WithinTx(ctx, s.db, func(txCtx context.Context) error {
		d, err := s.repo.FindByID(txCtx, id)
		if err != nil {
			return err
		}
		if d == nil {
			return ErrDialogNotFound
		}

		if req.Title != nil {
			d.Title = *req.Title
		}

		if err := s.repo.Update(txCtx, d); err != nil {
			return err
		}
		updated = d
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.log.InfoContext(ctx, "dialog updated", slog.Uint64("dialog_id", uint64(id)))
	return updated, nil
}

// Delete удаляет диалог; ErrDialogNotFound, если его не было.
func (s *DialogService) Delete(ctx context.Context, id uint) error {
	deleted, err := s.repo.Delete(ctx, id)
	if err != nil {
		return fmt.Errorf("delete dialog %d: %w", id, err)
	}
	if !deleted {
		return ErrDialogNotFound
	}
	s.log.InfoContext(ctx, "dialog deleted", slog.Uint64("dialog_id", uint64(id)))
	return nil
}
