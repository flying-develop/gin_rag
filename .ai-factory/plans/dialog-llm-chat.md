# Implementation Plan: Диалоги с LLM — базовый чат

Branch: none (git отключён в config)
Created: 2026-09-01

## Original Request
веха «Диалоги с LLM (базовый чат)» из .ai-factory/ROADMAP.md: модель DialogMessage (роль, контент, привязка к Dialog) + миграция, интеграция langchaingo chat-модели (OpenAI) через интерфейс llms.Model в internal/infrastructure/llm, эндпоинт POST /api/v1/dialogs/:id/messages — отправка сообщения пользователя, вызов LLM с историей диалога, сохранение обоих сообщений в PostgreSQL, возврат ответа ассистента. Архитектура — Structured Modules, см. .ai-factory/ARCHITECTURE.md.

## Settings
- Testing: yes  # репозиторий (реальный Postgres) + chat-сервис/хендлер с подставным llms.Model
- Logging: standard  # INFO — отправка сообщения, вызов LLM (модель, длительность), ошибки
- Docs: yes

## Roadmap Linkage
Milestone: "Диалоги с LLM (базовый чат)"
Rationale: план реализует весь объём вехи — модель DialogMessage, интеграцию langchaingo (OpenAI) за интерфейсом llms.Model, эндпоинт отправки сообщения с сохранением истории.

## Scope

В объёме:
- Зависимость `github.com/tmc/langchaingo`; `OPENAI_MODEL` в конфиге
- `internal/infrastructure/llm` — `New(cfg) (llms.Model, error)` (провайдер OpenAI)
- Модель `DialogMessage` (id, dialog_id → dialogs, role, content, created_at) + миграция `000003`
- `ChatService.SendMessage` — история диалога → вызов LLM → сохранение сообщения пользователя и ответа ассистента **в одной транзакции после успешного вызова** (атомарно: сбой LLM → 502, в БД ничего)
- Эндпоинты `POST /api/v1/dialogs/:id/messages`, `GET /api/v1/dialogs/:id/messages`
- `apperr.KindUpstream` → HTTP 502 для сбоев LLM
- Тесты: репозиторий сообщений + chat-сервис/хендлер с подставным `llms.Model`

Вне объёма:
- Streaming-ответы (SSE) — позже
- Tool calling — веха «Tool calling у LLM»
- Диалог как граф — веха «Диалог как state machine»
- RAG-контекст — вехи RAG
- Подсчёт/лимиты токенов, ретраи LLM

## Commit Plan
<!-- git.enabled = false в config — чекпоинты фиксируют логическую группировку -->
- **Чекпоинт 1** (задачи 1-4): "feat(dialog): add DialogMessage model, llm infra and migration"
- **Чекпоинт 2** (задачи 5-6): "feat(dialog): add chat service with llm history"
- **Чекпоинт 3** (задачи 7-8): "feat(dialog): expose POST /dialogs/:id/messages"
- **Чекпоинт 4** (задачи 9-10): "test(dialog): cover message repository and chat flow"

## Tasks

### Phase 1: LLM-инфраструктура и модель

- [x] Task 1: Зависимость langchaingo и параметр модели (зависит от вехи «Фундамент БД»).
  - `go get github.com/tmc/langchaingo` (в контейнере `golang:1.25`).
  - `internal/infrastructure/config/config.go`: поле `OpenAIModel` (`OPENAI_MODEL`, дефолт `gpt-4o-mini`).
  - `.env.example`: добавить `OPENAI_MODEL=gpt-4o-mini`.
  - Файлы: `go.mod`, `go.sum`, `internal/infrastructure/config/config.go`, `.env.example`.
  - Логирование: не требуется. Проверка: `go build ./...`, `go mod tidy`.

- [x] Task 2: `internal/infrastructure/llm` — инициализация chat-модели (зависит от 1).
  - `llm.go`: `New(cfg *config.Config, logger *slog.Logger) (llms.Model, error)` —
    `openai.New(openai.WithToken(cfg.OpenAIAPIKey), openai.WithModel(cfg.OpenAIModel))`.
    Пустой `OPENAI_API_KEY` → обёрнутая ошибка (эндпоинт чата от этого зависит).
  - Возвращаемый тип — интерфейс `llms.Model` из langchaingo, чтобы вызывающий код не знал про провайдера.
  - Файлы: `internal/infrastructure/llm/llm.go`.
  - Логирование: INFO `"llm client initialized"` с `model`.

