[← Tool calling](tool-calling.md) · [Back to README](../README.md)

# Машина состояний диалога

Логика диалога — явная машина состояний с двумя состояниями `agent` и
`tools` и условным переходом между ними. Это полноценный агентный цикл:
модель может запрашивать инструменты несколько раз подряд, пока не даст
финальный текстовый ответ.

`internal/modules/dialog/service/machine.go`.

## Состояние

```go
type dialogState struct {
    messages []llms.MessageContent   // накопленная история диалога
}
```

Каждое состояние получает `*dialogState`, дописывает в него сообщения и
указывает следующее состояние.

```go
type transition struct {
    messages []llms.MessageContent   // что дописать в состояние
    next     stateName               // куда перейти ("" = стоп)
}
```

## Состояния и переходы

```
START → agent ─┬─(нет tool calls)─→ END
               └─(есть tool calls)─→ tools ─→ agent  (цикл)
```

- **`agent`** — вызывает модель (`GenerateContent` с `llms.WithTools`),
  дописывает ответ ассистента в состояние. Если в ответе есть запросы
  инструментов — переход в `tools`, иначе — стоп.
- **`tools`** — берёт запросы инструментов из последнего сообщения
  состояния, выполняет их через `llm.ExecuteToolCalls` (см.
  [tool-calling.md](tool-calling.md)), дописывает результаты
  (`llms.ToolCallResponse`) и возвращается в `agent`.

Цикл `agent ⇄ tools` повторяется, пока модель запрашивает инструменты.

### Предохранитель `maxDialogSteps`

`Run` считает шаги (переходы между состояниями). При превышении
`maxDialogSteps` (25) возвращается ошибка `dialog machine: превышен лимит
шагов` — в `ChatService` она превращается в `502`. Защита от модели,
которая зациклилась на запросах инструментов.

## Раннер

```go
m := newDialogMachine(model, tools, logger)
final, err := m.Run(ctx, prompt)      // prompt — []llms.MessageContent
answer := lastMessageText(final.messages)
```

`Run` крутит переходы от `start` до `stateEnd`, накапливая сообщения в
состоянии. Входной срез `prompt` не мутируется. Ошибка состояния
оборачивается: `dialog machine: состояние "agent": <...>`.

`lastMessageText` собирает текст из `llms.TextContent`-частей последнего
сообщения состояния — финальный ответ ассистента.

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
финальный текстовый ответ.

## Тесты

```bash
docker compose up -d postgres
docker compose run --rm tests
```

`internal/modules/dialog/service/machine_test.go` — `Run` напрямую с
`llmtest.Fake`: без инструментов, один раунд, **несколько раундов подряд**,
tool-сообщения в состоянии, лимит шагов (`Fake.ToolLoop`), накопление
сообщений, неизменность входного среза, проброс ошибки. Существующие тесты
`ChatService` проходят без изменений — поведенческий паритет.

## See Also

- [Tool calling](tool-calling.md) — `ExecuteToolCalls`, `Tool`, как добавить инструмент
- [Модуль dialog](dialog.md) — CRUD и эндпоинты диалога
- [Архитектура](../.ai-factory/ARCHITECTURE.md) — паттерн Structured Modules
