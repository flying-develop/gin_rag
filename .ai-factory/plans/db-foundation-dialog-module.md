# Implementation Plan: Модуль dialog — сквозной CRUD

Branch: none (git отключён в этом проекте)
Created: 2026-09-01

## Original Request
второй план вехи «Фундамент работы с БД»: модуль dialog в internal/modules/dialog/ (model/repository/service/handler/dto) со сквозным CRUD через repository-паттерн, миграция таблицы dialogs (golang-migrate), CRUD-эндпоинты Gin, подключение *gorm.DB из main.go в модуль. Архитектура — Structured Modules, см. .ai-factory/ARCHITECTURE.md.

## Settings
- Testing: yes  # репозиторий (против реального Postgres) + хендлеры (httptest)
- Logging: standard  # INFO — создание/обновление/удаление диалога
- Docs: yes  # обязательный чекпоинт документации в /aif-implement после завершения

## Roadmap Linkage
Milestone: "Фундамент работы с БД"
Rationale: второй и последний план вехи — добавляет первый доменный модуль (`dialog`) со сквозным потоком `handler → service → repository` через repository-паттерн поверх инфраструктуры БД из плана 1. После этого плана веха закрывается.

## Scope

В объёме:
- Миграция `dialogs` (golang-migrate up/down)
- `internal/modules/dialog/` — `model`, `dto`, `repository`, `service`, `handler` по ARCHITECTURE.md
- Сущность `Dialog` (id, user_id, title, created_at, updated_at) — без `DialogMessage` (это веха «Диалоги с LLM»)
- CRUD-эндпоинты под `/api/v1/dialogs`
- Минимальный error-middleware в `internal/infrastructure/httpserver` — доменные ошибки → JSON + HTTP-код
- Сборка `repository → service → handler` в `cmd/api/main.go`, монтирование роутов модуля
- БД становится обязательной при старте (эндпоинты от неё зависят)
- Тесты репозитория (реальный Postgres) и хендлеров (httptest)

Вне объёма:
- `DialogMessage`, интеграция с LLM — веха «Диалоги с LLM»
- Аутентификация / API-ключи (`user_id` пока приходит в запросе) — веха «API-ключи и контексты доступа»
- Полный `application/problem+json` — веха «Устойчивость и наблюдаемость»
- Пагинация сложнее `limit`/`offset`

## Commit Plan
<!-- git.enabled = false в config — чекпоинты фиксируют логическую группировку -->
- **Чекпоинт 1** (после задач 1-5): "feat(dialog): add dialog model, repository and service"
- **Чекпоинт 2** (после задач 6-8): "feat(dialog): expose CRUD endpoints with error middleware"
- **Чекпоинт 3** (после задач 9-10): "test(dialog): cover repository and handlers"

## Tasks

### Phase 1: Доменное ядро модуля

- [x] Task 1: Миграция таблицы `dialogs` (зависит от инфраструктуры плана 1).
  - `migrations/000002_create_dialogs.up.sql`:
    - `CREATE TABLE dialogs (id bigserial PRIMARY KEY, user_id bigint NOT NULL, title text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now());`
    - `CREATE INDEX idx_dialogs_user_id ON dialogs (user_id);`
  - `migrations/000002_create_dialogs.down.sql`: `DROP TABLE dialogs;`
  - Файлы: `migrations/000002_create_dialogs.up.sql`, `migrations/000002_create_dialogs.down.sql`.
  - Проверка: `docker compose build app`, `docker compose run --rm app migrate up` → `schema_migrations.version = 2`; `migrate down` → откат; снова `up`.
  - Логирование: не требуется (SQL).

- [x] Task 2: Модель и DTO модуля (зависит от 1).
  - `internal/modules/dialog/model/dialog.go`: struct `Dialog` (`ID uint gorm:"primaryKey"`, `UserID uint gorm:"index"`, `Title string`, `CreatedAt`, `UpdatedAt time.Time`); метод `TableName() string` → `"dialogs"`.
  - `internal/modules/dialog/dto/dialog.go`:
    - `CreateDialogRequest{ UserID uint json:"user_id" binding:"required", Title string json:"title" binding:"max=200" }`
    - `UpdateDialogRequest{ Title *string json:"title" binding:"omitempty,max=200" }`
    - `DialogResponse{ ID, UserID uint, Title string, CreatedAt, UpdatedAt time.Time }` + `NewDialogResponse(*model.Dialog) DialogResponse` и `NewDialogListResponse([]model.Dialog) []DialogResponse`.
  - Файлы: `internal/modules/dialog/model/dialog.go`, `internal/modules/dialog/dto/dialog.go`.
  - Логирование: не требуется (типы данных).