- [x] Task 3: Миграция `000003_create_dialog_messages` (зависит от вехи «Фундамент БД»).
  - `up.sql`:
    - `CREATE TABLE dialog_messages (id bigserial PRIMARY KEY, dialog_id bigint NOT NULL REFERENCES dialogs(id) ON DELETE CASCADE, role text NOT NULL, content text NOT NULL, created_at timestamptz NOT NULL DEFAULT now());`
    - `CREATE INDEX idx_dialog_messages_dialog_id ON dialog_messages (dialog_id, id);`
  - `down.sql`: `DROP TABLE dialog_messages;`
  - Файлы: `migrations/000003_create_dialog_messages.up.sql`, `.down.sql`.
  - Проверка: `docker compose build app`, `migrate up` → version 3; `migrate down`; `migrate up`.

- [x] Task 4: Модель `DialogMessage` и DTO (зависит от 3).
  - `internal/modules/dialog/model/dialog_message.go`: struct `DialogMessage` (`ID`, `DialogID uint`, `Role string`, `Content string`, `CreatedAt time.Time`); `TableName() → "dialog_messages"`; константы `RoleUser = "user"`, `RoleAssistant = "assistant"`, `RoleSystem = "system"`.
  - `internal/modules/dialog/dto/message.go`:
    - `SendMessageRequest{ Text string json:"text" binding:"required,max=8000" }`
    - `MessageResponse{ ID, DialogID uint, Role, Content string, CreatedAt time.Time }` + `NewMessageResponse`, `NewMessageListResponse`.
  - Файлы: два новых файла.
  - Логирование: не требуется (типы).
<!-- Чекпоинт 1: задачи 1-4 -->

### Phase 2: Сервис чата

- [x] Task 5: Порты и методы репозитория для сообщений (зависит от 4).
  - `internal/modules/dialog/service/ports.go`: расширить `DialogRepository`:
    - `AppendMessages(ctx, msgs ...*model.DialogMessage) error` (создать в порядке передачи)
    - `ListMessages(ctx, dialogID uint, limit int) ([]model.DialogMessage, error)` (по `id ASC`, limit ≤ 0 → дефолт 100)
  - `internal/modules/dialog/repository/dialog_repository.go`: реализовать оба метода через `r.conn(ctx)` (транзакция подхватывается из ctx).
  - Файлы: `service/ports.go`, `repository/dialog_repository.go`.
  - Логирование: не требуется (SQL — на DEBUG через адаптер).

- [x] Task 6: `apperr.KindUpstream` + `ChatService` (зависит от 2, 5).
  - `internal/apperr/apperr.go`: добавить `KindUpstream` + хелпер `Upstream(message)`.
  - `internal/infrastructure/httpserver/errors.go`: `KindUpstream` → `http.StatusBadGateway` (502).
  - `internal/modules/dialog/service/chat_service.go`: struct `ChatService{ repo DialogRepository; db *gorm.DB; llm llms.Model; log *slog.Logger }`, `NewChatService(repo, db, llm)`.
    - `SendMessage(ctx, dialogID uint, text string) (*model.DialogMessage, error)`:
      1. `repo.FindByID` → nil → `ErrDialogNotFound`
      2. `repo.ListMessages(dialogID, 100)` — история
      3. собрать `[]llms.MessageContent` (история по ролям + новое сообщение пользователя); при пустой истории — опционально системный prompt-константа
      4. `llm.GenerateContent(ctx, msgs)` → ошибка → `apperr.Upstream("llm request failed")` (обёрнутая причина в лог)
      5. `db.WithinTx`: `repo.AppendMessages(user, assistant)`; вернуть сообщение ассистента
  - Файлы: `internal/apperr/apperr.go`, `internal/infrastructure/httpserver/errors.go`, `internal/modules/dialog/service/chat_service.go`.
  - Логирование: INFO `"chat message sent"` (`dialog_id`, `model`, `elapsed_ms`, `history_len`); ERROR при сбое LLM с текстом причины.
<!-- Чекпоинт 2: задачи 5-6 -->

