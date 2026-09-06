[← Модуль dialog](dialog.md) · [Back to README](../README.md)

# Tool calling

Модель не только генерирует текст, но и вызывает инструменты-функции:
параметры описаны JSON Schema, результат вызова возвращается модели, и она
формирует финальный ответ. Паттерн переиспользуемый — на следующих вехах
те же инструменты дадут доступ к БД, RAG-поиску и внешним API.

## Интерфейс `llm.Tool`

`internal/infrastructure/llm/tools.go`:

```go
type Tool interface {
    Name() string                 // имя функции (== Definition().Function.Name)
    Definition() llms.Tool        // описание + JSON Schema параметров для модели
    Execute(ctx context.Context, argumentsJSON string) (string, error)
}
```

`Execute` получает аргументы как JSON-строку от модели и возвращает строку —
она уходит модели как результат вызова.

## Хелпер `GenerateWithTools`

```go
func GenerateWithTools(
    ctx context.Context,
    model llms.Model,
    tools []Tool,
    messages []llms.MessageContent,
    logger *slog.Logger,
    opts ...llms.CallOption,
) (*llms.ContentResponse, error)
```

Выполняет **ровно один раунд** tool calling:

1. запрос к модели со списком инструментов (`llms.WithTools`);
2. модель не запросила инструменты → ответ возвращается как есть (полностью
   совместимо с обычным `GenerateContent` — один сетевой вызов);
3. модель запросила инструменты → каждый выполняется, результат
   (`llms.ToolCallResponse`) добавляется в историю, следующий запрос к модели
   даёт финальный ответ.

Входной срез `messages` не мутируется — история собирается в новом срезе.

### Graceful-деградация

Любая проблема при вызове инструмента превращается в **текст для модели**,
а не в ошибку HTTP 5xx — модель должна суметь ответить сама:

| Ситуация | Что уходит модели | Лог |
|----------|-------------------|-----|
| Инструмент не найден | `Error: unknown tool "X"` | `WARN` |
| `Execute` вернул ошибку | `Error: tool "X" failed: <...>` | `WARN` |
| Паника в инструменте | `Error: tool "X" failed: паника в инструменте: <...>` | `WARN` |
| Некорректный запрос (нет `FunctionCall`) | `Error: malformed tool call` | `WARN` |

Ошибка самого сетевого вызова модели (первичного или финального) —
оборачивается и пробрасывается вызывающему (в чате → `502`).

### Один раунд, без рекурсии

Если после результата инструмента модель **снова** запрашивает инструмент
(многошаговый агентный цикл) — это логируется как `WARN`, но новый раунд не
запускается: возвращается последний ответ модели. Полноценные циклы — веха
«Диалог как state machine».

## Как добавить инструмент

1. Реализовать `llm.Tool` (обычно небольшая структура без состояния).
2. Описать параметры в `Definition()` как JSON Schema (`map[string]any`).
3. Добавить инструмент в список, который передаётся в `GenerateWithTools`.

Для диалога список — `service.DialogTools` в
`internal/modules/dialog/service/tools.go`; он передаётся в `NewChatService`
из `cmd/api/main.go`.

## Пример: `get_current_time`

Встроенный инструмент — текущее время в таймзоне (формат RFC 3339):

```go
// параметры, которые видит модель
{
  "type": "object",
  "properties": {
    "timezone": { "type": "string",
      "description": "IANA-имя таймзоны, например Europe/Moscow. По умолчанию UTC." }
  }
}
```

Неизвестная таймзона → `Error: unknown timezone "X"` (текст модели, не ошибка).

### База таймзон в бинаре

`time.LoadLocation` для любой зоны кроме `UTC`/`Local` требует базу IANA,
которой нет в distroless-образе. Поэтому `cmd/api/main.go` содержит
blank-import `_ "time/tzdata"` — база встроена в бинарь (~450 КБ).

## В диалоге

`ChatService.SendMessage` прогоняет машину состояний диалога
([dialog-state-machine.md](dialog-state-machine.md)); `GenerateWithTools`
вызывается внутри состояния `agent`. Промежуточный обмен (запрос
инструмента + его результат) **не сохраняется** в `dialog_messages` — в
историю диалога попадает только финальный текстовый ответ ассистента.
Полный обмен живёт в рамках одного вызова `SendMessage`.

## Тесты

```bash
docker compose up -d postgres
docker compose run --rm tests
```

Покрыто:

- `internal/infrastructure/llm` — `GenerateWithTools` со скриптованным
  `llmtest.Fake`: без инструментов, выполнение инструмента, неизвестный
  инструмент, ошибка выполнения, неизменность входного среза;
- `internal/modules/dialog/service` — сам инструмент `get_current_time`
  (валидная/дефолтная/неизвестная зона) и сквозной сценарий через
  `ChatService` (в БД — только финальный ответ).

## See Also

- [Модуль dialog](dialog.md) — где подключается tool calling
- [Машина состояний диалога](dialog-state-machine.md) — состояние `agent` вызывает `GenerateWithTools`
- [Конфигурация](configuration.md) — `OPENAI_API_KEY`, `OPENAI_MODEL`
- [Архитектура](../.ai-factory/ARCHITECTURE.md) — место `internal/infrastructure/llm`
