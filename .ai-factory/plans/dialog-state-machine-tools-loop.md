# Implementation Plan: Диалог как state machine — состояния agent/tools и цикл

Branch: none (git отключён в config)
Created: 2026-09-06

## Original Request
Веха 5 «Диалог как state machine», ВТОРОЙ и последний план вехи. Разложить единственное состояние `agent` (из internal/modules/dialog/service/machine.go) на два состояния `agent`/`tools` с условным переходом (если в ответе модели есть tool calls — идти в `tools`, иначе — стоп), реализовать многошаговый tool calling через машину состояний вместо однораундового ограничения. Закрывает веху. Паритет с Python-планом langgraph-dialog-graph-tools-loop.md, но БЕЗ LangGraph и БЕЗ упоминаний Python/Laravel/LangGraph в артефактах проекта.

## Settings
- Testing: yes  # многошаговый tool calling через машину (2+ раунда), лимит шагов, прямые тесты `ExecuteToolCalls`, регрессия существующих тестов машины/`ChatService`/`GenerateWithTools`
- Logging: standard  # INFO на вход/выход состояний `agent` и `tools` (число сообщений / число tool-вызовов), без содержимого — тот же уровень, что сейчас
- Docs: yes  # обязательный чекпоинт /aif-docs; этот план закрывает веху в ROADMAP

## Roadmap Linkage
Milestone: "Диалог как state machine"
Rationale: второй и последний план вехи. Первый ввёл машину состояний с одним состоянием `agent`, эквивалентным прежнему прямому вызову `llm.GenerateWithTools` (один раунд tool calling). Этот план заменяет тело `agent` на пару состояний `agent`/`tools` с условным переходом и циклом `agent → tools → agent → …`, снимая ограничение «один раунд» и закрывая веху. После него веха отмечается `[x]` в ROADMAP.

## Scope

В объёме:
- Рефакторинг `internal/infrastructure/llm/tools.go`: выделить переиспользуемый `ExecuteToolCalls` + экспортировать примитивы (`ToolDefinitions`, `AssistantToolCallMessage`, `ToolCallsFromMessage`); `GenerateWithTools` переписать поверх них (чистый рефакторинг, поведение и сигнатура не меняются)
- `machine.go`: состояния `agent` (прямой вызов модели с инструментами) и `tools` (выполнение tool calls), условный переход, цикл, лимит шагов `maxDialogSteps`
- Расширение `llmtest.Fake` для теста лимита шагов (бесконечный tool-loop)
- Тесты: многораундовый цикл, tool-сообщения в состоянии, лимит шагов, прямые тесты `ExecuteToolCalls`; регрессия существующих
- Документация; закрытие вехи в ROADMAP

Вне объёма:
- Персист промежуточных сообщений машины (`AIMessage` с tool calls / tool-ответы) в `dialog_messages` — в историю по-прежнему идёт только финальный текст
- Параллельное выполнение инструментов
- Ветвление по типам диалога, произвольные пользовательские состояния
- Streaming

## Commit Plan
<!-- git.enabled = false в config — чекпоинты фиксируют логическую группировку для журнала -->
- **Чекпоинт 1** (задачи 1-2): "feat(dialog): split agent state into agent/tools with conditional routing and loop"
- **Чекпоинт 2** (задачи 3-4): "test(dialog): cover multi-round tool calling through the state machine"
- **Чекпоинт 3** (задачи 5-6): "docs(dialog): document agent/tools loop; close dialog state-machine milestone"

## Tasks

### Phase 1: Примитивы и состояния

