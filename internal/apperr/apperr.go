// Package apperr — общий словарь доменных ошибок приложения.
//
// Живёт вне infrastructure/ и modules/, чтобы и доменные модули, и
// HTTP-слой могли зависеть от него, не нарушая правил зависимостей
// (см. .ai-factory/ARCHITECTURE.md). HTTP-обработчик ошибок использует
// Kind для выбора кода ответа.
package apperr

import (
	"errors"
	"fmt"
)

// Kind — категория ошибки, определяющая HTTP-код ответа.
type Kind int

const (
	// KindInternal — непредвиденная ошибка (500).
	KindInternal Kind = iota
	// KindNotFound — запрошенный ресурс не существует (404).
	KindNotFound
	// KindValidation — некорректный ввод (422).
	KindValidation
	// KindConflict — конфликт состояния (409).
	KindConflict
	// KindUpstream — сбой внешнего сервиса, от которого зависит запрос (502).
	KindUpstream
)

// Error — доменная ошибка с категорией и сообщением для клиента.
type Error struct {
	Kind    Kind
	Message string
	Err     error // опциональная обёрнутая причина (в ответ не попадает)
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Err }

// NotFound создаёт ошибку категории KindNotFound.
func NotFound(message string) *Error {
	return &Error{Kind: KindNotFound, Message: message}
}

// Validation создаёт ошибку категории KindValidation.
func Validation(message string) *Error {
	return &Error{Kind: KindValidation, Message: message}
}

// Validationf — Validation с форматированием.
func Validationf(format string, args ...any) *Error {
	return &Error{Kind: KindValidation, Message: fmt.Sprintf(format, args...)}
}

// Conflict создаёт ошибку категории KindConflict.
func Conflict(message string) *Error {
	return &Error{Kind: KindConflict, Message: message}
}

// Upstream создаёт ошибку категории KindUpstream с обёрнутой причиной.
func Upstream(message string, cause error) *Error {
	return &Error{Kind: KindUpstream, Message: message, Err: cause}
}

// KindOf извлекает категорию из ошибки; для не-*Error возвращает KindInternal.
func KindOf(err error) Kind {
	if e := as(err); e != nil {
		return e.Kind
	}
	return KindInternal
}

// ClientMessage возвращает сообщение, безопасное для отдачи клиенту.
func ClientMessage(err error) string {
	if e := as(err); e != nil {
		return e.Message
	}
	return "internal error"
}

// as разворачивает цепочку ошибок до первого *Error.
func as(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return nil
}
