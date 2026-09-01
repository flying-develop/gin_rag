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
3. вызывает chat-модель через `internal/infrastructure/llm` (langchaingo,
   провайдер OpenAI за интерфейсом `llms.Model`);
4. **при успешном ответе** одной транзакцией сохраняет сообщение
   пользователя и ответ ассистента, возвращает ответ ассистента (201).

Поведение **атомарное**: если вызов LLM завершился ошибкой — ответ `502`
(`{"error":"llm request failed"}`), в БД ничего не пишется. Если
`OPENAI_API_KEY` не задан — `502 {"error":"llm not configured"}`
(приложение при этом стартует, `/health` и CRUD работают).

Конфигурация: `OPENAI_API_KEY` (обязателен для чата), `OPENAI_MODEL`
(по умолчанию `gpt-4o-mini`) — см. [configuration.md](configuration.md).

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

- Streaming-ответы (SSE), tool calling, RAG-контекст — отдельные вехи.
- Аутентификация: `user_id` пока приходит в запросе — веха «API-ключи и контексты доступа».

## See Also

- [БД и миграции](db.md) — GORM engine, транзакции, миграции
- [Архитектура](../.ai-factory/ARCHITECTURE.md) — паттерн Structured Modules
