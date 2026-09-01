# Implementation Plan: Bootstrap проекта

Branch: none (git отключён в этом проекте)
Created: 2026-09-01

## Original Request
веха «Bootstrap проекта» из .ai-factory/ROADMAP.md: скелет Gin-приложения (go mod, cmd/api/main.go, internal/infrastructure/config на env), структурированное логирование (log/slog, уровень через LOG_LEVEL), /health-эндпоинт; Dockerfile + docker-compose (app + PostgreSQL + Redis + Qdrant). Архитектура — Structured Modules, см. .ai-factory/ARCHITECTURE.md. Аналогии — с Laravel.

## Settings
- Testing: no
- Logging: standard  # INFO — ключевые события (старт сервера, shutdown); DEBUG для /health по желанию
- Docs: yes  # обязательный чекпоинт документации в /aif-implement после завершения

## Roadmap Linkage
Milestone: "Bootstrap проекта"
Rationale: план реализует весь объём этой вехи из .ai-factory/ROADMAP.md — скелет Gin-приложения, конфиг из env, структурированное логирование, /health, Dockerfile и docker-compose (app + postgres + redis + qdrant). Доменных модулей и работы с БД здесь нет — это следующая веха.

## Scope

В объёме:
- `go.mod` + структура каталогов `cmd/` и `internal/` по ARCHITECTURE.md
- `internal/infrastructure/config` — загрузка настроек из env / `.env`
- `internal/infrastructure/logging` — `log/slog` c уровнем из `LOG_LEVEL`
- `internal/infrastructure/httpserver` — сборка Gin-движка, recovery, `/health`
- `cmd/api/main.go` — сборка зависимостей, запуск сервера, graceful shutdown
- `Dockerfile` (multi-stage) + `docker-compose.yml` (app + postgres + redis + qdrant) + `.env.example` + `.dockerignore`

Вне объёма (следующие вехи):
- GORM engine/session, миграции golang-migrate, доменные модули — веха «Фундамент работы с БД»
- langchaingo, asynq-воркер (`cmd/worker`), Qdrant-клиент — контейнеры поднимаются, но код-интеграции нет
- Тесты — начинаются с вехи «Фундамент работы с БД»

## Commit Plan
<!-- git.enabled = false — коммитов не будет; чекпоинты фиксируют логическую группировку -->
- **Чекпоинт 1** (после задач 1-4): "feat: bootstrap gin app skeleton with config, logging and /health"
- **Чекпоинт 2** (после задач 5-7): "feat: add docker-compose stack (app, postgres, redis, qdrant)"

## Tasks

### Phase 1: Скелет приложения

- [x] Task 1: Инициализировать Go-модуль и структуру каталогов.
  - Команды: `go mod init github.com/flying-develop/ai-app-go` (имя модуля уточнить у пользователя при реализации, дефолт — этот).
  - Создать каталоги по `.ai-factory/ARCHITECTURE.md`: `cmd/api/`, `internal/infrastructure/{config,logging,httpserver}/`, `internal/modules/` (с пустым `doc.go`), `migrations/` (пустой).
  - Зависимости через `go get`: `github.com/gin-gonic/gin`, `github.com/caarlos0/env/v11`, `github.com/joho/godotenv`. (`log/slog` — стандартная библиотека.)
  - Файлы: `go.mod`, `go.sum`, каталоги-заглушки.
  - Проверка: `go build ./...` без ошибок.

- [x] Task 2: Конфигурация приложения из env / `.env` (зависит от 1).
  - `internal/infrastructure/config/config.go`: структура `Config` с полями и env-тегами (`github.com/caarlos0/env/v11`):
    - `AppName` (`APP_NAME`, дефолт `ai-app-go`)
    - `HTTPPort` (`HTTP_PORT`, дефолт `8080`)
    - `LogLevel` (`LOG_LEVEL`, дефолт `INFO`)
    - `DatabaseURL` (`DATABASE_URL`, дефолт `postgres://aiapp:aiapp@postgres:5432/ai_app?sslmode=disable`) — заготовка под вехи БД
    - `RedisURL` (`REDIS_URL`, дефолт `redis://redis:6379/0`) — заготовка под asynq
    - `QdrantURL` (`QDRANT_URL`, дефолт `http://qdrant:6333`) — заготовка под RAG
    - `OpenAIAPIKey` (`OPENAI_API_KEY`, без дефолта) — заготовка под вехи LLM
  - Функция `Load() (*Config, error)`: `godotenv.Load()` (не падать, если `.env` нет — в контейнере переменные из окружения), затем `env.ParseAs[Config]()`. Обёрнутая ошибка при неудаче парсинга.
  - Файлы: `internal/infrastructure/config/config.go`.
  - Логирование: не требуется внутри `Load()` (логгер ещё не поднят). Валидацию критичных полей отложить до вех, где они используются.

- [x] Task 3: Структурированное логирование через `log/slog` (зависит от 2).
  - `internal/infrastructure/logging/logging.go`: `Setup(level string) *slog.Logger` — парсит `LOG_LEVEL` (`DEBUG`/`INFO`/`WARN`/`ERROR`, регистронезависимо, дефолт `INFO` при неизвестном значении), создаёт `slog.NewTextHandler(os.Stdout, ...)` (key=value формат), ставит его через `slog.SetDefault`, возвращает логгер.
  - Хелпер маппинга строки уровня в `slog.Level`.
  - Файлы: `internal/infrastructure/logging/logging.go`.
  - Логирование: одна запись `INFO "logging initialized" level=<level>` в конце `Setup`.

