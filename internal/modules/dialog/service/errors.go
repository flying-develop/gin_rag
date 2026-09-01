package service

import "github.com/flying-develop/ai-app-go/internal/apperr"

// ErrDialogNotFound возвращается, когда диалог с указанным id не существует.
// Категория KindNotFound → HTTP 404 в обработчике ошибок.
var ErrDialogNotFound = apperr.NotFound("dialog not found")
