# Журнал реализации: Bootstrap проекта

План: `.ai-factory/plans/bootstrap-project.md`
Веха roadmap: «Bootstrap проекта»

## Общее

- На хосте Go не установлен — все команды (`go mod`, `go build`, `go vet`,
  `gofmt`, `go run`) выполнялись через контейнер `golang:1.25` с
  bind-mount проекта и persistent volume `aiappgo-gocache:/go` для кэша
  модулей и сборки.

## Task 1 — Go-модуль и структура каталогов

- `go mod init github.com/flying-develop/ai-app-go`.
- Каталоги: `cmd/api/`, `internal/infrastructure/{config,logging,httpserver}/`,
  `internal/modules/` (заглушка `doc.go`), `migrations/` (`.gitkeep`).
- Зависимости: `github.com/gin-gonic/gin`, `github.com/caarlos0/env/v11`,
  `github.com/joho/godotenv`.
- ⚠️ **Отклонение от плана по версии Go:** план указывал `golang:1.23`,
  но `gin-gonic/gin v1.12.0` требует `go >= 1.25.0`. Модуль и Dockerfile
  переведены на Go 1.25; `go.mod` → `go 1.25.14`. Обновлены упоминания
  версии в плане и `.ai-factory/`-документах.

## Task 2 — Конфигурация (`internal/infrastructure/config`)

- Структура `Config` с env-тегами (`caarlos0/env/v11`): `AppName`,
  `HTTPPort`, `LogLevel` + заготовки `DatabaseURL`, `RedisURL`,
  `QdrantURL`, `OpenAIAPIKey` (нигде не используются на этой вехе).
- `Load()`: `godotenv.Load()` (отсутствие `.env` — не ошибка,
  проверяется через `errors.Is(err, os.ErrNotExist)`), затем
  `env.ParseAs[Config]()`; ошибки оборачиваются с контекстом.
- Дефолт учётных данных БД — `aiapp:aiapp` (не `kwork`).

## Task 3 — Логирование (`internal/infrastructure/logging`)

- `Setup(level string) *slog.Logger`: `slog.NewTextHandler(os.Stdout)`
  (формат `key=value`), уровень из `LOG_LEVEL` (`DEBUG`/`INFO`/`WARN`/
  `ERROR`, регистронезависимо; неизвестное → `INFO`), `slog.SetDefault`.
- Логирует `INFO "logging initialized" level=<level>` в конце.

## Task 4 — HTTP-сервер и точка входа

- `httpserver.New(cfg, logger)`: `gin.New()` + middleware `accessLog`
  (метод/путь/статус/latency/ip через slog) + `gin.Recovery()`.
  `GET /health` → `200 {"status":"ok"}` (+ DEBUG-лог). `gin.SetMode`:
  debug только при `LOG_LEVEL=DEBUG`, иначе release.
- `cmd/api/main.go`: `config.Load()` → `logging.Setup()` →
  `httpserver.New()` → `http.Server` (с `ReadHeaderTimeout`), запуск в
  горутине, graceful shutdown по `SIGINT`/`SIGTERM` через
  `signal.NotifyContext` + `srv.Shutdown` (таймаут 10s).
- ➕ **Дополнение к плану:** подкоманда `api healthcheck` — GET /health к
  `127.0.0.1:$HTTP_PORT`, exit 0/1. Нужна для `HEALTHCHECK` в
  distroless-образе, где нет `curl`/`wget`.
- Проверка: `go run ./cmd/api` в контейнере → `curl /health` → `200`,
  `{"status":"ok"}`, `/missing` → `404`. Логи структурированные.

## Task 5 — Dockerfile

- Multi-stage: `golang:1.25` (build, `CGO_ENABLED=0`, `-trimpath -ldflags="-s -w"`)
  → `gcr.io/distroless/static-debian12:nonroot` (runtime).
- `HEALTHCHECK CMD ["/app/api", "healthcheck"]`.
- `.dockerignore`: `.git`, `.claude`, `.ai-factory`, `*.md`, `.env*`
  (кроме `.env.example`), локальные бинарники.
- Проверка: `docker build` ок; `docker run` → `/health` `200`,
  `docker exec ... /app/api healthcheck` → exit 0. Тестовый образ удалён.

## Task 6 — docker-compose.yml + .env.example

- Сервисы: `app` (build), `postgres:16` (healthcheck `pg_isready`),
  `redis:7`, `qdrant/qdrant`. Volumes: `pg_data`, `redis_data`,
  `qdrant_data`. `app.depends_on`: postgres `service_healthy`,
  redis/qdrant `service_started`. Healthcheck `app` — `["CMD", "/app/api", "healthcheck"]`.
- ⚠️ **Отклонение от плана по портам:** host-порт 8080 уже занят другим
  контейнером на машине. Разведены две переменные: `HTTP_PORT` (порт
  внутри контейнера, всегда 8080 в compose) и `APP_HOST_PORT` (маппинг
  наружу, дефолт 8080). В локальном `.env` выставлен `APP_HOST_PORT=8090`.
- `.env.example` — все переменные, хосты под имена сервисов compose.

## Task 7 — Полный запуск через docker compose

- `docker compose up -d --build` — 4 сервиса, `app` и `postgres` →
  `healthy`, redis/qdrant → `running`.
- `curl http://localhost:8090/health` → `200`, `{"status":"ok"}`.
- Логи `app` — структурированные, уровень `INFO`, виден внутренний
  healthcheck-запрос от `127.0.0.1`.
- `docker compose down` — стек удалён, volumes оставлены.

**Итог вехи:** локальное окружение поднимается одной командой
(`docker compose up -d --build`), сервис на Gin отвечает на `/health`,
логирование и конфиг из env работают. База готова для вехи «Фундамент
работы с БД».
