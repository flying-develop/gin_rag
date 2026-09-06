# Implementation Plan: Tool calling у LLM — переиспользуемый паттерн + пример-инструмент

Branch: none (git отключён в config)
Created: 2026-09-06

## Original Request
Веха 4 «Tool calling у LLM (structured output)» из .ai-factory/ROADMAP.md. Реализовать сквозной переиспользуемый паттерн tool calling через langchaingo (llms.WithTools → обработка resp.Choices[].ToolCalls → выполнение инструмента → ToolCallResponse обратно в модель → финальный ответ) на одном самодостаточном примере-инструменте без новых зависимостей (например, получение текущего времени), встроенном в существующий ChatService.SendMessage. Один раунд tool calling без рекурсии (многошаговые агентные циклы — отдельная веха «Диалог как state machine»). Паттерн должен переиспользоваться будущими вехами (RAG-поиск, БД-запросы, внешние API). Паритет с Python-планом tool-calling-llm.md, но НИКАКИХ упоминаний Python/Laravel в артефактах.

## Settings
- Testing: yes  # реальный Postgres (как сейчас) + расширенный `llmtest.Fake` со скриптованными ToolCalls, без реальных вызовов OpenAI
- Logging: standard  # INFO на каждый выполненный вызов инструмента (имя, tool_call_id, без полного результата); WARN — неизвестный инструмент / ошибка выполнения / повторный запрос инструментов
- Docs: yes  # обязательный чекпоинт /aif-docs по завершении

## Roadmap Linkage
Milestone: "Tool calling у LLM (structured output)"
Rationale: первый и единственный план вехи, реализует весь заявленный объём — сквозной паттерн tool calling (`llms.WithTools` → разбор `ToolCalls` → выполнение инструмента → `ToolCallResponse` → финальный ответ) как переиспользуемый хелпер `llm.GenerateWithTools` плюс один самодостаточный пример-инструмент `get_current_time`, встроенный в `ChatService`. Паттерн спроектирован для переиспользования будущими модулями (RAG-поиск, запросы к БД, внешние API): им достаточно реализовать интерфейс `llm.Tool` и передать список в тот же хелпер.

## Scope

В объёме:
- `internal/infrastructure/llm/tools.go` — интерфейс `Tool` (описание для модели + исполнитель) и хелпер `GenerateWithTools` (один раунд tool calling)
- Встроенная база таймзон (`_ "time/tzdata"`) — distroless-образ не содержит zoneinfo
- `internal/modules/dialog/service/tools.go` — пример-инструмент `get_current_time` + набор `DialogTools`
- Интеграция в `ChatService.SendMessage` (конструктор принимает `tools []llm.Tool`)
- Расширение `internal/infrastructure/llm/llmtest.Fake` — скриптованные ответы с `ToolCalls`
- Тесты: хелпер напрямую, сам инструмент, сквозной сценарий через `ChatService`

Вне объёма:
- Многошаговые агентные циклы / рекурсия tool calling — веха «Диалог как state machine»
- Streaming tool calls (SSE)
- Параллельное выполнение инструментов
- Персист промежуточного tool-обмена (`AIMessage` с tool_calls / `ToolMessage`) в `dialog_messages` — в истории остаётся только финальный текстовый ответ ассистента
- Реальные инструменты (БД, RAG-поиск, внешние API) — следующие вехи
- Форсирование инструмента через `ToolChoice`

## Commit Plan
<!-- git.enabled = false в config — чекпоинты фиксируют логическую группировку для журнала -->
- **Чекпоинт 1** (задачи 1-2): "feat(llm): add Tool interface and GenerateWithTools helper with get_current_time example"
- **Чекпоинт 2** (задачи 3-5): "feat(dialog): wire tool calling into ChatService; test: tool-calling coverage"
- **Чекпоинт 3** (задачи 6-7): "docs(llm): document tool-calling pattern"

## Tasks

### Phase 1: Переиспользуемый паттерн и пример-инструмент

