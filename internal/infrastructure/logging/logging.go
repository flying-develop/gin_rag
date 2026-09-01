// Package logging настраивает структурированное логирование приложения
// на базе стандартного log/slog. Формат вывода — key=value (TextHandler),
// уровень управляется переменной окружения LOG_LEVEL.
package logging

import (
	"log/slog"
	"os"
	"strings"
)

// Setup конфигурирует глобальный логгер slog и возвращает его.
//
// Вызывать один раз при старте приложения, до первых записей в лог.
// Уровень полностью определяется параметром level (значение LOG_LEVEL):
// DEBUG | INFO | WARN | ERROR, регистр не важен. Неизвестное значение
// трактуется как INFO.
func Setup(level string) *slog.Logger {
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLevel(level),
	})

	logger := slog.New(handler)
	slog.SetDefault(logger)

	logger.Info("logging initialized", slog.String("level", normalizeLevel(level)))

	return logger
}

// parseLevel переводит строковое имя уровня в slog.Level.
func parseLevel(level string) slog.Level {
	switch normalizeLevel(level) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// normalizeLevel приводит имя уровня к верхнему регистру без пробелов;
// неизвестные значения заменяет на INFO.
func normalizeLevel(level string) string {
	switch v := strings.ToUpper(strings.TrimSpace(level)); v {
	case "DEBUG", "INFO", "WARN", "ERROR":
		return v
	default:
		return "INFO"
	}
}
