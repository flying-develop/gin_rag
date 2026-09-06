[← БД и миграции](db.md) · [Back to README](../README.md)

# Модуль dialog

Первый доменный модуль: CRUD по диалогам (чат-сессиям). Реализован по
паттерну Structured Modules — `handler → service → repository`.

## Структура

```
internal/modules/dialog/
├── model/       # GORM-модель Dialog
├── dto/         # запросы/ответы API + мапперы
├── service/     # use cases, доменные ошибки, порт репозитория
├── repository/  # доступ к БД (GORM), единственное место с *gorm.DB
└── handler/     # Gin-хендлеры, регистрация роутов
```

Сборка зависимостей — в `cmd/api/main.go`:
`repository → service → handler`, роуты монтируются в группу `/api/v1`.

## Эндпоинты

Базовый префикс — `/api/v1`.

| Метод | Путь | Описание | Коды |
|-------|------|----------|------|
| `POST` | `/dialogs` | создать диалог | 201, 422 |
| `GET` | `/dialogs?user_id=&limit=&offset=` | список диалогов пользователя | 200, 422 |
| `GET` | `/dialogs/:id` | получить диалог | 200, 404 |
| `PATCH` | `/dialogs/:id` | обновить (частично) | 200, 404, 422 |
| `DELETE` | `/dialogs/:id` | удалить | 204, 404 |
| `POST` | `/dialogs/:id/messages` | отправить сообщение, получить ответ LLM | 201, 404, 422, 502 |
| `GET` | `/dialogs/:id/messages` | история сообщений диалога | 200, 404 |

`user_id` в `GET /dialogs` обязателен. `limit` по умолчанию — 50.

### Форматы

```jsonc
// POST /api/v1/dialogs
{ "user_id": 7, "title": "Обсуждение задачи" }   // title опционален, ≤ 200 символов

// PATCH /api/v1/dialogs/1
{ "title": "Новое имя" }                          // поля-указатели: отсутствие = не менять

// ответ (DialogResponse)
{ "id": 1, "user_id": 7, "title": "...", "created_at": "...", "updated_at": "..." }
```

## Сообщения и LLM

Модель `DialogMessage` (id, `dialog_id` → `dialogs` c `ON DELETE CASCADE`,
`role`, `content`, `created_at`). Роли: `user`, `assistant`, `system`.

`POST /dialogs/:id/messages` (`{"text": "..."}`, ≤ 8000 символов):

1. проверяет существование диалога (иначе 404);
2. читает историю (до 100 последних сообщений);
3. прогоняет машину состояний диалога (`service/machine.go`) — внутри
   состояния `agent` вызов chat-модели через `internal/infrastructure/llm`
   (langchaingo, провайдер OpenAI за интерфейсом `llms.Model`);
4. **при успешном ответе** одной транзакцией сохраняет сообщение
   пользователя и финальный ответ ассистента, возвращает его (201).

Поведение **атомарное**: если вызов LLM завершился ошибкой — ответ `502`
(`{"error":"llm request failed"}`), в БД ничего не пишется. Если
`OPENAI_API_KEY` не задан — `502 {"error":"llm not configured"}`
(приложение при этом стартует, `/health` и CRUD работают).

Конфигурация: `OPENAI_API_KEY` (обязателен для чата), `OPENAI_MODEL`
(по умолчанию `gpt-4o-mini`) — см. [configuration.md](configuration.md).

### Машина состояний и tool calling

Шаг 3 — прогон машины состояний ([dialog-state-machine.md](dialog-state-machine.md)).
Пока одно состояние `agent`, внутри которого `llm.GenerateWithTools`:
модель может вызвать инструмент-функцию (например `get_current_time`), его
результат возвращается модели, и она формирует финальный ответ. Один раунд
без рекурсии; промежуточный обмен в `dialog_messages` не сохраняется.
Как добавить инструмент — [tool-calling.md](tool-calling.md).

## Обработка ошибок

Доменные ошибки описаны через `internal/apperr` (категории `NotFound`,
`Validation`, `Conflict`, `Internal`). HTTP-обработчик
(`internal/infrastructure/httpserver`) сопоставляет категорию с кодом:

| Категория | Код | Тело |
|-----------|-----|------|
| `NotFound` | 404 | `{"error":"dialog not found"}` |
| `Validation` | 422 | `{"error":"<детали>"}` |
| `Conflict` | 409 | `{"error":"<сообщение>"}` |
| `Upstream` | 502 | `{"error":"<сообщение>"}` (сбой внешнего сервиса, напр. LLM; логируется ERROR) |
| `Internal` / прочее | 500 | `{"error":"internal error"}` (полный текст — в лог ERROR) |

Хендлеры не пишут тело ошибки сами — кладут её через `c.Error(err)` и
выходят, middleware формирует ответ. Полный `application/problem+json`
появится на вехе «Устойчивость и наблюдаемость».

## Пример

```bash
curl -s -XPOST http://localhost:8080/api/v1/dialogs \
  -H 'Content-Type: application/json' \
  -d '{"user_id": 7, "title": "первый"}'
# {"id":1,"user_id":7,"title":"первый","created_at":"...","updated_at":"..."}

curl -s 'http://localhost:8080/api/v1/dialogs?user_id=7'
```

## Тесты

```bash
docker compose up -d postgres
docker compose run --rm tests
```

Покрыты: репозиторий (CRUD, not-found, фильтрация/сортировка `List`) и
хендлеры (полный CRUD-цикл, валидация) — против реального Postgres.

## Вне объёма (следующие вехи)

- Streaming-ответы (SSE), RAG-контекст — отдельные вехи.
- Многошаговые агентные циклы (рекурсивный tool calling) — веха «Диалог как state machine».
- Аутентификация: `user_id` пока приходит в запросе — веха «API-ключи и контексты доступа».

## See Also

- [БД и миграции](db.md) — GORM engine, транзакции, миграции
- [Tool calling](tool-calling.md) — вызов инструментов моделью
- [Машина состояний диалога](dialog-state-machine.md) — `dialogState`, состояние `agent`, `Run`
- [Архитектура](../.ai-factory/ARCHITECTURE.md) — паттерн Structured Modules
