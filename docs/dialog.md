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

## Обработка ошибок

Доменные ошибки описаны через `internal/apperr` (категории `NotFound`,
`Validation`, `Conflict`, `Internal`). HTTP-обработчик
(`internal/infrastructure/httpserver`) сопоставляет категорию с кодом:

| Категория | Код | Тело |
|-----------|-----|------|
| `NotFound` | 404 | `{"error":"dialog not found"}` |
| `Validation` | 422 | `{"error":"<детали>"}` |
| `Conflict` | 409 | `{"error":"<сообщение>"}` |
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

- `DialogMessage` и интеграция с LLM — веха «Диалоги с LLM».
- Аутентификация: `user_id` пока приходит в запросе — веха «API-ключи и контексты доступа».

## See Also

- [БД и миграции](db.md) — GORM engine, транзакции, миграции
- [Архитектура](../.ai-factory/ARCHITECTURE.md) — паттерн Structured Modules