### Phase 3: HTTP-слой и сборка

- [x] Task 7: Хендлеры сообщений (зависит от 6).
  - `internal/modules/dialog/handler/dialog_handler.go`: `NewDialogHandler(svc *service.DialogService, chat *service.ChatService)`.
    - `SendMessage` (POST `/dialogs/:id/messages`) → 201 `MessageResponse` (ответ ассистента) / 404 / 422 / 502.
    - `ListMessages` (GET `/dialogs/:id/messages`) → 200 `[]MessageResponse` / 404.
    - Зарегистрировать в `Register(rg)`.
  - Файлы: `internal/modules/dialog/handler/dialog_handler.go`.
  - Логирование: не дублировать сервис.

- [x] Task 8: Сборка в `cmd/api/main.go` (зависит от 7).
  - `run()`: после `db.Open` — `llmClient, err := llm.New(cfg, logger)` (фатально при ошибке — эндпоинт чата обязателен).
  - `chatSvc := dialogservice.NewChatService(dialogRepo, gormDB, llmClient)`.
  - `dialoghandler.NewDialogHandler(dialogSvc, chatSvc).Register(engine.Group("/api/v1"))`.
  - Файлы: `cmd/api/main.go`.
  - Логирование: INFO уже из `llm.New`.
<!-- Чекпоинт 3: задачи 7-8 -->

### Phase 4: тесты и сквозная проверка

- [x] Task 9: Тесты (зависит от 6, 7).
  - `internal/modules/dialog/repository/dialog_message_repository_test.go` (реальный Postgres):
    - `AppendMessages` сохраняет в порядке; `ListMessages` возвращает по `id ASC` с limit.
    - Каскад: `DELETE dialogs` удаляет связанные `dialog_messages`.
  - `internal/modules/dialog/service/chat_service_test.go`:
    - подставной `llms.Model` (fake) — возвращает фиксированный текст, запоминает переданные `MessageContent`.
    - `SendMessage`: несуществующий диалог → `ErrDialogNotFound`; успех → в БД ровно 2 новых сообщения (`user` + `assistant`), fake получил историю + новое сообщение; сбой fake → `apperr` KindUpstream и **в БД ничего не добавлено**.
  - `internal/modules/dialog/handler/dialog_handler_test.go`: расширить — `POST /dialogs/:id/messages` с fake-llm → 201; несуществующий диалог → 404; пустой `text` → 422.
  - Файлы: новые/расширенные `*_test.go`; общий fake `llms.Model` — в `internal/modules/dialog/service` (экспортируемый helper для тестов) либо в отдельном `internal/infrastructure/llm/llmtest`.
  - Логирование: тесты — `logging.Setup("DEBUG")`.

- [x] Task 10: Сквозная проверка (зависит от 8, 9).
  - `docker compose up -d postgres`; `docker compose build app`; `migrate up`; `docker compose run --rm tests`.
  - `docker compose up -d --build app`:
    - `POST /api/v1/dialogs/999/messages` → 404.
    - Без валидного `OPENAI_API_KEY`: `POST .../:id/messages` для существующего диалога → 502 (или ошибка старта, если ключа нет вообще — тогда задокументировать, что ключ обязателен).
    - Если в `.env` задан рабочий `OPENAI_API_KEY` — полный happy-path: создать диалог → отправить сообщение → 201 с ответом ассистента → `GET .../:id/messages` показывает 2 записи.
  - `docker compose down`.
  - Файлы: нет новых.
<!-- Чекпоинт 4: задачи 9-10 -->

## Документация (чекпоинт /aif-docs после Task 10)
- `docs/dialog.md` — раздел «Сообщения и LLM»: модель `DialogMessage`, эндпоинты `/dialogs/:id/messages`, поведение при сбое LLM (502, атомарность), требование `OPENAI_API_KEY`/`OPENAI_MODEL`.
- `docs/configuration.md` — `OPENAI_MODEL`, актуализировать `OPENAI_API_KEY` (теперь обязателен для запуска).
- `README.md`, `AGENTS.md` (`internal/infrastructure/llm`, langchaingo), `.ai-factory/ROADMAP.md` — веха → `[x]`.
- Новый журнал `.ai-factory/journal/dialog-llm-chat.md`.