- [x] Task 1: Рефакторинг `internal/infrastructure/llm/tools.go` — выделить `ExecuteToolCalls` + экспортировать примитивы (зависит от вехи «Tool calling у LLM»).
  - Новые экспортируемые функции:
    - `ToolDefinitions(tools []Tool) []llms.Tool` — собрать описания инструментов для `llms.WithTools`.
    - `AssistantToolCallMessage(choice *llms.ContentChoice) llms.MessageContent` — переименовать существующую `assistantToolCallMessage`, сделать nil-safe (`choice == nil` → пустое AI-сообщение).
    - `ToolCallsFromMessage(msg llms.MessageContent) []llms.ToolCall` — извлечь `llms.ToolCall`-части из сообщения (нужно состоянию `tools` — прочитать запросы из последнего AI-сообщения состояния).
    - `ExecuteToolCalls(ctx context.Context, tools []Tool, calls []llms.ToolCall, logger *slog.Logger) []llms.MessageContent` — по одному `llms.MessageContent{Role: ChatMessageTypeTool, Parts: []{llms.ToolCallResponse{...}}}` на каждый вызов, в том же порядке. Внутри — та же логика `runToolCall` (неизвестный инструмент / ошибка / паника / отсутствие `FunctionCall` → текст `Error: ...` + WARN; успех → INFO). `logger == nil` → `slog.Default()`.
  - `GenerateWithTools` переписать поверх `ToolDefinitions` + `AssistantToolCallMessage` + `ExecuteToolCalls` — **чистый рефакторинг**: поведение (один раунд, WARN на повторный запрос инструментов, неизменность входного среза) и сигнатура не меняются; существующие тесты `internal/infrastructure/llm/tools_test.go` должны пройти без изменений в утверждениях.
  - Причина выделения: `ExecuteToolCalls` нужен и `GenerateWithTools` (однораундовый хелпер для прямых вызовов), и новому состоянию `tools` машины (Task 2) — без дублирования обработки ошибок.
  - Файлы: `internal/infrastructure/llm/tools.go`.
  - Логирование: без изменения уровня — тот же WARN/INFO, перенесённый в `ExecuteToolCalls`.
  - Проверка: `go build ./...`, `go vet ./...`, `docker compose run --rm tests` (пакет `llm`).

- [x] Task 2: Состояния `agent`/`tools` + цикл в `machine.go` (зависит от 1).
  - `newDialogMachine`: один раз собрать `defs := llm.ToolDefinitions(tools)`.
  - Состояние `stateAgent` (`"agent"`):
    - INFO `"state enter"` (`state`, `messages`).
    - `resp, err := model.GenerateContent(ctx, st.messages, llms.WithTools(defs))` (при пустых `tools` — без `WithTools`).
    - `err != nil` → `return transition{}, err`.
    - `choice := firstChoiceOf(resp)` (новый хелпер в `machine.go`: `*llms.ContentChoice` или `nil`).
    - `msg := llm.AssistantToolCallMessage(choice)`.
    - `next := stateEnd`; если `len(choice.ToolCalls) > 0` → `next = stateTools`.
    - INFO `"state exit"` (`state`, `tool_calls` = число запрошенных).
    - `return transition{messages: []llms.MessageContent{msg}, next: next}, nil`.
  - Состояние `stateTools` (`"tools"`):
    - INFO `"state enter"` (`state`).
    - `last := st.messages[len(st.messages)-1]`; `calls := llm.ToolCallsFromMessage(last)`.
    - `toolMsgs := llm.ExecuteToolCalls(ctx, tools, calls, m.log)`.
    - INFO `"state exit"` (`state`, `results` = `len(toolMsgs)`).
    - `return transition{messages: toolMsgs, next: stateAgent}, nil`.
  - `Run`: добавить счётчик шагов и `const maxDialogSteps = 25`. Каждая итерация цикла — шаг; при `steps > maxDialogSteps` → `fmt.Errorf("dialog machine: превышен лимит шагов (%d)", maxDialogSteps)` (ловится `SendMessage` → 502).
  - Убрать `firstChoice` из `chat_service.go`, если станет неиспользуемым (его роль занял `lastMessageText`; извлечение choice — `firstChoiceOf` в `machine.go`).
  - Обновить doc-комментарий типа `dialogMachine` (уже не «одно состояние»).
  - Файлы: `internal/modules/dialog/service/machine.go`, `internal/modules/dialog/service/chat_service.go`.
  - Логирование: INFO вход/выход `agent` и `tools`.
  - Проверка: `go build ./...`, `go vet ./...`.

### Phase 2: Fake и тесты

- [x] Task 3: Режим бесконечного tool-loop в `llmtest.Fake` (зависит от 1).
  - `internal/infrastructure/llm/llmtest/fake.go`: поле `ToolLoop []llms.ToolCall`. Если `Responses` пуст и `ToolLoop` не пуст — `GenerateContent` всегда возвращает `&llms.ContentResponse{Choices: []*llms.ContentChoice{{ToolCalls: f.ToolLoop}}}` (для теста лимита шагов машины). Иначе — прежнее поведение (`Answer`/`Err`).
  - Файлы: `internal/infrastructure/llm/llmtest/fake.go`.
  - Логирование: не требуется (тестовый код).

