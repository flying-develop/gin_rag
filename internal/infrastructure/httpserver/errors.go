package httpserver

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/flying-develop/ai-app-go/internal/apperr"
)

// errorHandler — middleware единой обработки ошибок.
//
// Хендлеры не пишут тело ошибки сами: они кладут ошибку через c.Error(err)
// и выходят. Этот middleware после c.Next() берёт последнюю ошибку,
// сопоставляет её категорию (apperr.Kind) с HTTP-кодом и отдаёт
// JSON вида {"error": "<сообщение>"}.
//
// Полный формат application/problem+json — на вехе «Устойчивость и
// наблюдаемость».
func errorHandler(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) == 0 || c.Writer.Written() {
			return
		}

		err := c.Errors.Last().Err
		status := statusForError(err)

		// 5xx логируем всегда — это сбои сервиса или его зависимостей.
		if status >= http.StatusInternalServerError {
			logger.Error("request failed",
				slog.Int("status", status),
				slog.String("path", c.Request.URL.Path),
				slog.String("method", c.Request.Method),
				slog.String("error", err.Error()),
			)
		}

		// Внутреннюю ошибку клиенту не раскрываем; для остальных категорий
		// (в т.ч. 502 upstream) отдаём заготовленное сообщение.
		if status == http.StatusInternalServerError {
			c.JSON(status, gin.H{"error": "internal error"})
			return
		}

		c.JSON(status, gin.H{"error": apperr.ClientMessage(err)})
	}
}

// statusForError сопоставляет ошибку с HTTP-кодом.
func statusForError(err error) int {
	switch apperr.KindOf(err) {
	case apperr.KindNotFound:
		return http.StatusNotFound
	case apperr.KindValidation:
		return http.StatusUnprocessableEntity
	case apperr.KindConflict:
		return http.StatusConflict
	case apperr.KindUpstream:
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}
