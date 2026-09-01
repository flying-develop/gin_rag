# Журнал реализации: Диалоги с LLM — базовый чат

План: `.ai-factory/plans/dialog-llm-chat.md`
Веха roadmap: «Диалоги с LLM (базовый чат)»

## Общее

- Все команды Go — через контейнер `golang:1.25` (`-buildvcs=false`),
  тесты — `docker compose run --rm tests`.

## Task 1 — langchaingo + параметр модели

- `go get github.com/tmc/langchaingo` (v0.1.14).
- `config`: поле `OpenAIModel` (`OPENAI_MODEL`, дефолт `gpt-4o-mini`).
  Обновлён комментарий к `OpenAIAPIKey` (теперь используется).
  Удалён мёртвый код `isNotExist`.
- `.env.example`: `OPENAI_MODEL=gpt-4o-mini`.

## Task 2 — `internal/infrastructure/llm`

- `New(cfg, logger) (llms.Model, error)` — `openai.New(WithToken, WithModel)`.
  Пустой ключ → ошибка. Возврат — интерфейс `llms.Model` (провайдер скрыт).
- INFO `"llm client initialized" model=...`.

## Task 3 — Миграция `000003_create_dialog_messages`

- `dialog_messages` (id, dialog_id → `dialogs(id) ON DELETE CASCADE`,
  role, content, created_at) + `idx_dialog_messages_dialog_id (dialog_id, id)`.
- Проверено: `migrate up` → version 3; `migrate down`/`up` цикл чистый.

## Task 4 — Модель и DTO

- `model/dialog_message.go` — `DialogMessage` + `TableName()` + константы
  ролей `RoleUser`/`RoleAssistant`/`RoleSystem`.
- `dto/message.go` — `SendMessageRequest` (`text` required, max 8000),
  `MessageResponse` + мапперы.

## Task 5 — Порты и методы репозитория

- `service/ports.go`: `DialogRepository` расширен `AppendMessages(...*DialogMessage)`
  и `ListMessages(dialogID, limit)` (по `id ASC`, дефолт 100).
- `repository`: обе реализации через `r.conn(ctx)` — транзакция из ctx
  подхватывается автоматически.

## Task 6 — `apperr.KindUpstream` + `ChatService`

- `apperr`: `KindUpstream` + `Upstream(message, cause)`.
- `httpserver/errors.go`: `KindUpstream` → 502. Заодно уточнена логика:
  5xx логируются всегда (ERROR `"request failed"` с `status`), но клиенту
  «internal error» отдаётся только для чистых 500 — для 502 идёт
  заготовленное сообщение.
- `service/chat_service.go` — `ChatService.SendMessage`:
  `FindByID` → `ListMessages` → `buildPrompt` (история по ролям +
  сообщение пользователя) → `llm.GenerateContent` → **при успехе**
  `db.WithinTx` сохраняет `user` + `assistant`. Сбой LLM →
  `apperr.Upstream("llm request failed", err)`, в БД ничего.
- INFO `"chat message sent"` (`dialog_id`, `history_len`, `elapsed_ms`).

## Task 7 — Хендлеры

- `DialogHandler` получил `chat *ChatService` (2-й аргумент конструктора).
  `POST /dialogs/:id/messages` → 201 (ответ ассистента),
  `GET /dialogs/:id/messages` → 200 (сначала проверка существования диалога
  через `svc.GetByID`, иначе пустой список неотличим от 404).
- Добавлен `DialogService.ListMessages` (read-only история).

## Task 8 — Сборка в main.go

- ⚠️ **Отклонение от плана:** план говорил «фатально при ошибке `llm.New`».
  Сделано **нефатально**: без `OPENAI_API_KEY` приложение поднимается,
  `/health` и CRUD работают, а `llmClient = nil`. `ChatService.SendMessage`
  первым делом проверяет `s.llm == nil` → `apperr.Upstream("llm not configured")`
  → 502. Причина: не ломать `docker compose up` и CI без ключа.

## Task 9 — Тесты

- `internal/infrastructure/llm/llmtest/fake.go` — `Fake` реализует
  `llms.Model` (Answer/Err, запоминает `LastMessages`, счётчик `Calls`).
- `repository/dialog_message_repository_test.go` — append/list (порядок,
  limit), каскадное удаление сообщений при `Delete` диалога.
- `service/chat_service_test.go` — SendMessage: успех → в БД ровно
  +2 сообщения и fake получил историю; несуществующий диалог →
  `ErrDialogNotFound`; сбой fake → `apperr.KindUpstream` и **0 сообщений**.
- `handler/dialog_handler_test.go` — `newRouter` теперь отдаёт `(engine, *Fake)`;
  добавлены `SendMessage` (201 + ответ) и `SendMessage_Errors` (404, 422).
- 🐛 **`TRUNCATE dialogs`** ломался из-за FK от `dialog_messages` —
  во всех тест-хелперах заменено на
  `TRUNCATE dialogs, dialog_messages RESTART IDENTITY`.
- 19/19 тестов зелёные.

## Task 10 — Сквозная проверка

- `docker compose build app` → `migrate up` (version 3) → `docker compose run --rm tests` зелёное.
- `docker compose up -d app` (без ключа): лог
  `ERROR "llm client unavailable"` + `INFO "dialog module mounted"`,
  `/health` healthy.
- `curl`: создать диалог → `POST /dialogs/:id/messages` → 502
  `{"error":"llm not configured"}`; пустой `text` → 422;
  `GET /dialogs/:id/messages` → 200.
- Happy-path с реальным OpenAI — покрыт юнит-тестами с fake; для e2e нужен
  рабочий `OPENAI_API_KEY` в `.env`.

**Итог вехи:** модель `DialogMessage`, инфраструктура `internal/infrastructure/llm`
(langchaingo/OpenAI за интерфейсом `llms.Model`), `ChatService` с историей
диалога и атомарным сохранением, эндпоинты `/dialogs/:id/messages`.
Веха закрыта.
