# Implementation Plan: Фундамент БД — GORM + golang-migrate

Branch: none (git отключён в этом проекте)
Created: 2026-09-01

## Original Request
веха «Фундамент работы с БД» из .ai-factory/ROADMAP.md: GORM (*gorm.DB, пул соединений, хелпер транзакций), golang-migrate миграции, первый модуль dialog со сквозным CRUD через repository-паттерн. Архитектура — Structured Modules, см. .ai-factory/ARCHITECTURE.md.

## Settings
- Testing: yes
- Logging: standard  # INFO — подключение к БД, миграции; DEBUG для транзакций по желанию
- Docs: yes  # обязательный чекпоинт документации в /aif-implement после завершения

## Roadmap Linkage
Milestone: "Фундамент работы с БД"
Rationale: первый из двух последовательных планов этой вехи — закладывает инфраструктуру подключения к БД (GORM engine/пул, хелпер транзакций, golang-migrate) без доменных моделей. Модуль `dialog` со сквозным CRUD через repository-паттерн — отдельный следующий план той же вехи (`db-foundation-dialog-module`).

## Scope

В объёме:
- Зависимости: `gorm.io/gorm`, `gorm.io/driver/postgres`, `github.com/golang-migrate/migrate/v4`, `github.com/stretchr/testify`
- `internal/infrastructure/db` — `Open()` (engine, пул, ping, GORM-логгер поверх slog), `Close()`, хелпер транзакций `WithinTx()`
- Встроенные (`//go:embed`) миграции golang-migrate + baseline-миграция
- Подкоманды `api migrate up|down`
- Нефатальное подключение к БД при старте `cmd/api`
- Сервис `tests` в docker-compose (образ `golang`, профиль `tools`) для запуска тестов
- Тесты инфраструктуры БД против реального Postgres из docker-compose

Вне объёма (следующий план вехи):
- Модуль `dialog` (model / repository / service / handler), его миграция и CRUD-эндпоинты
- Валидация DTO, единый обработчик ошибок API — более поздние вехи

## Commit Plan
<!-- git.enabled = false — коммитов не будет; чекпоинты фиксируют логическую группировку -->
- **Чекпоинт 1** (после задач 1-4): "feat: add gorm engine/pool + transaction helper"
- **Чекпоинт 2** (после задач 5-6): "feat: setup golang-migrate with embedded migrations"
- **Чекпоинт 3** (после задач 7-8): "test: cover db infrastructure against real postgres"

## Tasks

### Phase 1: GORM (engine, пул, транзакции)

- [x] Task 1: Добавить зависимости через `go get` (в контейнере `golang:1.25`).
  - `gorm.io/gorm`, `gorm.io/driver/postgres` — ORM и драйвер.
  - `github.com/golang-migrate/migrate/v4` (+ драйверы `database/postgres` и `source/iofs`) — миграции.
  - `github.com/stretchr/testify` — ассерты в тестах.
  - Файлы: `go.mod`, `go.sum`.
  - Логирование: не требуется. Проверка: `go build ./...`, `go mod tidy` без ошибок.

- [x] Task 2: `internal/infrastructure/db/db.go` — подключение и пул (зависит от 1).
  - `Open(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*gorm.DB, error)`:
    - `gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{Logger: <адаптер на slog>})`
    - получить `*sql.DB` через `db.DB()`, настроить пул: `SetMaxOpenConns(10)`, `SetMaxIdleConns(5)`, `SetConnMaxLifetime(time.Hour)` (значения вынести в константы пакета)
    - `sqlDB.PingContext(ctx)` — вернуть обёрнутую ошибку при неудаче
  - Адаптер GORM-логгера: реализация `gorm/logger.Interface`, пишет в переданный `*slog.Logger` (уровни GORM → slog; SQL — на DEBUG).
  - `Close(db *gorm.DB) error` — закрыть `*sql.DB`.
  - Файлы: `internal/infrastructure/db/db.go`.
  - Логирование: INFO `"database connection established"` (с параметрами пула) при успехе; сам ping/ошибки — через возврат error, не логировать внутри.

- [x] Task 3: `internal/infrastructure/db/transaction.go` — хелпер транзакций (зависит от 2).
  - `WithinTx(ctx context.Context, db *gorm.DB, fn func(tx *gorm.DB) error) error` — оборачивает `db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return fn(tx) })`.
  - Транзакции инициируются в сервисном слое, операции внутри — через репозитории (см. `.ai-factory/rules/base.md`).
  - Файлы: `internal/infrastructure/db/transaction.go`.
  - Логирование: DEBUG `"tx begin"` / `"tx commit"` / `"tx rollback"` (с `error_type` при откате). Логгер брать из аргумента или из `slog.Default()`.

- [x] Task 4: Подключить БД к `cmd/api/main.go` (зависит от 2).
  - В `run()`: после `logging.Setup` вызвать `db.Open(ctx, cfg, logger)`.
    - При ошибке — `logger.Error("database unavailable at startup", ...)` и **продолжить** (на этой вехе ни один эндпоинт не зависит от БД; аналогично Bootstrap-подходу к нефатальным сбоям).
    - При успехе — `defer db.Close(gdb)`.
  - `*gorm.DB` пока никуда не передаётся (модулей нет) — только хранится в `run()`.
  - Файлы: `cmd/api/main.go`.
  - Логирование: INFO при успешном подключении (уже из Task 2), ERROR при неудаче.
