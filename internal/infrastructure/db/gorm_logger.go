package db

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm/logger"
)

// slogLogger — адаптер logger.Interface из GORM на *slog.Logger.
// Информационные сообщения GORM идут на INFO, предупреждения — на WARN,
// ошибки — на ERROR, а трассировка SQL-запросов — на DEBUG (и ERROR для
// запросов, завершившихся ошибкой).
type slogLogger struct {
	logger *slog.Logger
}

func newSlogLogger(l *slog.Logger) logger.Interface {
	if l == nil {
		l = slog.Default()
	}
	return &slogLogger{logger: l.With(slog.String("component", "gorm"))}
}

// LogMode оставлен для совместимости с интерфейсом: уровень логирования
// целиком определяется настройкой slog (LOG_LEVEL).
func (s *slogLogger) LogMode(logger.LogLevel) logger.Interface { return s }

func (s *slogLogger) Info(ctx context.Context, msg string, data ...any) {
	s.logger.InfoContext(ctx, fmt.Sprintf(msg, data...))
}

func (s *slogLogger) Warn(ctx context.Context, msg string, data ...any) {
	s.logger.WarnContext(ctx, fmt.Sprintf(msg, data...))
}

func (s *slogLogger) Error(ctx context.Context, msg string, data ...any) {
	s.logger.ErrorContext(ctx, fmt.Sprintf(msg, data...))
}

// Trace вызывается GORM после каждого SQL-запроса.
func (s *slogLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, rows := fc()
	attrs := []any{
		slog.String("sql", sql),
		slog.Int64("rows", rows),
		slog.Duration("elapsed", time.Since(begin)),
	}

	if err != nil && !errors.Is(err, logger.ErrRecordNotFound) {
		s.logger.ErrorContext(ctx, "sql error", append(attrs, slog.String("error", err.Error()))...)
		return
	}
	s.logger.DebugContext(ctx, "sql", attrs...)
}
