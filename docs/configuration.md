[← Быстрый старт](getting-started.md) · [Back to README](../README.md)

# Конфигурация

Все настройки читаются из переменных окружения (`.env` в корне проекта,
см. `.env.example`) через `internal/infrastructure/config`
(`Config`, `Load()`, пакеты `caarlos0/env` + `joho/godotenv`).

Отсутствие `.env` не является ошибкой: в контейнере переменные приходят
из окружения docker-compose.

## Переменные окружения

| Переменная | По умолчанию | Описание |
|------------|--------------|----------|
| `APP_NAME` | `ai-app-go` | Имя приложения (в логах) |
| `LOG_LEVEL` | `INFO` | Уровень логирования: `DEBUG` / `INFO` / `WARN` / `ERROR` (регистр не важен; неизвестное значение → `INFO`) |
| `HTTP_PORT` | `8080` | Порт, на котором сервис слушает внутри контейнера |
| `APP_HOST_PORT` | `8080` | Порт на хосте, на который проброшен `app` в docker-compose |
| `DATABASE_URL` | `postgres://aiapp:aiapp@postgres:5432/ai_app?sslmode=disable` | Подключение к PostgreSQL (GORM engine и миграции golang-migrate) |
| `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB` | `aiapp` / `aiapp` / `ai_app` | Учётные данные контейнера `postgres`; держать синхронно с `DATABASE_URL` |
| `REDIS_URL` | `redis://redis:6379/0` | Подключение к Redis (используется начиная с вехи фоновых задач) |
| `QDRANT_URL` | `http://qdrant:6333` | Подключение к Qdrant (используется начиная с вех RAG) |
| `OPENAI_API_KEY` | — (пусто) | Ключ OpenAI. Обязателен для эндпоинтов чата (`/dialogs/:id/messages`); без него они отвечают `502`, остальное приложение работает |
| `OPENAI_MODEL` | `gpt-4o-mini` | Имя chat-модели OpenAI для langchaingo |

`REDIS_URL`, `QDRANT_URL` на текущем этапе только читаются в `Config` —
ни один эндпоинт от них пока не зависит. `DATABASE_URL`, `OPENAI_API_KEY`,
`OPENAI_MODEL` — используются (БД и эндпоинты чата, см. [db.md](db.md) и
[dialog.md](dialog.md)).

## Хосты в Docker vs локально

Значения по умолчанию в `.env.example` рассчитаны на запуск через
`docker compose` — хосты (`postgres`, `redis`, `qdrant`) совпадают с
именами сервисов в `docker-compose.yml`. Для запуска без Docker
замените их на `localhost`.

## See Also

- [Быстрый старт](getting-started.md) — установка и запуск
- [Архитектура](../.ai-factory/ARCHITECTURE.md) — где используется конфигурация
