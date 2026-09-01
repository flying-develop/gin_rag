// Package handler — HTTP-обработчики модуля dialog (тонкий слой:
// биндинг, вызов сервиса, формирование ответа).
package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/flying-develop/ai-app-go/internal/apperr"
	"github.com/flying-develop/ai-app-go/internal/modules/dialog/dto"
	"github.com/flying-develop/ai-app-go/internal/modules/dialog/service"
)

// DialogHandler обслуживает CRUD-эндпоинты диалогов.
type DialogHandler struct {
	svc *service.DialogService
}

// NewDialogHandler создаёт обработчик поверх сервиса модуля.
func NewDialogHandler(svc *service.DialogService) *DialogHandler {
	return &DialogHandler{svc: svc}
}

// Register монтирует роуты модуля в группу (например, /api/v1).
func (h *DialogHandler) Register(rg *gin.RouterGroup) {
	rg.POST("/dialogs", h.Create)
	rg.GET("/dialogs", h.List)
	rg.GET("/dialogs/:id", h.Get)
	rg.PATCH("/dialogs/:id", h.Update)
	rg.DELETE("/dialogs/:id", h.Delete)
}

// Create — POST /dialogs.
func (h *DialogHandler) Create(c *gin.Context) {
	var req dto.CreateDialogRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperr.Validationf("invalid body: %v", err))
		return
	}

	d, err := h.svc.Create(c.Request.Context(), req)
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, dto.NewDialogResponse(d))
}

// List — GET /dialogs?user_id=&limit=&offset=.
func (h *DialogHandler) List(c *gin.Context) {
	userID, err := queryUint(c, "user_id")
	if err != nil {
		_ = c.Error(err)
		return
	}
	limit := queryIntDefault(c, "limit", 0)
	offset := queryIntDefault(c, "offset", 0)

	dialogs, err := h.svc.List(c.Request.Context(), userID, limit, offset)
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.NewDialogListResponse(dialogs))
}

// Get — GET /dialogs/:id.
func (h *DialogHandler) Get(c *gin.Context) {
	id, err := pathUint(c, "id")
	if err != nil {
		_ = c.Error(err)
		return
	}

	d, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.NewDialogResponse(d))
}

// Update — PATCH /dialogs/:id.
func (h *DialogHandler) Update(c *gin.Context) {
	id, err := pathUint(c, "id")
	if err != nil {
		_ = c.Error(err)
		return
	}

	var req dto.UpdateDialogRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperr.Validationf("invalid body: %v", err))
		return
	}

	d, err := h.svc.Update(c.Request.Context(), id, req)
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.NewDialogResponse(d))
}

// Delete — DELETE /dialogs/:id.
func (h *DialogHandler) Delete(c *gin.Context) {
	id, err := pathUint(c, "id")
	if err != nil {
		_ = c.Error(err)
		return
	}

	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		_ = c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}

// pathUint парсит положительный целочисленный path-параметр.
func pathUint(c *gin.Context, name string) (uint, error) {
	v, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil {
		return 0, apperr.Validationf("invalid path parameter %q", name)
	}
	return uint(v), nil
}

// queryUint парсит обязательный положительный целочисленный query-параметр.
func queryUint(c *gin.Context, name string) (uint, error) {
	raw := c.Query(name)
	if raw == "" {
		return 0, apperr.Validationf("query parameter %q is required", name)
	}
	v, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, apperr.Validationf("invalid query parameter %q", name)
	}
	return uint(v), nil
}

// queryIntDefault парсит необязательный целочисленный query-параметр.
func queryIntDefault(c *gin.Context, name string, def int) int {
	raw := c.Query(name)
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return v
}
