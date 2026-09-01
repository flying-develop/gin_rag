# ai-app-go

AI-сервис на Go: диалоги с LLM, RAG по статьям, конвейер обработки задач
и модерация контента. HTTP API на [Gin](https://gin-gonic.com/), фоновые
задачи на [asynq](https://github.com/hibiken/asynq), AI-оркестрация через
[langchaingo](https://github.com/tmc/langchaingo).

Возможности вводятся поэтапно — см. [roadmap](.ai-factory/ROADMAP.md).

## Быстрый старт

```bash
cp .env.example .env
docker compose up -d --build
docker compose run --rm app migrate up
curl http://localhost:8080/health
# {"status":"ok"}
```

Если порт 8080 на хосте занят — поменяйте `APP_HOST_PORT` в `.env`.

Подробности — в [docs/getting-started.md](docs/getting-started.md).

## Возможности (на текущем этапе)

- **HTTP-сервис на Gin** — скелет с `/health`-эндпоинтом и graceful shutdown.
- **Конфигурация через переменные окружения** — единый `.env`, типизированный `Config`.
- **Структурированное логирование** — `log/slog`, уровень через `LOG_LEVEL`.
- **PostgreSQL через GORM** — engine/пул соединений, хелпер транзакций, миграции golang-migrate (встроены в бинарь, `app migrate up|down`).
- **Локальное окружение в Docker** — `app` + PostgreSQL + Redis + Qdrant одной командой.

Остальное (диалоги с LLM, RAG, task pipeline, модерация) появляется
поэтапно — см. [roadmap](.ai-factory/ROADMAP.md).

## Документация

| Раздел | Описание |
|--------|----------|
| [Быстрый старт](docs/getting-started.md) | Установка, запуск через Docker и локально |
| [Конфигурация](docs/configuration.md) | Переменные окружения |
| [БД и миграции](docs/db.md) | GORM engine/пул, golang-migrate, тесты |
| [Архитектура](.ai-factory/ARCHITECTURE.md) | Паттерн Structured Modules, структура папок, правила зависимостей |
| [Описание проекта](.ai-factory/DESCRIPTION.md) | Цели, стек |
| [Roadmap](.ai-factory/ROADMAP.md) | Этапы разработки |

## Лицензия

Личный проект, без формальной лицензии.
