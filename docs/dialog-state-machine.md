[← Tool calling](tool-calling.md) · [Back to README](../README.md)

# Машина состояний диалога

Логика диалога оформлена как явная машина состояний, а не линейный вызов
LLM. Сейчас в машине **одно** состояние — это переходный шаг: следующий
план вехи разложит его на `agent`/`tools` с циклом (многошаговый tool
calling).

`internal/modules/dialog/service/machine.go`.

## Состояние

```go
type dialogState struct {
    messages []llms.MessageContent   // накопленная история диалога
}
```

Каждое состояние машины получает `*dialogState`, при желании добавляет в
него сообщения и указывает следующее состояние.

```go
type transition struct {
    messages []llms.MessageContent   // что дописать в состояние
    next     stateName               // куда перейти ("" = стоп)
}

type stateFunc func(ctx context.Context, st *dialogState) (transition, error)
```

## Состояние `agent`

Единственное состояние. Внутри — вызов `llm.GenerateWithTools` (см.
[tool-calling.md](tool-calling.md)) на всей накопленной истории:
модель отвечает, при необходимости делает **один раунд** tool calling,
возвращается финальный текст. Состояние добавляет ответ ассистента в
`messages` и переходит в терминальное (`stateEnd`).

```
start → agent → END
```

## Раннер

```go
m := newDialogMachine(model, tools, logger)
final, err := m.Run(ctx, prompt)      // prompt — []llms.MessageContent
answer := lastMessageText(final.messages)
```

`Run` крутит переходы от `start` до `stateEnd`, накапливая сообщения в
состоянии. Входной срез `prompt` не мутируется (машина работает с копией).
Неизвестное имя состояния → ошибка `dialog machine: неизвестное состояние`.
Ошибка состояния оборачивается: `dialog machine: состояние "agent": <...>`.

`lastMessageText` собирает текст из `llms.TextContent`-частей последнего
сообщения состояния — это финальный ответ ассистента.

## В `ChatService`

`ChatService` хранит `*dialogMachine` (строится в `NewChatService`, если
`model != nil`). `SendMessage`:

1. проверяет диалог и `s.machine != nil` (иначе `502 llm not configured`);
2. собирает историю + новое сообщение пользователя в `prompt`;
3. `final, err := s.machine.Run(ctx, prompt)`;
4. `answer := lastMessageText(final.messages)`;
5. атомарно сохраняет сообщение пользователя и финальный ответ.

Промежуточные сообщения машины (запросы инструментов, их результаты) в
`dialog_messages` **не сохраняются** — в историю диалога идёт только
финальный текстовый ответ. Наблюдаемое поведение и коды ответов не
изменились по сравнению с прямым вызовом `GenerateWithTools`.

## Тесты

```bash
docker compose up -d postgres
docker compose run --rm tests
```

`internal/modules/dialog/service/machine_test.go` — `Run` напрямую с
`llmtest.Fake`: без инструментов, с инструментом (в состоянии — финальный
ответ, не tool-обмен), накопление сообщений, неизменность входного среза,
проброс ошибки. Существующие тесты `ChatService` проходят без изменений —
поведенческий паритет.

## See Also

- [Tool calling](tool-calling.md) — `GenerateWithTools` внутри состояния `agent`
- [Модуль dialog](dialog.md) — CRUD и эндпоинты диалога
- [Архитектура](../.ai-factory/ARCHITECTURE.md) — паттерн Structured Modules
