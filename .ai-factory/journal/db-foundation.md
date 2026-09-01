# Журнал реализации: Фундамент работы с БД

Веха roadmap: «Фундамент работы с БД»
Планы вехи:
- `.ai-factory/plans/db-foundation-gorm-migrate.md` — GORM + golang-migrate (реализован, 8/8)
- Следующий план (не начат): модуль `dialog` со сквозным CRUD через repository-паттерн

## Общее

- Go на хосте нет — сборка/vet/fmt через контейнер `golang:1.25`
  (volume `aiappgo-gocache:/go`). Тесты — через сервис compose `tests`.

## План 1: GORM + golang-migrate

### Task 1 — Зависимости

- `go get gorm.io/gorm gorm.io/driver/postgres github.com/golang-migrate/migrate/v4 github.com/stretchr/testify`.
- Драйвер БД для GORM — `pgx/v5` (тянется `gorm.io/driver/postgres`).
- ⚠️ **`error obtaining VCS status`**: в проекте появился `.git` (создан
  извне). `go build` в контейнере от root спотыкается о dubious ownership.
  Решение: `-buildvcs=false` в командах сборки в контейнере. В Docker-образе
  проблемы нет — `.dockerignore` исключает `.git`.

### Task 2 — `internal/infrastructure/db/db.go`

- `Open(ctx, cfg, logger) (*gorm.DB, error)`: `gorm.Open(postgres.Open(url))`
  с `SkipDefaultTransaction: true`, пул `SetMaxOpenConns(10)` /
  `SetMaxIdleConns(5)` / `SetConnMaxLifetime(time.Hour)` (константы пакета),
  `PingContext`. `Close(*gorm.DB) error`.
- `gorm_logger.go` — адаптер `logger.Interface` GORM → slog: Info/Warn/Error
  на соответствующие уровни, `Trace` → DEBUG `"sql"` (ERROR при ошибке
  запроса, `ErrRecordNotFound` не считается ошибкой).
- 🐛 **nil-логгер**: тесты вызывают `Open(ctx, cfg, nil)` → паника в
  `logger.Info`. Добавлен guard `if logger == nil { logger = slog.Default() }`
  и в `Open`, и в `newSlogLogger`.

### Task 3 — `internal/infrastructure/db/transaction.go`

- `WithinTx(ctx, db, fn)` — обёртка `db.WithContext(ctx).Transaction(...)`.
  DEBUG-логи `"tx begin"` / `"tx commit"` / `"tx rollback"` (+ `error_type`
  через `%T`). Транзакции инициируются в сервисах, операции — через
  репозитории (правило проекта).

### Task 4 — Подключение БД в `cmd/api/main.go`

- `main` переписан на `dispatch(os.Args[1:])`: подкоманды `healthcheck`,
  `migrate up|down`, иначе `run()`.
- В `run()`: `db.Open(ctx, cfg, logger)` после `logging.Setup`. При ошибке —
  ERROR `"database unavailable at startup"` и продолжаем (эндпоинтов на
  БД пока нет). При успехе — `defer db.Close`.

### Task 5 — Встроенные миграции

- `migrations/embed.go` (`package migrations`, `//go:embed *.sql` → `FS`) —
  отдельный пакет в корне, т.к. `go:embed` не видит файлы выше пакета.
- `migrations/000001_baseline.{up,down}.sql` — `CREATE/DROP EXTENSION pgcrypto`.
- `internal/infrastructure/db/migrate.go`: `Migrate(cfg, direction)` через
  `iofs` source + blank-import `database/postgres`. `migrate.ErrNoChange`
  → INFO `"no migration changes"`, не ошибка. Хелпер `migrateDatabaseURL`
  нормализует `postgresql://` → `postgres://` (migrate понимает только
  вторую схему).

### Task 6 — Подкоманды `migrate up|down`

- Первый прогон `docker compose run --rm app migrate up` **завис в
  режиме сервера**: образ `app` был собран ДО добавления подкоманды
  `migrate`, старый бинарь падал в `run()`. Решение: `docker compose build app`
  перед запуском.
- После пересборки: `migrate up` → `schema_migrations.version=1`,
  `pgcrypto` установлен; повторный `up` → `no migration changes`;
  `down` → таблица пуста, расширение снято. Цикл чистый.

### Task 7 — Сервис `tests` + тесты пакета `db`

- `docker-compose.yml`: сервис `tests` (`image: golang:1.25`,
  `profiles: ["tools"]`, `.:/src` + volume `gocache`, `env_file: .env`,
  `DATABASE_URL` собирается из `POSTGRES_*`, `depends_on: postgres healthy`,
  `command: ["go","test","./..."]`). Рантайм-образ `app` distroless — Go в
  нём нет, тесты только в отдельном контейнере.
- `db_test.go` (пакет `db_test`, реальный Postgres): `TestOpen_PingAndPool`
  (ping + `Stats().MaxOpenConnections == 10`), `TestWithinTx_CommitsOnSuccess`,
  `TestWithinTx_RollsBackOnError` (через таблицу `tx_test`, TRUNCATE в
  setup, DROP в cleanup). 3/3 passed.

### Task 8 — Сквозная проверка

- `docker compose up -d postgres` → healthy; `migrate up`;
  `docker compose run --rm tests` → зелёные; `docker compose up -d --build app`
  → лог `"database connection established" max_open_conns=10 ...`;
  `curl :8090/health` → `200`; `docker compose down`.

**Итог плана:** инфраструктура БД готова — GORM engine/пул, `WithinTx`,
встроенные миграции golang-migrate с подкомандами `migrate up|down`,
тесты против реального Postgres. Моделей и репозиториев ещё нет — это
следующий план вехи (`dialog`-модуль).
