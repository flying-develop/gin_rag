# Implementation Plan: Диалог как state machine — базовый скелет

Branch: none (git отключён в config)
Created: 2026-09-06

## Original Request
Веха 5 «Диалог как state machine» из .ai-factory/ROADMAP.md, ПЕРВЫЙ из двух последовательных планов вехи — базовый скелет машины состояний. Ввести явный тип состояния диалога (накопленные []llms.MessageContent) и раннер машины состояний с ОДНИМ состоянием "agent", внутри которого по-прежнему вызывается существующий llm.GenerateWithTools БЕЗ изменений. ChatService.SendMessage вызывает раннер вместо прямого GenerateWithTools. Наблюдаемое поведение (включая один раунд tool calling) НЕ меняется — это инфраструктурный переходный шаг. Второй план вехи разложит на состояния agent↔tools с циклом и снимет ограничение «один раунд». Паритет с Python-планом langgraph-dialog-graph-skeleton.md, но БЕЗ LangGraph (в Go его нет — руками) и БЕЗ упоминаний Python/Laravel/LangGraph в артефактах проекта.

## Settings
- Testing: yes  # тесты раннера напрямую с `llmtest.Fake` (без OpenAI) + регрессия существующих тестов `ChatService` (паритет поведения)
- Logging: standard  # INFO на вход/выход состояния (число сообщений в состоянии, длина ответа), без содержимого сообщений — тот же уровень, что сейчас в `SendMessage`
- Docs: yes  # обязательный чекпоинт /aif-docs по завершении

## Roadmap Linkage
Milestone: "Диалог как state machine"
Rationale: первый из двух планов вехи. Вводит саму структуру машины состояний — тип `dialogState` (накопленные сообщения) и раннер `dialogMachine` с одним состоянием `agent`, вызываемый из `ChatService.SendMessage` вместо прямого `llm.GenerateWithTools`. Наблюдаемое поведение (включая один раунд tool calling) сохраняется без изменений — инфраструктурный переходный шаг. Второй план вехи разложит `agent` на состояния `agent`/`tools` с условными переходами и циклом (многошаговый tool calling), закрыв веху.

## Scope

В объёме:
- `internal/modules/dialog/service/machine.go` — `dialogState`, `dialogMachine`, `newDialogMachine` (одно состояние `agent`), метод `Run`
- Состояние `agent` внутри вызывает существующий `llm.GenerateWithTools` **без изменений**
- `ChatService` использует машину вместо прямого вызова хелпера; поведение и коды ответов не меняются
- Тесты раннера + регрессия `ChatService`

Вне объёма (второй план вехи и дальше):
- Состояние `tools` как отдельный шаг, условные переходы `agent`↔`tools`, цикл, лимит итераций — многошаговый tool calling
- Снятие ограничения «один раунд» в `GenerateWithTools`
- Персист промежуточных состояний/сообщений машины в БД
- Ветвление по типам диалога, произвольные пользовательские состояния

## Commit Plan
<!-- git.enabled = false в config — чекпоинты фиксируют логическую группировку для журнала -->
- **Чекпоинт 1** (задачи 1-3): "feat(dialog): introduce explicit dialog state machine with single agent state"
- **Чекпоинт 2** (задачи 4-5): "docs(dialog): document dialog state machine skeleton"

## Tasks

### Phase 1: Машина состояний

- [x] Task 1: `dialogState` + `dialogMachine` + `Run` в `internal/modules/dialog/service/machine.go` (зависит от вехи «Tool calling у LLM»).
  - Новый файл `machine.go`, `package service`.
  - Типы:
    ```go
    // dialogState — состояние машины: накопленные сообщения диалога.
    type dialogState struct { messages []llms.MessageContent }

    type stateName string
    const (
        stateAgent stateName = "agent"
        stateEnd   stateName = ""   // терминальное состояние (выход)
    )

    // transition — результат шага: что добавить в состояние и куда перейти.
    type transition struct {
        messages []llms.MessageContent
        next     stateName
    }

    type stateFunc func(ctx context.Context, st *dialogState) (transition, error)

    type dialogMachine struct {
        start  stateName
        states map[stateName]stateFunc
        log    *slog.Logger
    }
    ```
  - `newDialogMachine(model llms.Model, tools []llm.Tool, logger *slog.Logger) *dialogMachine`:
    - `start = stateAgent`; `log = logger.With(slog.String("component", "dialog.machine"))`.
    - Единственное состояние `stateAgent`:
      - INFO `"state enter"` (`state`, `messages` = `len(st.messages)`).
      - `resp, err := llm.GenerateWithTools(ctx, model, tools, st.messages, m.log)` — хелпер **не меняется**.
      - `err != nil` → `return transition{}, err`.
      - `answer := firstChoice(resp)` (существующий хелпер из `chat_service.go`).
      - INFO `"state exit"` (`state`, `answer_len`).
      - `return transition{ messages: []llms.MessageContent{llms.TextParts(llms.ChatMessageTypeAI, answer)}, next: stateEnd }, nil`.
  - `Run(ctx context.Context, initial []llms.MessageContent) (*dialogState, error)`:
    - `st := &dialogState{messages: append([]llms.MessageContent(nil), initial...)}` (копия входа — не мутируем аргумент).
    - Цикл `for current := m.start; current != stateEnd;`:
      - `fn, ok := m.states[current]`; `!ok` → `fmt.Errorf("dialog machine: неизвестное состояние %q", current)`.
      - `tr, err := fn(ctx, st)`; `err != nil` → `fmt.Errorf("dialog machine: состояние %q: %w", current, err)`.
      - `st.messages = append(st.messages, tr.messages...)`; `current = tr.next`.
    - `return st, nil`.
  - Хелпер `lastMessageText(msgs []llms.MessageContent) string` — конкатенация всех `llms.TextContent`-частей последнего сообщения (для извлечения финального ответа; пустой срез → `""`). Разместить в `machine.go`.
  - Файлы: `internal/modules/dialog/service/machine.go` (новый).
  - Логирование: INFO на вход/выход состояния (счётчики, не содержимое).
  - Проверка: `go build ./...`, `go vet ./...`.