- [x] Task 1: `llm.Tool` + `GenerateWithTools` в `internal/infrastructure/llm/tools.go` (зависит от вехи «Диалоги с LLM»).
  - Интерфейс:
    ```go
    // Tool — исполняемый инструмент для LLM: описание для модели + реализация.
    type Tool interface {
        Name() string                 // имя функции; совпадает с Definition().Function.Name
        Definition() llms.Tool        // описание (JSON Schema параметров) для передачи модели
        Execute(ctx context.Context, argumentsJSON string) (string, error)
    }
    ```
  - `func GenerateWithTools(ctx context.Context, model llms.Model, tools []Tool, messages []llms.MessageContent, logger *slog.Logger, opts ...llms.CallOption) (*llms.ContentResponse, error)`.
  - Логика (ровно один раунд — без рекурсии/агентных циклов):
    1. `logger == nil` → `slog.Default()`; `log := logger.With(slog.String("component", "llm.tools"))`.
    2. `len(tools) == 0` → `return model.GenerateContent(ctx, messages, opts...)` (полная обратная совместимость — один вызов, как раньше).
    3. Построить `defs []llms.Tool` из `t.Definition()` и индекс `byName map[string]Tool`. `callOpts := append([]llms.CallOption{llms.WithTools(defs)}, opts...)`.
    4. `resp, err := model.GenerateContent(ctx, messages, callOpts...)` → err → `fmt.Errorf("llm: первичный вызов с инструментами: %w", err)`.
    5. Если `len(resp.Choices) == 0` или `len(resp.Choices[0].ToolCalls) == 0` → `return resp, nil` (модель не запросила инструменты — ровно один вызов).
    6. Иначе — построить **новый** срез (входной `messages` не мутировать):
       `extended := make([]llms.MessageContent, 0, len(messages)+1+len(choice.ToolCalls))`, `append(extended, messages...)`,
       затем ассистентское сообщение с tool-calls: `llms.MessageContent{Role: llms.ChatMessageTypeAI, Parts: parts}` — `parts` содержит `llms.TextContent{Text: choice.Content}` (если непусто) и по одному `llms.ToolCall` на каждый вызов.
    7. Для каждого `tc := range choice.ToolCalls`:
       - `tc.FunctionCall == nil` → `result = "Error: malformed tool call"`, WARN `"malformed tool call"`.
       - инструмент не найден в `byName` → `result = fmt.Sprintf("Error: unknown tool %q", name)`, WARN `"unknown tool requested"` (`tool_name`), исключение не выбрасывать.
       - найден → `result, execErr := safeExecute(ctx, tool, tc.FunctionCall.Arguments)` (обёртка с `recover()` → ошибка). При `execErr` → `result = fmt.Sprintf("Error: tool %q failed: %v", name, execErr)`, WARN `"tool execution failed"` (`tool_name`, `error_type`), не пробрасывать.
       - успех → INFO `"tool call executed"` (`tool_name`, `tool_call_id`).
       - `append(extended, llms.MessageContent{Role: llms.ChatMessageTypeTool, Parts: []llms.ContentPart{llms.ToolCallResponse{ToolCallID: tc.ID, Name: name, Content: result}}})`.
    8. `final, err := model.GenerateContent(ctx, extended, callOpts...)` → err → `fmt.Errorf("llm: финальный вызов после инструментов: %w", err)`.
    9. Если `final.Choices[0].ToolCalls` непусто (модель снова просит инструмент — многошаговый tool calling вне скоупа) → WARN `"final response still requests tools; multi-round tool calling not supported"` (`tool_names`). Поведение не меняется.
    10. `return final, nil`.
  - Файлы: `internal/infrastructure/llm/tools.go` (новый).
  - Логирование: INFO — успешный вызов инструмента; WARN — неизвестный/некорректный инструмент, ошибка выполнения, повторный запрос инструментов в финальном ответе.
  - Проверка: `go build ./...`, `go vet ./...`.

- [x] Task 2: Пример-инструмент `get_current_time` + встроенная база таймзон (зависит от 1).
  - `cmd/api/main.go`: добавить blank-import `_ "time/tzdata"` с комментарием (distroless-рантайм не содержит системную zoneinfo; без этого `time.LoadLocation` для любой зоны кроме `UTC`/`Local` вернёт ошибку).
  - `internal/modules/dialog/service/tools.go` (новый, `package service`):
    - `type currentTimeTool struct{}`
    - `Name() → "get_current_time"`.
    - `Definition()` → `llms.Tool{Type: "function", Function: &llms.FunctionDefinition{Name: "get_current_time", Description: "Возвращает текущие дату и время в указанной таймзоне (формат RFC 3339).", Parameters: map[string]any{"type": "object", "properties": map[string]any{"timezone": map[string]any{"type": "string", "description": "IANA-имя таймзоны, например Europe/Moscow. По умолчанию UTC."}}}}}`.
    - `Execute(ctx, argumentsJSON string) (string, error)`:
      - `var args struct { Timezone string \`json:"timezone"\` }`; если `argumentsJSON != ""` → `json.Unmarshal([]byte(argumentsJSON), &args)` → err → `return "", fmt.Errorf("разбор аргументов: %w", err)`.
      - `tz := args.Timezone; if tz == "" { tz = "UTC" }`.
      - `loc, err := time.LoadLocation(tz)` → err → `return fmt.Sprintf("Error: unknown timezone %q", tz), nil` (graceful — та же философия, что в Task 1: модель получит текст ошибки и ответит сама).
      - `return time.Now().In(loc).Format(time.RFC3339), nil`.
    - `var DialogTools = []llm.Tool{currentTimeTool{}}` — экспортируемый набор инструментов ассистента в диалоге; импорт `internal/infrastructure/llm`.
  - Файлы: `cmd/api/main.go`, `internal/modules/dialog/service/tools.go` (новый).
  - Логирование: не требуется (чистые функции; вызовы логируются в `GenerateWithTools`).
  - Проверка: `go build ./...`.
