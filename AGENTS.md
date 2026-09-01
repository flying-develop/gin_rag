# AGENTS.md

> Поддерживай этот файл в актуальном состоянии по мере роста
> структуры проекта.

## Обзор проекта

AI-сервис на Go: диалоги с LLM, RAG по статьям, конвейер обработки
задач и модерация контента. HTTP API на Gin + фоновые воркеры на asynq.
Подробности — в `.ai-factory/DESCRIPTION.md`. Разработка поэтапная,
порядок вех — в `.ai-factory/ROADMAP.md`.

## Технологический стек

- **Язык:** Go 1.23+
- **Веб-фреймворк:** Gin
- **AI-оркестрация:** langchaingo (`github.com/tmc/langchaingo`);
  конвейеры состояний — вручную как state machine
- **База данных:** PostgreSQL
- **ORM / доступ к БД:** GORM + repository-паттерн
- **Миграции:** golang-migrate, SQL up/down файлы
- **Векторная БД:** Qdrant
- **Фоновые задачи:** asynq + Redis
- **Конфигурация:** env через `caarlos0/env` + `joho/godotenv`
- **Логирование:** `log/slog`, уровень через `LOG_LEVEL`
- **Тесты:** `testing` + `stretchr/testify`
- **Окружение разработки:** Docker / docker compose

## Структура проекта

Целевая структура — Structured Modules (Technical Layer), подробности в
`.ai-factory/ARCHITECTURE.md`:

```
cmd/
├── api/main.go                # точка входа HTTP-сервиса
└── worker/main.go             # asynq-воркер (веха фоновых задач)
internal/
├── apperr/                    # общий словарь доменных ошибок (Kind → HTTP-код)
├── modules/                   # доменные модули: dialog, rag, tasks, moderation, files
│   └── <module>/{handler,service,repository,model,dto}/
└── infrastructure/            # config, db, redis, qdrant, llm, httpserver, logging
migrations/                    # SQL up/down (golang-migrate) + embed.go
```

Текущее состояние: вехи `Bootstrap проекта`, `Фундамент работы с БД`,
`Диалоги с LLM (базовый чат)` завершены (2026-09-01). Есть:
- `internal/infrastructure/{config,logging,httpserver,db,llm}`
  (`llm` — langchaingo/OpenAI за `llms.Model`; `llm/llmtest` — fake для тестов)
- `internal/apperr` — доменные ошибки (+ `KindUpstream` → 502)
- `internal/modules/dialog/` — CRUD `/api/v1/dialogs` + чат `/dialogs/:id/messages`
- `cmd/api` — подкоманды `healthcheck`, `migrate up|down`
- `Dockerfile`, `docker-compose.yml` (+ сервис `tests`, профиль `tools`)

## Ключевые точки входа

| Файл | Назначение |
|------|------------|
| `cmd/api/main.go` | точка входа; `dispatch()`: подкоманды `healthcheck`, `migrate up\|down`, иначе HTTP-сервер |
| `internal/infrastructure/config/config.go` | загрузка настроек из env / `.env` (`Config`, `Load()`) |
| `internal/infrastructure/logging/logging.go` | настройка `log/slog` (`Setup()`) |
| `internal/infrastructure/httpserver/server.go` | сборка Gin-движка, middleware `accessLog`, `/health` |
| `internal/infrastructure/db/db.go` | GORM engine, пул соединений, `Open()`/`Close()` |
| `internal/infrastructure/db/transaction.go` | `WithinTx()` (tx кладётся в ctx) + `Conn(ctx, fallback)` |
| `internal/infrastructure/db/migrate.go` | `Migrate(cfg, up\|down)` поверх golang-migrate |
| `internal/infrastructure/httpserver/errors.go` | `errorHandler` — `apperr.Kind` → HTTP-код |
| `internal/infrastructure/llm/llm.go` | `New(cfg) (llms.Model, error)` — chat-модель langchaingo (OpenAI) |
| `internal/apperr/apperr.go` | доменные ошибки (`NotFound`/`Validation`/`Conflict`/`Upstream`/`Internal`) |
| `internal/modules/dialog/` | модуль dialog: CRUD + `ChatService` (сообщения, вызов LLM) |
| `migrations/` | SQL up/down + `embed.go` (встроены в бинарь через `go:embed`) |
| `Dockerfile` | multi-stage сборка статического бинаря → distroless |
| `docker-compose.yml` | локальное окружение: app + PostgreSQL + Redis + Qdrant + `tests` (профиль `tools`) |

## Тесты, сборка и миграции — только через Docker

- На хосте Go нет. Сборка/vet/fmt: `docker run --rm -v "$PWD":/app -w /app -e GOFLAGS=-buildvcs=false golang:1.25 sh -c "gofmt -l . && go vet ./... && go build ./..."`.
- Тесты: `docker compose up -d postgres`, затем `docker compose run --rm tests`
  (образ `golang`; рантайм-образ `app` — distroless без Go). Реальная БД, без моков.
  Прогон последовательный (`-p 1`) — пакеты БД-тестов делят одну базу.
- Миграции: `docker compose run --rm app migrate up` / `migrate down`.
  После изменения кода подкоманд — сначала `docker compose build app`.

## Документация

| Документ | Путь | Описание |
|----------|------|----------|
| README | `README.md` | Лендинг-страница проекта |
| Быстрый старт | `docs/getting-started.md` | Установка, запуск через Docker и локально |
| Конфигурация | `docs/configuration.md` | Переменные окружения |
| БД и миграции | `docs/db.md` | GORM engine/пул, `WithinTx`, golang-migrate, тесты |
| Модуль dialog | `docs/dialog.md` | CRUD по диалогам, структура модуля, эндпоинты |
| DESCRIPTION | `.ai-factory/DESCRIPTION.md` | Спецификация проекта, стек |
| ARCHITECTURE | `.ai-factory/ARCHITECTURE.md` | Structured Modules — структура папок, правила зависимостей, примеры кода |
| Roadmap | `.ai-factory/ROADMAP.md` | Этапы разработки |
| Базовые правила | `.ai-factory/rules/base.md` | Конвенции именования, структура модулей, обработка ошибок |
| AI Factory config | `.ai-factory/config.yaml` | Конфигурация AI Factory (язык — ru, git выключен) |

## Правила для агента

- Доменные термины: «сообщения», «модерация проектов».
- Разбивай составные shell-команды на отдельные шаги вместо
  объединения через `&&`.
- AI Factory работает в режиме без git (`config.yaml` → `git.enabled: false`):
  `/aif-plan` не создаёт ветки, чекпоинты в планах — только логические.
  Репозиторий git при этом существует (`master`) — коммиты делает
  пользователь вручную, пока `git.enabled` не переключён.
- Локальное окружение (БД, Redis, Qdrant, сам сервис) поднимается
  через Docker / docker compose, а не напрямую на хосте.
- Перед сдачей кода: `gofmt`/`goimports`, `go vet`, `go build ./...`,
  `go test ./...` (через контейнер).
- Комментарии и doc-комментарии — на русском языке.
