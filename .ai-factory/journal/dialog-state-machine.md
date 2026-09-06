# Журнал реализации: Диалог как state machine

Веха roadmap: «Диалог как state machine» — закрыта 2026-09-06.
Планы вехи:
- `.ai-factory/plans/dialog-state-machine-skeleton.md` — базовый скелет машины (реализован, 5/5).
- `.ai-factory/plans/dialog-state-machine-tools-loop.md` — состояния `agent`/`tools` с условным переходом и циклом, многошаговый tool calling (реализован, 6/6). Закрывает веху.

## Общее

- Все команды Go — через контейнер `golang:1.25` (`-buildvcs=false`),
  линт — `golangci/golangci-lint:v2.5.0`, тесты — `docker compose run --rm tests`.
- Новых зависимостей нет.

## План 1: Базовый скелет машины

### Task 1 — `dialogState` + `dialogMachine` + `Run`

- `internal/modules/dialog/service/machine.go` (новый):
  - `dialogState{ messages []llms.MessageContent }` — накопленная история.
  - `stateName` (`stateAgent = "agent"`, `stateEnd = ""`), `transition{ messages, next }`,
    `stateFunc func(ctx, *dialogState) (transition, error)`.
  - `dialogMachine{ start, states map, log }`.
  - `newDialogMachine(model, tools, logger)` — единственное состояние `agent`,
    внутри которого `llm.GenerateWithTools(ctx, model, tools, st.messages, log)`
    **без изменений**; результат → `llms.TextParts(ChatMessageTypeAI, answer)`, `next = stateEnd`.
  - `Run(ctx, initial)` — копирует вход (`append([]T(nil), initial...)`), крутит переходы
    `start → … → stateEnd`, накапливая `st.messages`. Неизвестное состояние → ошибка;
    ошибка состояния оборачивается `состояние %q: %w`.
  - `lastMessageText(msgs)` — конкатенация `llms.TextContent`-частей последнего сообщения.
  - INFO `"state enter"`/`"state exit"` (счётчики, не содержимое).

### Task 2 — Wiring в `ChatService`

- `ChatService`: поля `llm`/`tools` заменены на `machine *dialogMachine`.
- `NewChatService` — **сигнатура та же** (`repo, db, model, tools`); машина строится
  внутри только при `model != nil`, иначе `machine == nil` (guard → 502 `llm not configured`).
- `SendMessage`: `resp, err := llm.GenerateWithTools(...)` → `final, err := s.machine.Run(ctx, prompt)`;
  `answer := firstChoice(resp)` → `lastMessageText(final.messages)`. Обработка ошибки, транзакция
  сохранения, INFO `"chat message sent"` — без изменений.
- `main.go` и хендлер-тесты не тронуты (конструктор совместим).

### Task 3 — Тесты машины + регрессия

- `internal/modules/dialog/service/machine_test.go` (новый, `package service`): `Run` без
  инструментов; `Run` с инструментом (в состоянии — финальный ответ, `fake.Calls == 2`);
  накопление сообщений (system+human+AI); неизменность входного среза; проброс ошибки
  (`состояние "agent"`).
- Существующие тесты `ChatService` (`PersistsBothMessages`, `DialogNotFound`,
  `LLMFailure_NothingPersisted`, `UsesToolResultInFinalReply`) — без изменений в утверждениях,
  поведенческий паритет подтверждён.

### Task 4 — Документация (обязательный чекпоинт)

- Выбор пользователя: отдельная страница `docs/dialog-state-machine.md` (`dialogState`,
  состояние `agent`, `Run`, `lastMessageText`, почему пока одно состояние, план 2 вехи).
- Перекрёстные ссылки: `docs/dialog.md` (шаг 3 → машина состояний, See Also),
  `docs/tool-calling.md` (раздел «В диалоге», See Also), `README.md` (возможности + таблица),
  `AGENTS.md` (текущее состояние, структура, точки входа, таблица документации).
- `.ai-factory/ROADMAP.md` — веха **не** отмечена `[x]`; добавлена пометка «план 1/2 готов».

### Task 5 — Финальная проверка

- `gofmt -l .` / `go vet ./...` / `go build ./...` — чисто.
- `golangci-lint run ./...` (v2.5.0) — `0 issues`.
- `docker compose run --rm tests` — весь набор зелёный, существующие тесты `ChatService` без правок.
- Smoke: `/health` 200; `POST /dialogs/:id/messages` без ключа → 502 (поведение как в вехе 4).

