[Back to README](../README.md) · [Конфигурация →](configuration.md)

# Быстрый старт

## Предварительные требования

- [Docker](https://docs.docker.com/get-docker/) + Docker Compose (v2, команда `docker compose`)
- Для сборки без Docker: Go 1.25+

## Запуск через Docker (рекомендуется)

```bash
cp .env.example .env
docker compose up -d --build
```

Поднимутся 4 сервиса: `app` (Gin на порту `APP_HOST_PORT`, по умолчанию
8080), `postgres`, `redis`, `qdrant`.

Проверить, что всё работает:

```bash
curl http://localhost:8080/health
# {"status":"ok"}

# CRUD по диалогам (см. docs/dialog.md)
curl -s -XPOST http://localhost:8080/api/v1/dialogs \
  -H 'Content-Type: application/json' -d '{"user_id":7,"title":"первый"}'

docker compose ps
# app и postgres должны быть в статусе "healthy"
```

Применить миграции БД:

```bash
docker compose run --rm app migrate up
```

Логи приложения (структурированный формат `key=value`):

```bash
docker compose logs -f app
```

Остановить стек:

```bash
docker compose down
```

### Порт 8080 занят

`app` пробрасывается на хост-порт `APP_HOST_PORT` (см. `.env`). Если
8080 занят другим процессом — поставьте, например, `APP_HOST_PORT=8090`
и повторите `docker compose up -d`. Внутри контейнера сервис всегда
слушает 8080.

## Локальный запуск без Docker

БД/Redis/Qdrant всё равно нужны отдельно (например,
`docker compose up -d postgres redis qdrant`).

```bash
cp .env.example .env
# отредактируйте .env — хосты postgres/redis/qdrant заменить на localhost
go run ./cmd/api
```

```bash
curl http://localhost:8080/health
# {"status":"ok"}
```

## Сборка и проверки

На хосте без Go — через контейнер:

```bash
docker run --rm -v "$PWD":/app -w /app -e GOFLAGS=-buildvcs=false golang:1.25 \
  sh -c "gofmt -l . && go vet ./... && go build ./..."
```

Тесты (нужен запущенный `postgres`):

```bash
docker compose up -d postgres
docker compose run --rm tests
```

## Следующие шаги

Дальнейшие возможности (работа с БД, диалоги с LLM, RAG, ...) появляются
поэтапно — см. [Roadmap](../.ai-factory/ROADMAP.md).

## See Also

- [Конфигурация](configuration.md) — переменные окружения
- [Архитектура](../.ai-factory/ARCHITECTURE.md) — структура проекта
