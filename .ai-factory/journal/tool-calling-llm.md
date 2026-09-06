# Журнал реализации: Tool calling у LLM — переиспользуемый паттерн

План: `.ai-factory/plans/tool-calling-llm.md`
Веха roadmap: «Tool calling у LLM (structured output)»

## Общее

- Все команды Go — через контейнер `golang:1.25` (`-buildvcs=false`),
  линт — `golangci/golangci-lint:v2.5.0`, тесты — `docker compose run --rm tests`.
- Новых зависимостей нет (пример-инструмент — на stdlib).

## Task 1 — `llm.Tool` + `GenerateWithTools`

- `internal/infrastructure/llm/tools.go` (новый):
  - интерфейс `Tool` — `Name()`, `Definition() llms.Tool`, `Execute(ctx, argumentsJSON string) (string, error)`.
  - `GenerateWithTools(ctx, model, tools, messages, logger, opts...)` — ровно
    один раунд: запрос с `llms.WithTools` → если `ToolCalls` пусто, ответ как
    есть (один сетевой вызов) → иначе выполнить каждый инструмент, вернуть
    `llms.ToolCallResponse` модели, отдать финальный ответ.
  - Входной `messages` не мутируется — `extended` собирается заново.
  - Ассистентское сообщение с tool-calls восстанавливается (`assistantToolCallMessage`)
    и кладётся в историю перед результатами — модель этого ожидает.
  - Graceful: неизвестный инструмент / ошибка `Execute` / паника / отсутствие
    `FunctionCall` → текст `Error: ...` модели, WARN в лог, наружу не пробрасывается.
  - `safeExecute` — `recover()` вокруг стороннего кода инструмента (последний
    рубеж, аналогично recovery-middleware из base.md).
  - Повторный запрос инструментов в финальном ответе → WARN, новый раунд не
    запускается (многошаговые циклы — веха «Диалог как state machine»).
  - INFO `"tool call executed"` (`tool_name`, `tool_call_id`).

## Task 2 — пример-инструмент `get_current_time`

- `cmd/api/main.go`: blank-import `_ "time/tzdata"` — distroless-рантайм не
  содержит zoneinfo, без этого `time.LoadLocation` падает на любой зоне кроме
  UTC/Local.
- `internal/modules/dialog/service/tools.go` (новый):
  - `currentTimeTool` — `Definition()` с JSON Schema (`timezone` string, дефолт UTC),
    `Execute` → `time.Now().In(loc).Format(time.RFC3339)`.
  - Неизвестная зона → `Error: unknown timezone "X"` (текст, не ошибка).
    Некорректный JSON аргументов → ошибка (обернётся хелпером).
  - `var DialogTools = []llm.Tool{currentTimeTool{}}` — набор для `ChatService`.

## Task 3 — wiring в `ChatService`

- `ChatService` получил поле `tools []llm.Tool`; `NewChatService` — 4-й
  параметр `tools`. `SendMessage` вызывает `llm.GenerateWithTools(ctx, s.llm,
  s.tools, prompt, s.log)` вместо прямого `GenerateContent`.
- Промежуточный tool-обмен **не** сохраняется в `dialog_messages` — в историю
  идёт только финальный текстовый ответ (`firstChoice(resp)` без изменений).
- `cmd/api/main.go`: `NewChatService(..., dialogservice.DialogTools)`.
- Обработка ошибок не менялась: сбой сетевого вызова → `apperr.Upstream` → 502.

## Task 4 — расширение `llmtest.Fake`

- Тип `Response{ Content; ToolCalls []llms.ToolCall; Err }` + поле
  `Fake.Responses []Response` (очередь; приоритет над `Answer`/`Err`).
- `Fake.Prompts [][]llms.MessageContent` — копия messages каждого вызова
  (копия, не ссылка) для проверки, что получил 2-й запрос к модели.
- Полная обратная совместимость: существующие тесты на `Answer`/`Err` не тронуты.

## Task 5 — тесты

- `internal/infrastructure/llm/tools_test.go` (новый, `package llm_test`,
  локальный `fakeTool`): без tool-calls (1 вызов, инструмент не тронут);
  nil tools (pass-through); выполнение инструмента (правильные args, 2-й запрос
  получил `ToolCallResponse` с верным `ToolCallID`, вернулся 2-й ответ);
  неизвестный инструмент (graceful); ошибка `Execute` (graceful, `boom` в тексте);
  неизменность входного среза; обёртка ошибки первичного вызова.
- `internal/modules/dialog/service/tools_test.go` (новый, internal, blank-import
  `_ "time/tzdata"`): валидная зона → RFC3339; `""`/`"{}"` → UTC (нулевое смещение);
  неизвестная зона → текст `unknown timezone`; кривой JSON → ошибка; `Definition()`.
- `chat_service_test.go`: `TestChatService_SendMessage_UsesToolResultInFinalReply`
  — скрипт `[tool_call, финал]`, в БД ровно 2 сообщения (user + финальный
  assistant), `fake.Calls == 2`. Хелпер `newChat` обновлён под 4-й аргумент.
- `handler/dialog_handler_test.go`: вызов `NewChatService` обновлён (+`DialogTools`).
- `gofmt`/`go vet`/`golangci-lint` — чисто; `docker compose run --rm tests` — всё зелёное.

## Task 6 — документация

- Новая `docs/tool-calling.md`: интерфейс `Tool`, `GenerateWithTools` (один
  раунд, без рекурсии), таблица graceful-деградации, как добавить инструмент,
  пример `get_current_time`, про `_ "time/tzdata"`, поведение в диалоге, тесты.
- `docs/dialog.md`: врезка «Tool calling» в разделе «Сообщения и LLM»,
  ссылка в See Also, правка «Вне объёма».
- `README.md`: пункт в «Возможности» + строка в таблице документации.
- `AGENTS.md`: текущее состояние (веха 4, 2026-09-06), точки входа
  (`llm/tools.go`, `service/tools.go`), таблица документации.
- `.ai-factory/ROADMAP.md`: веха → `[x]`, строка в «Завершено» (2026-09-06).

## Task 7 — сквозная проверка

- `gofmt -l .` / `go vet ./...` / `go build ./...` — чисто.
- `golangci-lint run ./...` (v2.5.0) — `0 issues`.
- `docker compose run --rm tests` — все пакеты зелёные (добавлен пакет
  `internal/infrastructure/llm`).
- Реальный tool-calling запрос к OpenAI без ключа/сети не проверяется —
  покрыт юнит-тестами со скриптованным fake.

**Итог вехи:** переиспользуемый паттерн tool calling (`llm.Tool` +
`GenerateWithTools`, один раунд) поверх langchaingo, пример-инструмент
`get_current_time`, интеграция в `ChatService`. Будущим вехам достаточно
реализовать `llm.Tool` и передать список в тот же хелпер. Веха закрыта.
