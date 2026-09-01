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

## План 2: Модуль dialog — сквозной CRUD

### Task 1 — Миграция `000002_create_dialogs`

- `dialogs` (id bigserial, user_id bigint, title text, created_at/updated_at
  timestamptz) + `idx_dialogs_user_id`. up/down.

### Task 2 — Модель и DTO

- `model/dialog.go` — `Dialog` + `TableName()`.
- `dto/dialog.go` — `CreateDialogRequest` (`user_id` required, `title` max=200),
  `UpdateDialogRequest` (`*string` title), `DialogResponse` + мапперы.

### Task 3 — Доменные ошибки и порт репозитория

- ⚠️ **Отклонение от плана (к лучшему):** вместо `errors.New` и импорта
  `dialog/service` в `httpserver` заведён нейтральный пакет
  `internal/apperr` (Kind: NotFound/Validation/Conflict/Internal, тип
  `*Error`, хелперы). И модули, и `httpserver` зависят от него — правило
  «repository/model не зависят от внешних слоёв» не нарушается.
- `service/errors.go`: `ErrDialogNotFound = apperr.NotFound("dialog not found")`.
- `service/ports.go`: интерфейс `DialogRepository` на стороне потребителя.
  Метод `WithTx` **не добавлен** — транзакции прозрачны через ctx.

### Task 4 — Репозиторий

- `repository/dialog_repository.go` — Create/FindByID/List/Update/Delete.
- ⚠️ **Отклонение:** вместо `r.db.WithContext(ctx)` метод `r.conn(ctx)`
  через новый хелпер `db.Conn(ctx, fallback)` — достаёт активную
  транзакцию из ctx, иначе базовое подключение. Это исключило импорт
  `service` в `repository` (не было бы восходящей зависимости) и
  необходимость `WithTx`-метода в интерфейсе.
- `db.WithinTx` переписан: сигнатура `func(ctx context.Context) error`,
  кладёт `*gorm.DB` транзакции в ctx (ключ `txContextKey`). Тесты
  `db_test.go` из плана 1 обновлены под новую сигнатуру.

### Task 5 — Сервис

- `service/dialog_service.go` — `DialogService{repo, db, log}`.
  `Update` — внутри `db.WithinTx(ctx, s.db, func(txCtx) {...})`, репозиторий
  подхватывает транзакцию из `txCtx`.
- INFO-логи `dialog created/updated/deleted` с `dialog_id`.

### Task 6 — Error-middleware

- `httpserver/errors.go` — `errorHandler(logger)`: `apperr.KindOf(err)` →
  HTTP-код (404/422/409/500). 5xx → ERROR-лог + `{"error":"internal error"}`;
  4xx → `{"error": apperr.ClientMessage(err)}`. Подключён в цепочку
  middleware в `server.go`.

### Task 7 — Хендлеры

- `handler/dialog_handler.go` — `DialogHandler` + 5 методов + `Register(rg)`.
  Ошибки биндинга/парсинга `:id` → `apperr.Validationf(...)` через `c.Error`.
  Доменные ошибки сервиса — `c.Error(err)`, middleware разбирает.

### Task 8 — Сборка в main.go

- `run()`: `db.Open` теперь **фатально** при ошибке. Сборка
  `repo → service → handler`, `h.Register(engine.Group("/api/v1"))`,
  INFO `"dialog module mounted"`.

### Task 9 — Тесты

- `repository/dialog_repository_test.go` (5 тестов) и
  `handler/dialog_handler_test.go` (CRUD-цикл + валидация) против реального
  Postgres. `db.Migrate(cfg, up)` в setup, `TRUNCATE ... RESTART IDENTITY`.
- 🐛 **Гонка между пакетами:** `go test ./...` гоняет пакеты параллельно —
  `repository_test` и `handler_test` бьют в одну БД, тесты видели чужие
  строки. Фикс: `command: ["go","test","-p","1","./..."]` в сервисе
  `tests` (последовательный прогон пакетов).

### Task 10 — Сквозная проверка

- `docker compose run --rm tests` → всё зелёное.
- `curl` полный цикл против `:8090/api/v1/dialogs`: POST 201 → GET 200 →
  LIST 200 → PATCH 200 (`updated_at` изменился) → DELETE 204 → GET 404;
  POST без `user_id` → 422; LIST без `user_id` → 422.

**Итог вехи «Фундамент работы с БД»:** оба плана завершены. Есть
инфраструктура БД (GORM, транзакции, миграции) и первый доменный модуль
`dialog` со сквозным `handler → service → repository` и CRUD-эндпоинтами.
Появился общий пакет `internal/apperr` для доменных ошибок. Веха закрыта.