<!-- Чекпоинт 1: задачи 1-4 -->

### Phase 2: golang-migrate

- [x] Task 5: Встроенные миграции + baseline (зависит от 1).
  - `migrations/000001_baseline.up.sql` — `CREATE EXTENSION IF NOT EXISTS "pgcrypto";` (для `gen_random_uuid()` в будущих модулях).
  - `migrations/000001_baseline.down.sql` — `DROP EXTENSION IF EXISTS "pgcrypto";`.
  - `internal/infrastructure/db/migrate.go`:
    - `//go:embed migrations/*.sql` — встроить SQL в бинарь. **Примечание:** `go:embed` не видит файлы выше пакета; вынести `embed.FS` в отдельный пакет `internal/migrations` (`migrations/embed.go` с `//go:embed *.sql`) и импортировать его из `db`.
    - `Migrate(cfg *config.Config, direction string) error` — `iofs` source из встроенной FS + `database/postgres`, применяет `Up()` / `Down()`; `errors.Is(err, migrate.ErrNoChange)` не считать ошибкой.
  - Файлы: `migrations/000001_baseline.up.sql`, `migrations/000001_baseline.down.sql`, `migrations/embed.go`, `internal/infrastructure/db/migrate.go`. Удалить `migrations/.gitkeep`.
  - Логирование: INFO `"migrations applied"` / `"migrations reverted"` с направлением; INFO `"no migration changes"` при `ErrNoChange`.

- [x] Task 6: Подкоманды `api migrate up|down` + проверка цикла (зависит от 4, 5).
  - В `cmd/api/main.go`: разобрать `os.Args` — `migrate up` / `migrate down` (по аналогии с уже существующей веткой `healthcheck`). Подкоманда: `config.Load()` → `logging.Setup()` → `db.Migrate(cfg, dir)` → exit 0/1.
  - `docker-compose.yml` не меняется — `docker compose run --rm app migrate up` работает через существующий `ENTRYPOINT`.
  - Файлы: `cmd/api/main.go`.
  - Проверка: `docker compose up -d postgres`; `docker compose run --rm app migrate up` → `migrate down` → `migrate up`; таблица `schema_migrations` в БД корректна.
<!-- Чекпоинт 2: задачи 5-6 -->

### Phase 3: тесты и сквозная проверка

- [x] Task 7: Сервис `tests` в docker-compose + тесты пакета `db` (зависит от 3, 5).
  - `docker-compose.yml`: сервис `tests` — `image: golang:1.25`, `profiles: ["tools"]`, `volumes: .:/src` + `gocache`, `working_dir: /src`, `env_file: .env`, `depends_on: postgres (service_healthy)`, `command: ["go", "test", "./..."]`. (Рантайм-образ `app` — distroless без Go, тесты в нём запустить нельзя.)
  - Именованный volume `gocache` для кэша модулей/сборки.
  - `internal/infrastructure/db/db_test.go`:
    - `Open()` возвращает рабочий `*gorm.DB`, ping проходит, пул настроен (`Stats().MaxOpenConnections`).
    - `WithinTx` коммитит при `nil` и откатывает при ошибке из `fn` (проверка через временную таблицу `CREATE TEMP TABLE` или запись/чтение в baseline-агностичной временной структуре).
  - Тесты берут `DATABASE_URL` из окружения (в сервисе `tests` — из `.env`, хост `postgres`).
  - Файлы: `docker-compose.yml`, `internal/infrastructure/db/db_test.go`.
  - Логирование: тесты используют `logging.Setup("DEBUG")`, чтобы в выводе были видны tx-логи при отладке падений.

- [x] Task 8: Сквозная проверка вехи-плана (зависит от 6, 7).
  - `docker compose up -d postgres` → дождаться `healthy`.
  - `docker compose run --rm app migrate up`.
  - `docker compose run --rm tests` (профиль подхватывается по имени сервиса) → все тесты зелёные.
  - `docker compose up -d --build app` → лог `app` показывает успешное `"database connection established"`.
  - `curl http://localhost:8090/health` → `200`.
  - `docker compose down` — volumes не трогать.
  - Файлы: нет новых, только проверка.
<!-- Чекпоинт 3: задачи 7-8 -->

## Документация (чекпоинт /aif-docs после Task 8)
- `docs/db.md` — GORM engine/пул, хелпер транзакций, golang-migrate (встроенные миграции, команды `migrate up|down`), запуск тестов через сервис `tests`.
- Обновить `docs/getting-started.md` (шаг миграций), `docs/configuration.md` (использование `DATABASE_URL`).
- Обновить `AGENTS.md` (новые пакеты, команды миграций и тестов) и `.ai-factory/ROADMAP.md` не трогать — веха закрывается только после второго плана (модуль `dialog`).
- Дописать `.ai-factory/journal/db-foundation.md` по ходу реализации.