- [x] Task 4: HTTP-сервер, `/health` и точка входа (зависит от 3).
  - `internal/infrastructure/httpserver/server.go`:
    - `New(cfg *config.Config, log *slog.Logger) *gin.Engine` — `gin.New()` (не `gin.Default()`), подключить `gin.Recovery()` и middleware структурного лог-доступа (метод, путь, статус, latency — через `slog`).
    - Зарегистрировать `GET /health` → `200 {"status":"ok"}` (DEBUG-лог на каждый вызов).
    - `gin.SetMode` по `LOG_LEVEL`/окружению (release, если не DEBUG).
  - `cmd/api/main.go`:
    - `config.Load()` → `logging.Setup()` → `httpserver.New()`.
    - `http.Server{Addr: ":"+cfg.HTTPPort, Handler: engine}`; запуск в горутине.
    - Graceful shutdown: `signal.NotifyContext` на `SIGINT`/`SIGTERM`, `srv.Shutdown(ctx)` с таймаутом 10s.
    - INFO-логи: `"server starting" addr=...`, `"server stopped"`.
  - Файлы: `internal/infrastructure/httpserver/server.go`, `cmd/api/main.go`.
  - Логирование: INFO на старт/остановку сервера; DEBUG на каждый `/health`.
  - Проверка: `go run ./cmd/api`, затем `curl -i localhost:8080/health` → `200`, `{"status":"ok"}`; SIGTERM → чистый shutdown-лог.
<!-- Чекпоинт 1: задачи 1-4 -->

### Phase 2: Docker-окружение

- [x] Task 5: Dockerfile для приложения (зависит от 4).
  - Multi-stage:
    - `build`: `golang:1.25` → `COPY go.mod go.sum` → `go mod download` (кэш-слой) → `COPY . .` → `CGO_ENABLED=0 go build -o /out/api ./cmd/api`.
    - `runtime`: `gcr.io/distroless/static` или `alpine` → `COPY --from=build /out/api /app/api` → `EXPOSE 8080` → `ENTRYPOINT ["/app/api"]`.
  - `.dockerignore`: `.git`, `.ai-factory`, `.claude`, `*.md`, `.env`, локальные бинарники.
  - Файлы: `Dockerfile`, `.dockerignore`.
  - Проверка: `docker build -t ai-app-go:bootstrap-test .` собирается; `docker run --rm -p 8080:8080 ai-app-go:bootstrap-test` + `curl /health` → `200`. Тестовый образ удалить после проверки.

- [x] Task 6: docker-compose.yml + .env.example (зависит от 5).
  - `docker-compose.yml`, сервисы:
    - `app`: `build: .`, `env_file: .env`, `ports: 8080:8080`, `depends_on` postgres (`service_healthy`), redis/qdrant (`service_started`), healthcheck на `/health`.
    - `postgres:16` — `POSTGRES_USER/PASSWORD/DB` из env, volume `pg_data`, healthcheck `pg_isready`.
    - `redis:7` — volume `redis_data`.
    - `qdrant/qdrant` — volume `qdrant_data`.
    - volumes: `pg_data`, `redis_data`, `qdrant_data`.
  - `.env.example`: `APP_NAME`, `HTTP_PORT`, `LOG_LEVEL`, `DATABASE_URL` (хост `postgres`), `POSTGRES_USER/PASSWORD/DB` (синхронно с `DATABASE_URL`), `REDIS_URL` (хост `redis`), `QDRANT_URL` (хост `qdrant`), `OPENAI_API_KEY=`.
  - Файлы: `docker-compose.yml`, `.env.example`.
  - Проверка: `cp .env.example .env`, `docker compose config` резолвится без ошибок.

- [x] Task 7: Полный запуск через docker compose (зависит от 6).
  - `docker compose up -d --build` — все 4 сервиса поднимаются; `app` и `postgres` — `healthy`.
  - `curl http://localhost:8080/health` → `200`, `{"status":"ok"}`.
  - `docker compose logs app` — структурированные записи, уровень из `LOG_LEVEL`.
  - `docker compose down` — стек останавливается; volumes не трогать.
  - Файлы: нет новых, только проверка.
<!-- Чекпоинт 2: задачи 5-7 -->

## Документация (чекпоинт /aif-docs после Task 7)
- `README.md` — лендинг (быстрый старт через docker compose, ссылки на `.ai-factory/`).
  README описывает сервис как самостоятельный продукт (Go + Gin + langchaingo:
  диалоги с LLM, RAG, обработка задач, модерация). Ограничения по формулировкам
  в документации — см. память проекта.
- `docs/getting-started.md`, `docs/configuration.md`.
- Обновить `AGENTS.md` (структура проекта, точки входа) и `.ai-factory/ROADMAP.md` (веха «Bootstrap проекта» → `[x]`, запись в «Завершено»).
- Завести `.ai-factory/journal/bootstrap-project.md` по ходу реализации.