<!-- Чекпоинт 1: задачи 1-2 -->

### Phase 2: Интеграция, расширение fake, тесты

- [x] Task 3: Wiring в `ChatService.SendMessage` (зависит от 1, 2).
  - `internal/modules/dialog/service/chat_service.go`:
    - `ChatService` — добавить поле `tools []llm.Tool`.
    - `NewChatService(repo DialogRepository, database *gorm.DB, model llms.Model, tools []llm.Tool) *ChatService` — новый параметр.
    - В `SendMessage`: заменить `resp, err := s.llm.GenerateContent(ctx, prompt)` на
      `resp, err := llm.GenerateWithTools(ctx, s.llm, s.tools, prompt, s.log)`.
      Существующая обработка ошибки не меняется: err → `apperr.Upstream("llm request failed", err)` (в БД ничего не пишется).
    - `firstChoice(resp)` по-прежнему извлекает финальный текст; промежуточные tool-сообщения **не персистятся** (осознанное упрощение базового паттерна — в `dialog_messages` только финальный ответ ассистента).
    - Импорт `github.com/flying-develop/ai-app-go/internal/infrastructure/llm`.
  - `cmd/api/main.go`: `chatSvc := dialogservice.NewChatService(dialogRepo, gormDB, llmClient, dialogservice.DialogTools)`.
  - Файлы: `internal/modules/dialog/service/chat_service.go`, `cmd/api/main.go`.
  - Логирование: без изменений (INFO `"chat message sent"` остаётся; вызовы инструментов логируются в хелпере).
  - Проверка: `go build ./...`.

- [x] Task 4: Расширить `llmtest.Fake` под скриптованные tool calls (зависит от 1).
  - `internal/infrastructure/llm/llmtest/fake.go`:
    - `type Response struct { Content string; ToolCalls []llms.ToolCall; Err error }`.
    - Поля `Fake`: `Responses []Response` (очередь; каждый `GenerateContent` берёт `Responses[0]` и сдвигает), `Prompts [][]llms.MessageContent` (копия messages каждого вызова).
    - `GenerateContent`: `f.Calls++`; `f.LastMessages = messages`; `f.Prompts = append(f.Prompts, append([]llms.MessageContent(nil), messages...))`.
      - Если `len(f.Responses) > 0` → `r := f.Responses[0]; f.Responses = f.Responses[1:]`; `r.Err != nil` → `return nil, r.Err`; иначе `return &llms.ContentResponse{Choices: []*llms.ContentChoice{{Content: r.Content, ToolCalls: r.ToolCalls}}}, nil`.
      - Иначе — прежнее поведение (`f.Err` / `f.Answer`), полная обратная совместимость с уже существующими тестами.
  - Файлы: `internal/infrastructure/llm/llmtest/fake.go`.
  - Логирование: не требуется (тестовый код).