- [x] Task 3: Доменные ошибки и интерфейс репозитория (зависит от 2).
  - `internal/modules/dialog/service/errors.go`: `ErrDialogNotFound = errors.New("dialog not found")`.
  - `internal/modules/dialog/service/ports.go`: интерфейс `DialogRepository` (объявлен на стороне потребителя):
    - `Create(ctx, *model.Dialog) error`
    - `FindByID(ctx, id uint) (*model.Dialog, error)` (nil, nil если не найдено)
    - `List(ctx, userID uint, limit, offset int) ([]model.Dialog, error)`
    - `Update(ctx, *model.Dialog) error`
    - `Delete(ctx, id uint) (bool, error)` (false если строки не было)
  - Файлы: `internal/modules/dialog/service/errors.go`, `internal/modules/dialog/service/ports.go`.
  - Логирование: не требуется.

- [x] Task 4: Реализация репозитория (зависит от 3).
  - `internal/modules/dialog/repository/dialog_repository.go`: struct `DialogRepository{ db *gorm.DB }`, `NewDialogRepository(db) *DialogRepository`.
    - Все методы — `r.db.WithContext(ctx)...`.
    - `FindByID`: `errors.Is(err, gorm.ErrRecordNotFound)` → `nil, nil`.
    - `List`: фильтр по `user_id`, `Order("created_at DESC")`, `Limit`/`Offset` (limit ≤ 0 → дефолт 50).
    - `Delete`: по `RowsAffected` вернуть bool.
  - Прямые вызовы `*gorm.DB` — только здесь (правило проекта).
  - Файлы: `internal/modules/dialog/repository/dialog_repository.go`.
  - Логирование: не требуется (SQL логируется адаптером GORM на DEBUG).

- [x] Task 5: Сервис модуля (зависит от 3, 4).
  - `internal/modules/dialog/service/dialog_service.go`: struct `DialogService{ repo DialogRepository; db *gorm.DB }`, `NewDialogService(repo, db) *DialogService`.
    - `Create(ctx, CreateDialogRequest) (*model.Dialog, error)` — собрать модель, `repo.Create`.
    - `GetByID(ctx, id) (*model.Dialog, error)` — `repo.FindByID`; nil → `ErrDialogNotFound`.
    - `List(ctx, userID, limit, offset) ([]model.Dialog, error)`.
    - `Update(ctx, id, UpdateDialogRequest) (*model.Dialog, error)` — внутри `db.WithinTx`: `FindByID` (nil → `ErrDialogNotFound`), применить поля, `repo.Update`.
    - `Delete(ctx, id) error` — `repo.Delete`; false → `ErrDialogNotFound`.
  - Файлы: `internal/modules/dialog/service/dialog_service.go`.
  - Логирование: INFO `"dialog created"` / `"dialog updated"` / `"dialog deleted"` с `dialog_id`.
<!-- Чекпоинт 1: задачи 1-5 -->

### Phase 2: HTTP-слой

- [x] Task 6: Error-middleware в httpserver (зависит от 3).
  - `internal/infrastructure/httpserver/errors.go`: middleware `errorHandler(logger)` — после `c.Next()` смотрит `c.Errors.Last()`:
    - `errors.Is(err, dialogsvc.ErrDialogNotFound)` → 404 `{"error":"dialog not found"}`.
    - Тип-ошибка валидации биндинга (передаётся хендлером через `c.Error(...)` с пометкой `gin.ErrorTypeBind`) → 422 с деталями.
    - Иначе → 500 `{"error":"internal error"}` + ERROR-лог с полным текстом.
  - Импорт `dialog/service` из `httpserver` — допустимое исключение: infra знает про доменные ошибки для маппинга (альтернатива — общий пакет `apperr`; вынести при росте числа модулей, отметить `TODO`).
  - `server.go`: подключить `errorHandler(logger)` в цепочку middleware; `New()` теперь принимает `logger` (уже есть).
  - Файлы: `internal/infrastructure/httpserver/errors.go`, `internal/infrastructure/httpserver/server.go`.
  - Логирование: ERROR на необработанных ошибках (500) с `path`, `error`.