- [x] Task 4: Тесты (зависит от 2, 3).
  - `internal/modules/dialog/service/machine_test.go`:
    - Оставить/подтвердить: `Run` без tool calls (сразу `stateEnd`), `Run` один раунд (`agent → tools → agent → end`, `fake.Calls == 2`), накопление сообщений, неизменность входного среза, проброс ошибки (`состояние "agent"`).
    - Новый `TestDialogMachine_Run_MultipleToolRounds`: `Fake{Responses: [{ToolCalls: A}, {ToolCalls: B}, {Content: "финал"}]}` → `agent→tools→agent→tools→agent→end`; `fake.Calls == 3`, `lastMessageText == "финал"` (раньше было невозможно из-за ограничения «один раунд»).
    - Новый `TestDialogMachine_Run_StateIncludesToolMessages`: после раунда с инструментом в `st.messages` есть сообщение с `Role == llms.ChatMessageTypeTool`.
    - Новый `TestDialogMachine_Run_StepLimitExceeded`: `Fake{ToolLoop: [get_current_time]}` → `Run` возвращает ошибку с текстом `лимит шагов`.
  - `internal/infrastructure/llm/tools_test.go`:
    - Новые прямые тесты `ExecuteToolCalls`: успех (порядок результатов, `ToolCallID`/`Name`), неизвестный инструмент (`Error: unknown tool`), ошибка выполнения (`failed`), некорректный вызов (`malformed`).
    - Существующие тесты `GenerateWithTools` — **без изменений в утверждениях** (регрессия после рефакторинга Task 1).
  - `internal/modules/dialog/service/chat_service_test.go` — существующие тесты (`PersistsBothMessages`, `DialogNotFound`, `LLMFailure_NothingPersisted`, `UsesToolResultInFinalReply`) без изменений в утверждениях; `UsesToolResultInFinalReply` теперь проходит через полный цикл `agent/tools`, но с тем же наблюдаемым результатом.
  - Файлы: `machine_test.go`, `tools_test.go` (расширить); `chat_service_test.go` — только если потребуется (не ожидается).
  - Логирование: тесты — `logging.Setup("DEBUG")`.
  - Проверка: `docker compose up -d postgres`; `docker compose run --rm app migrate up`; `docker compose run --rm tests`.
<!-- Чекпоинт 2: задачи 3-4 -->

### Phase 3: Документация, проверка

- [x] Task 5: Документация — обязательный чекпоинт `/aif-docs` (зависит от 1-4).
  - `docs/dialog-state-machine.md` — переписать: состояния `agent`/`tools`, условный переход (`tool calls` → `tools`, иначе → стоп), цикл `agent ⇄ tools`, лимит `maxDialogSteps` (25) и как он проявляется (ошибка → 502). Отметить, что веха закрыта.
  - `docs/tool-calling.md` — `ExecuteToolCalls` теперь общий для `GenerateWithTools` и состояния `tools`; скорректировать блок про «один раунд»: многошаговый tool calling для диалога решается на уровне машины, `GenerateWithTools` как отдельный хелпер сохраняет однораундовое поведение.
  - `README.md` — обновить пункт «Машина состояний диалога» (многошаговый агентный цикл `agent`/`tools`).
  - `AGENTS.md` — описание `machine.go` (agent/tools), `llm/tools.go` (примитивы + `ExecuteToolCalls`); снять пометку «в работе», отметить веху завершённой.
  - `.ai-factory/ROADMAP.md` — веха «Диалог как state machine» → `[x]`, строка в таблицу «Завершено» (2026-09-06), убрать пометку «план 1/2».
  - `.ai-factory/journal/dialog-state-machine.md` — добавить раздел «План 2».
  - `.ai-factory/DESCRIPTION.md` — при необходимости (уже описывает «явные state machine в сервисном слое» — вероятно правок не нужно).

- [x] Task 6: Финальная проверка (зависит от 1-5).
  - `docker run --rm ... golang:1.25 sh -c "gofmt -l . && go vet ./... && go build ./..."`.
  - `docker run --rm ... golangci/golangci-lint:v2.5.0 golangci-lint run ./...`.
  - `docker compose up -d postgres`; `docker compose run --rm app migrate up`; `docker compose run --rm tests` — весь набор зелёный, существующие тесты машины/`ChatService`/`GenerateWithTools` без изменений.
  - `docker compose up -d --build app`: `/health` 200; `POST /dialogs/:id/messages` без `OPENAI_API_KEY` → 502.
  - Проверить отсутствие TODO/debug-маркеров.
  - `docker compose down`; убедиться, что не осталось файлов, принадлежащих root.
  - Файлы: нет новых.
<!-- Чекпоинт 3: задачи 5-6 -->