- [x] Task 5: Тесты tool calling (зависит от 3, 4).
  - `internal/infrastructure/llm/tools_test.go` (новый, `package llm_test`) — хелпер напрямую с `llmtest.Fake`, без БД. Локальный `fakeTool` (реализует `llm.Tool`, запоминает полученные `argumentsJSON`, отдаёт заданный результат/ошибку):
    - `GenerateWithTools` без tool_calls → ровно 1 вызов, `messages` без изменений, вернулся первый ответ.
    - выполнение инструмента → `Responses: [{ToolCalls: [get_current_time, args={"timezone":"UTC"}]}, {Content: "финал"}]`: инструмент вызван с правильными `argumentsJSON`; 2-й вызов получил `ToolCallResponse` с правильным `ToolCallID` и текстом результата; вернулся именно 2-й ответ.
    - неизвестный инструмент → `ToolCallResponse` с `"Error: unknown tool ..."`, ошибка наружу не выброшена.
    - ошибка выполнения инструмента (`fakeTool` возвращает `error`) → `ToolCallResponse` с текстом ошибки ушёл во 2-й вызов, наружу ошибки нет.
    - входной срез `messages` не мутируется (та же длина и содержимое, что до вызова — сравнить с копией).
    - `tools == nil` → ровно 1 вызов, вернулся ответ модели.
  - `internal/modules/dialog/service/tools_test.go` (новый, `package service`; blank-import `_ "time/tzdata"` для герметичности):
    - `Execute` с валидной зоной (`Europe/Moscow`) → парсится как RFC3339, без ошибки.
    - `Execute` с `""` и `"{}"` → валидный RFC3339 (UTC по умолчанию).
    - `Execute` с неизвестной зоной → строка содержит `unknown timezone`, ошибка `nil`.
    - `Definition()` → имя `get_current_time`, в параметрах есть `timezone`.
  - `internal/modules/dialog/service/chat_service_test.go` — добавить `TestChatService_SendMessage_UsesToolResultInFinalReply`:
    - `fake := &llmtest.Fake{Responses: []llmtest.Response{{ToolCalls: []llms.ToolCall{{ID: "c1", Type: "function", FunctionCall: &llms.FunctionCall{Name: "get_current_time", Arguments: "{}"}}}}, {Content: "итоговый ответ"}}}`.
    - в БД сохранён именно финальный (2-й) ответ ассистента; `countMessages == 2` (user + финальный assistant, без промежуточных); `fake.Calls == 2`.
    - обновить хелпер `newChat`: `service.NewChatService(repo, gormDB, fake, service.DialogTools)`.
  - `internal/modules/dialog/handler/dialog_handler_test.go:45` — обновить вызов `service.NewChatService(repo, gormDB, fake, service.DialogTools)`.
  - Файлы: 2 новых `*_test.go` + правки в `chat_service_test.go`, `dialog_handler_test.go`.
  - Логирование: тесты — `logging.Setup("DEBUG")`.
  - Проверка: `docker compose up -d postgres`; `docker compose run --rm app migrate up`; `docker compose run --rm tests`.
<!-- Чекпоинт 2: задачи 3-5 -->

### Phase 3: Документация, проверка

- [x] Task 6: Документация — обязательный чекпоинт `/aif-docs` (зависит от 1-5).
  - Описать паттерн tool calling: интерфейс `llm.Tool`, хелпер `GenerateWithTools` (ровно один раунд, без рекурсии), как добавить новый инструмент, пример `get_current_time`, graceful-обработка (неизвестный инструмент / ошибка выполнения → текст модели, не 5xx), инструменты не сохраняются в истории диалога, требование `_ "time/tzdata"`.
    Уточнить на чекпоинте: отдельная страница `docs/tool-calling.md` или раздел в `docs/dialog.md`.
  - `AGENTS.md` — точки входа (`internal/infrastructure/llm/tools.go`), структура (`tools.go` в модуле dialog), отметить tool calling в возможностях.
  - `README.md` — «Возможности»: пункт про tool calling.
  - `.ai-factory/DESCRIPTION.md` / `.ai-factory/ARCHITECTURE.md` — при необходимости (переиспользуемый паттерн инструментов).
  - `.ai-factory/ROADMAP.md` — веха «Tool calling у LLM (structured output)» → `[x]`, строка в таблицу «Завершено» с датой.
  - Новый журнал `.ai-factory/journal/tool-calling-llm.md`.
  - `.env.example` — без изменений (новых переменных нет).

- [x] Task 7: Финальная проверка (зависит от 1-6).
  - `docker run --rm -v "$PWD":/app -w /app -e GOFLAGS=-buildvcs=false golang:1.25 sh -c "gofmt -l . && go vet ./... && go build ./..."`.
  - `docker run --rm -v "$PWD":/app -w /app -e GOFLAGS=-buildvcs=false golangci/golangci-lint:v2.5.0 golangci-lint run ./...`.
  - `docker compose up -d postgres`; `docker compose run --rm app migrate up`; `docker compose run --rm tests` — весь набор зелёный.
  - `docker compose up -d --build app`: создать диалог → `POST /api/v1/dialogs/:id/messages`. Без реального `OPENAI_API_KEY` → `502` (реальный tool-calling запрос к OpenAI без ключа/сети не проверяется — только smoke). Если пользователь даст рабочий ключ — happy-path с проверкой, что финальный ответ учитывает результат инструмента.
  - Проверить отсутствие TODO/debug-маркеров в новом коде.
  - `docker compose down`; убедиться, что не осталось файлов, принадлежащих root.
  - Файлы: нет новых.
<!-- Чекпоинт 3: задачи 6-7 -->