- [x] Task 7: Хендлеры модуля (зависит от 5, 6).
  - `internal/modules/dialog/handler/dialog_handler.go`: struct `DialogHandler{ svc *service.DialogService }`, `NewDialogHandler(svc)`.
    - `Create` (POST `/dialogs`) → 201 `DialogResponse`.
    - `List` (GET `/dialogs?user_id=&limit=&offset=`) → 200 `[]DialogResponse` (`user_id` обязателен, иначе 422).
    - `Get` (GET `/dialogs/:id`) → 200 / 404.
    - `Update` (PATCH `/dialogs/:id`) → 200 / 404 / 422.
    - `Delete` (DELETE `/dialogs/:id`) → 204 / 404.
    - Парсинг `:id` — хелпер, ошибка → `c.Error(...).SetType(gin.ErrorTypeBind)`.
    - Доменные ошибки сервиса пробрасывать через `c.Error(err)` (middleware разберёт).
    - `Register(rg *gin.RouterGroup)` — регистрирует пять роутов.
  - Файлы: `internal/modules/dialog/handler/dialog_handler.go`.
  - Логирование: не дублировать сервис; DEBUG на вход хендлера по желанию.

- [x] Task 8: Сборка модуля в `cmd/api/main.go` (зависит от 7).
  - В `run()`: после `db.Open` (теперь **фатально** при ошибке — эндпоинты зависят от БД) собрать:
    `repo := repository.NewDialogRepository(gormDB)` → `svc := service.NewDialogService(repo, gormDB)` → `h := handler.NewDialogHandler(svc)`.
  - `engine := httpserver.New(cfg, logger)`; `h.Register(engine.Group("/api/v1"))`.
  - Файлы: `cmd/api/main.go`.
  - Логирование: INFO `"dialog module mounted" prefix=/api/v1`.
<!-- Чекпоинт 2: задачи 6-8 -->

### Phase 3: тесты и сквозная проверка

- [x] Task 9: Тесты репозитория и хендлеров (зависит от 4, 7).
  - `internal/modules/dialog/repository/dialog_repository_test.go` (пакет `repository_test`, реальный Postgres из compose):
    - Create → FindByID возвращает ту же запись; FindByID несуществующего → `nil, nil`.
    - List по `user_id` возвращает только свои, в порядке `created_at DESC`, уважает `limit`.
    - Update меняет `title`; Delete возвращает `true`, повторный Delete → `false`.
    - Setup: `TRUNCATE dialogs RESTART IDENTITY` в начале каждого теста; миграции применяются через `db.Migrate` в `TestMain`.
  - `internal/modules/dialog/handler/dialog_handler_test.go` (пакет `handler_test`, `httptest` + `gin`):
    - Полный цикл create(201) → get(200) → list(200) → update(200) → delete(204) → get(404).
    - Валидация: `POST` без `user_id` → 422; `GET /dialogs` без `user_id` → 422.
    - Использует реальный сервис + репозиторий против Postgres (сквозной тест слоя).
  - Файлы: два `*_test.go`.
  - Логирование: тесты поднимают `logging.Setup("DEBUG")`.

- [x] Task 10: Сквозная проверка вехи (зависит от 8, 9).
  - `docker compose up -d postgres`; `docker compose build app`; `docker compose run --rm app migrate up`.
  - `docker compose run --rm tests` → всё зелёное.
  - `docker compose up -d --build app`; полный CRUD через `curl`:
    `POST /api/v1/dialogs` → 201; `GET /api/v1/dialogs/:id` → 200; `GET /api/v1/dialogs?user_id=1` → список; `PATCH` → 200; `DELETE` → 204; `GET` удалённого → 404.
  - `docker compose down`.
  - Файлы: нет новых.
<!-- Чекпоинт 3: задачи 9-10 -->

## Документация (чекпоинт /aif-docs после Task 10)
- `docs/dialog.md` (или раздел в `docs/db.md`) — модуль dialog: структура слоёв, CRUD-эндпоинты `/api/v1/dialogs`, формат запросов/ответов, коды ошибок.
- Обновить `README.md` (возможности), `docs/getting-started.md` (пример запроса к API), `AGENTS.md` (первый доменный модуль, структура).
- `.ai-factory/ROADMAP.md` — веха «Фундамент работы с БД» → `[x]`, запись в «Завершено».
- Дописать `.ai-factory/journal/db-foundation.md` (план 2).