### Phase 2: Wiring и тесты

- [x] Task 2: Wiring в `ChatService` (зависит от 1).
  - `internal/modules/dialog/service/chat_service.go`:
    - `ChatService` — заменить поля `llm llms.Model` и `tools []llm.Tool` на `machine *dialogMachine` (убрать неиспользуемые импорты, если появятся).
    - `NewChatService(repo, database, model, tools)` — сигнатура **не меняется**; внутри: `machine` строится только при `model != nil` (`if model != nil { machine = newDialogMachine(model, tools, log) }`), иначе `machine == nil`.
    - `SendMessage`:
      - guard в начале: `if s.machine == nil { return nil, apperr.Upstream("llm not configured", nil) }` (замена проверки `s.llm == nil`).
      - вместо `resp, err := llm.GenerateWithTools(...)` → `final, err := s.machine.Run(ctx, prompt)`; при ошибке — та же ветка (`ERROR "llm request failed"` + `apperr.Upstream("llm request failed", err)`).
      - `answer := lastMessageText(final.messages)` вместо `firstChoice(resp)`.
      - остальное (сохранение `user` + `assistant` в транзакции, INFO `"chat message sent"`) — без изменений.
  - Файлы: `internal/modules/dialog/service/chat_service.go`.
  - Логирование: без новых строк (машина логирует состояния; `SendMessage` сохраняет `"chat message sent"`).
  - Проверка: `go build ./...` (main.go и тесты не трогаем — сигнатура конструктора та же).

- [x] Task 3: Тесты машины + регрессия (зависит от 2).
  - `internal/modules/dialog/service/machine_test.go` (новый, `package service` — тестирует непубличные `newDialogMachine`/`Run`):
    - `Run` без tool calls: `llmtest.Fake{Answer: "привет"}` → `lastMessageText(st.messages) == "привет"`; `len(st.messages) == len(initial)+1`.
    - `Run` с инструментом: `Fake{Responses: [{ToolCalls: [get_current_time]}, {Content: "финал"}]}` → последнее сообщение состояния == `"финал"` (не промежуточный tool-обмен); `fake.Calls == 2`.
    - `Run` накапливает вход: `initial` из 2 сообщений → в `st.messages` присутствуют оба плюс ответ состояния `agent`.
    - `Run` не мутирует входной срез: длина/содержимое `initial` после вызова не изменились.
    - `Run` пробрасывает ошибку: `Fake{Err: ...}` → `Run` возвращает обёрнутую ошибку (`состояние "agent"`).
  - `internal/modules/dialog/service/chat_service_test.go` — существующие тесты (`PersistsBothMessages`, `DialogNotFound`, `LLMFailure_NothingPersisted`, `UsesToolResultInFinalReply`) должны пройти **без изменений в утверждениях** — паритет поведения. Правки только при необходимости (не ожидается).
  - Файлы: `machine_test.go` (новый).
  - Логирование: тесты — `logging.Setup("DEBUG")`.
  - Проверка: `docker compose up -d postgres`; `docker compose run --rm app migrate up`; `docker compose run --rm tests`.
<!-- Чекпоинт 1: задачи 1-3 -->

### Phase 3: Документация, проверка

- [x] Task 4: Документация — обязательный чекпоинт `/aif-docs` (зависит от 1-3).
  - Уточнить на чекпоинте: отдельная страница `docs/dialog-state-machine.md` или раздел в `docs/dialog.md`.
  - Описать: тип `dialogState` (накопленные сообщения), состояние `agent`, метод `Run`, почему пока одно состояние (переходный шаг перед вторым планом вехи с состояниями `agent`/`tools` и циклом), что поведение (включая один раунд tool calling через `GenerateWithTools` внутри состояния) не изменилось.
  - Перекрёстные ссылки: `docs/tool-calling.md`, `docs/dialog.md`, `README.md`, `AGENTS.md` (структура — новый `machine.go`/`machine_test.go`; таблица точек входа; таблица документации).
  - `.ai-factory/ROADMAP.md` — веху **НЕ** отмечать `[x]` (закрывается вторым планом); при желании добавить пометку про 1/2.
  - Новый журнал `.ai-factory/journal/dialog-state-machine.md` (с пометкой: план 1 из 2, веха не закрыта).

- [x] Task 5: Финальная проверка (зависит от 1-4).
  - `docker run --rm ... golang:1.25 sh -c "gofmt -l . && go vet ./... && go build ./..."`.
  - `docker run --rm ... golangci/golangci-lint:v2.5.0 golangci-lint run ./...`.
  - `docker compose up -d postgres`; `docker compose run --rm app migrate up`; `docker compose run --rm tests` — весь набор зелёный, существующие тесты `ChatService` без изменений.
  - `docker compose up -d --build app`: `/health` 200; создать диалог → `POST /dialogs/:id/messages` без `OPENAI_API_KEY` → 502; поведение идентично вехе 4.
  - Проверить отсутствие TODO/debug-маркеров.
  - `docker compose down`; убедиться, что не осталось файлов, принадлежащих root.
  - Файлы: нет новых.
<!-- Чекпоинт 2: задачи 4-5 -->
