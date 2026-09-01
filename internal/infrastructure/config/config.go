// Package config загружает настройки приложения из переменных окружения
// (и опционального файла .env). Аналог config/*.php + .env в Laravel:
// единый типизированный объект Config вместо разрозненных вызовов
// config('...').
package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// Config — центральные настройки приложения.
//
// Часть полей (DatabaseURL, RedisURL, QdrantURL, OpenAIAPIKey) — заготовки
// под будущие вехи roadmap: на этапе Bootstrap они читаются, но нигде не
// используются. Валидация таких полей откладывается до вех, где они
// действительно нужны.
type Config struct {
	// AppName — имя приложения (используется в логах и метаданных).
	AppName string `env:"APP_NAME" envDefault:"ai-app-go"`

	// HTTPPort — порт HTTP-сервера Gin.
	HTTPPort string `env:"HTTP_PORT" envDefault:"8080"`

	// LogLevel — уровень логирования: DEBUG | INFO | WARN | ERROR
	// (регистр не важен). Неизвестное значение трактуется как INFO.
	LogLevel string `env:"LOG_LEVEL" envDefault:"INFO"`

	// DatabaseURL — строка подключения к PostgreSQL. Заготовка под веху
	// «Фундамент работы с БД».
	DatabaseURL string `env:"DATABASE_URL" envDefault:"postgres://aiapp:aiapp@postgres:5432/ai_app?sslmode=disable"`

	// RedisURL — подключение к Redis. Заготовка под вехи фоновых задач (asynq).
	RedisURL string `env:"REDIS_URL" envDefault:"redis://redis:6379/0"`

	// QdrantURL — подключение к Qdrant. Заготовка под вехи RAG.
	QdrantURL string `env:"QDRANT_URL" envDefault:"http://qdrant:6333"`

	// OpenAIAPIKey — ключ OpenAI. Заготовка под вехи «Диалоги с LLM».
	OpenAIAPIKey string `env:"OPENAI_API_KEY"`
}

// Load читает .env (если файл присутствует) и разбирает переменные
// окружения в Config.
//
// Отсутствие .env не является ошибкой: в контейнере переменные приходят
// из окружения docker-compose, а файла .env там нет.
func Load() (*Config, error) {
	// godotenv.Load не перезаписывает уже установленные переменные
	// окружения — приоритет у реального окружения, .env только заполняет
	// пропуски при локальном запуске.
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		// Единственный ожидаемый случай — файла нет. Любую другую ошибку
		// (битый синтаксис .env) полезно показать.
		return nil, fmt.Errorf("config: чтение .env: %w", err)
	}

	cfg, err := env.ParseAs[Config]()
	if err != nil {
		return nil, fmt.Errorf("config: разбор переменных окружения: %w", err)
	}

	return &cfg, nil
}

// isNotExist сообщает, вызвана ли ошибка отсутствием файла .env.
func isNotExist(err error) bool {
	// godotenv возвращает *os.PathError с обёрнутым syscall.ENOENT;
	// os.IsNotExist разбирается с обоими вариантами обёрток.
	return err != nil && (os.IsNotExist(err))
}
