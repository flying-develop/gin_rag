[← Конфигурация](configuration.md) · [Back to README](../README.md)

# База данных и миграции

Подключение к PostgreSQL через GORM + миграции golang-migrate. Это
инфраструктура: доменные модели и репозитории добавляются на следующих
вехах.

## Подключение и пул (`internal/infrastructure/db`)

- `Open(ctx, cfg, logger) (*gorm.DB, error)` — открывает подключение по
  `DATABASE_URL`, настраивает пул (`MaxOpenConns=10`, `MaxIdleConns=5`,
  `ConnMaxLifetime=1h`) и делает `PingContext`. Логи GORM (включая SQL на
  уровне `DEBUG`) идут через `log/slog`.
- `Close(db)` — закрывает пул соединений.
- `WithinTx(ctx, db, fn)` — выполняет `fn` внутри транзакции: коммит при
  `nil`, откат при любой ошибке. Транзакции открываются в сервисном слое,
  операции внутри — через репозитории.

При старте `cmd/api` подключение к БД **нефатально**: если БД недоступна,
пишется `ERROR`, но сервис поднимается (ни один эндпоинт пока от БД не
зависит).

## Миграции

SQL-файлы лежат в `migrations/` в формате golang-migrate
(`{version}_{name}.up.sql` / `.down.sql`) и **встроены в бинарь** через
`go:embed` (`migrations/embed.go`). Отдельный CLI-инструмент не нужен —
миграции запускаются подкомандой самого сервиса.

```bash
# применить все миграции
docker compose run --rm app migrate up

# откатить все миграции (golang-migrate Down() снимает всё до пустой БД)
docker compose run --rm app migrate down
```

Состояние хранится в таблице `schema_migrations`. Повторный `migrate up`
без новых файлов выводит `no migration changes` и завершается успешно.

### Новая миграция

Создать пару файлов вручную, увеличив номер версии:

```
migrations/000002_add_dialog_tables.up.sql
migrations/000002_add_dialog_tables.down.sql
```

Файлы подхватываются `go:embed` при следующей сборке образа.

## Тесты

Рантайм-образ `app` — distroless, без Go-тулчейна. Тесты запускаются в
отдельном сервисе `tests` (образ `golang`, профиль `tools`):

```bash
docker compose up -d postgres
docker compose run --rm app migrate up
docker compose run --rm tests
```

Тесты `internal/infrastructure/db` работают против реального Postgres из
docker-compose — без моков.

## See Also

- [Конфигурация](configuration.md) — переменная `DATABASE_URL`
- [Быстрый старт](getting-started.md) — запуск всего стека
- [Архитектура](../.ai-factory/ARCHITECTURE.md) — место `internal/infrastructure/db` в структуре