**Итог плана:** структура машины состояний введена в проект (`dialogState`, состояние `agent`,
раннер `Run`, вызов из `ChatService`) без изменения наблюдаемого поведения — переходный шаг.
Веха «Диалог как state machine» пока не закрыта: второй план разложит состояние `agent` на
`agent`/`tools` с условным переходом и циклом (многошаговый tool calling).

## План 2: Состояния agent/tools с условным переходом и циклом

### Task 1 — Примитивы в `internal/infrastructure/llm/tools.go`

- Выделены экспортируемые функции: `ToolDefinitions(tools) []llms.Tool`,
  `AssistantToolCallMessage(choice) llms.MessageContent` (переименована из `assistantToolCallMessage`,
  nil-safe), `ToolCallsFromMessage(msg) []llms.ToolCall`,
  `ExecuteToolCalls(ctx, tools, calls, logger) []llms.MessageContent` (цикл выполнения с
  graceful-деградацией, вынесен из `GenerateWithTools`).
- `GenerateWithTools` переписан поверх примитивов — чистый рефакторинг; существующие тесты
  `tools_test.go` прошли без изменений (регрессия подтверждена).
- Причина: `ExecuteToolCalls` нужен и `GenerateWithTools`, и состоянию `tools` машины.

### Task 2 — Состояния `agent`/`tools` + цикл в `machine.go`

- `stateAgent`: прямой `model.GenerateContent(ctx, st.messages, llms.WithTools(defs))`
  (defs собираются один раз в `newDialogMachine`); ответ ассистента через
  `llm.AssistantToolCallMessage`; переход `tools` если есть tool calls, иначе `stateEnd`.
- `stateTools`: `llm.ToolCallsFromMessage(последнее сообщение)` → `llm.ExecuteToolCalls` →
  дописать результаты, вернуться в `stateAgent`.
- `Run`: счётчик шагов + `const maxDialogSteps = 25`; превышение → ошибка `превышен лимит шагов`
  (в `ChatService` → 502).
- `firstChoice` (строковый) удалён из `chat_service.go` — заменён на `firstChoiceOf` (в `machine.go`)
  + `lastMessageText`.
- `ChatService.SendMessage` не изменился со времён плана 1 — вся новая логика внутри машины.

### Task 3 — `llmtest.Fake.ToolLoop`

- Поле `ToolLoop []llms.ToolCall`: при пустом `Responses` каждый вызов возвращает ответ
  с этими tool calls — для теста лимита шагов.

### Task 4 — Тесты

- `machine_test.go`: новые `MultipleToolRounds` (3 вызова модели подряд — было невозможно),
  `StateIncludesToolMessages` (tool-сообщение в состоянии), `StepLimitExceeded` (`ToolLoop` →
  ошибка `лимит шагов`). Существующие 5 тестов машины + 4 теста `ChatService` — без изменений.
- `tools_test.go`: прямые тесты `ExecuteToolCalls` (порядок, неизвестный инструмент, ошибка,
  malformed); тесты `GenerateWithTools` — без изменений (регрессия после рефакторинга).

### Task 5 — Документация

- `docs/dialog-state-machine.md` переписан под `agent`/`tools`, условный переход, цикл,
  `maxDialogSteps`.
- `docs/tool-calling.md`: раздел «Примитивы» (`ExecuteToolCalls` и др. общие для хелпера и
  машины); блок «Один раунд» — относится только к `GenerateWithTools`, многошаговость для
  диалога на уровне машины.
- `README.md`, `AGENTS.md` — обновлены описания.
- `.ai-factory/ROADMAP.md` — веха → `[x]`, строка в «Завершено» (2026-09-06).

### Task 6 — Финальная проверка

- `gofmt`/`go vet`/`go build` — чисто; `golangci-lint` v2.5.0 — `0 issues`.
- `docker compose run --rm tests` — весь набор зелёный; регрессия машины/`ChatService`/
  `GenerateWithTools` без изменений.
- Smoke: `/health` 200; `POST /dialogs/:id/messages` без ключа → 502.

**Итог вехи:** диалог реализован как явная машина состояний `agent` ⇄ `tools` с условным
переходом и циклом — полноценный агентный цикл с многошаговым tool calling и предохранителем
по числу шагов. Веха «Диалог как state machine» закрыта.
