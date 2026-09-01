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
├── modules/                   # доменные модули: dialog, rag, tasks, moderation, files
│   └── <module>/{handler,service,repository,model,dto}/
└── infrastructure/            # config, db, redis, qdrant, llm, httpserver, logging
migrations/                    # SQL up/down (golang-migrate)
```

Текущее состояние: веха `Bootstrap проекта` завершена (2026-09-01).
Созданы `internal/infrastructure/{config,logging,httpserver}`, `cmd/api`
(с подкомандой `healthcheck`), `Dockerfile`, `docker-compose.yml`.
`internal/modules/` пока пуст — модули появляются с вехи «Фундамент
работы с БД».

## Ключевые точки входа

| Файл | Назначение |
|------|------------|
| `cmd/api/main.go` | сборка зависимостей, запуск сервера, graceful shutdown; подкоманда `healthcheck` для Docker HEALTHCHECK |
| `internal/infrastructure/config/config.go` | загрузка настроек из env / `.env` (`Config`, `Load()`) |
| `internal/infrastructure/logging/logging.go` | настройка `log/slog` (`Setup()`) |
| `internal/infrastructure/httpserver/server.go` | сборка Gin-движка, middleware `accessLog`, `/health` |
| `Dockerfile` | multi-stage сборка статического бинаря → distroless |
| `docker-compose.yml` | локальное окружение: app + PostgreSQL + Redis + Qdrant |

## Тесты и миграции — только через Docker

- Тесты: `docker compose up -d postgres`, затем
  `docker compose run --rm app go test ./...` (реальная БД, без моков).
- Миграции: команды уточняются на вехе «Фундамент работы с БД».
- На хосте Go не установлен — сборка и проверки идут через контейнер
  `golang` или образ приложения.

## Документация

| Документ | Путь | Описание |
|----------|------|----------|
| README | `README.md` | Лендинг-страница проекта |
| Быстрый старт | `docs/getting-started.md` | Установка, запуск через Docker и локально |
| Конфигурация | `docs/configuration.md` | Переменные окружения |
| DESCRIPTION | `.ai-factory/DESCRIPTION.md` | Спецификация проекта, стек |
| ARCHITECTURE | `.ai-factory/ARCHITECTURE.md` | Structured Modules — структура папок, правила зависимостей, примеры кода |
| Roadmap | `.ai-factory/ROADMAP.md` | Этапы разработки |
| Базовые правила | `.ai-factory/rules/base.md` | Конвенции именования, структура модулей, обработка ошибок |
| AI Factory config | `.ai-factory/config.yaml` | Конфигурация AI Factory (язык — ru, git выключен) |

## Правила для агента

- Доменные термины: «сообщения», «модерация проектов».
- Разбивай составные shell-команды на отдельные шаги вместо
  объединения через `&&`.
- Проект без git (`.git` отсутствует, `config.yaml` → `git.enabled: false`)
  — ветки не создаются, все изменения делаются напрямую в рабочей
  директории, чекпоинты в планах — только логические.
- Локальное окружение (БД, Redis, Qdrant, сам сервис) поднимается
  через Docker / docker compose, а не напрямую на хосте.
- Перед сдачей кода: `gofmt`/`goimports`, `go vet`, `go build ./...`,
  `go test ./...` (через контейнер).
- Комментарии и doc-комментарии — на русском языке.
