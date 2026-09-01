# Архитектура: Structured Modules (Technical Layer)

## Обзор

Лёгкая модульная архитектура: каждый домен (диалоги, RAG, task
pipeline, модерация, файлы) — отдельный модуль со своими роутами,
сервисами, репозиториями и моделями. Внутри модуля — деление по
техническим слоям (`handler/service/repository/model`), без полного
DDD-формализма (Domain/Application/Infrastructure/Presentation).

Это осознанный компромисс: домены проекта довольно независимы
(диалоги, RAG-поиск и rerank, несколько видов модерации, файлы), но
проект ведёт небольшая команда — полный Explicit Architecture на каждый
bounded context дал бы слишком много boilerplate на старте. Structured
Modules даёт чёткие границы между доменами и путь для постепенного
роста в сторону Explicit Architecture, если модуль разрастётся.

## Обоснование выбора

- **Стек:** Go 1.23+, Gin, GORM + golang-migrate, langchaingo,
  PostgreSQL, Qdrant, asynq + Redis.
- **Ключевой фактор:** несколько независимых доменов (диалоги, RAG,
  task pipeline, несколько видов модерации, файлы) при небольшой
  команде — нужны чёткие границы модулей, но без полного DDD-формализма.

## Структура папок

```
cmd/
├── api/
│   └── main.go                          # точка входа HTTP-сервиса: сборка зависимостей, роутинг, запуск Gin
└── worker/
    └── main.go                          # точка входа asynq-воркера, появляется на вехе фоновых задач

internal/
├── modules/                             # ── ДОМЕННЫЕ МОДУЛИ ──
│   ├── dialog/                          # Диалоги с LLM
│   │   ├── handler/
│   │   │   └── dialog_handler.go        # Gin-хендлеры модуля
│   │   ├── service/
│   │   │   ├── dialog_service.go        # use cases по Dialog
│   │   │   └── statemachine/            # явная state machine диалога
│   │   ├── repository/
│   │   │   └── dialog_repository.go     # ЕДИНСТВЕННЫЙ доступ к БД для модуля
│   │   ├── model/                       # GORM-модели модуля
│   │   │   └── dialog.go
│   │   └── dto/                         # структуры запросов/ответов модуля
│   │       └── dialog.go
│   │
│   ├── rag/                             # Индексация статей, поиск, rerank
│   │   ├── handler/
│   │   ├── service/
│   │   │   ├── chunker_service.go
│   │   │   ├── indexer_service.go
│   │   │   ├── retriever_service.go
│   │   │   └── rerank_service.go
│   │   ├── repository/
│   │   ├── model/
│   │   └── dto/
│   │
│   ├── tasks/                           # Task pipeline (Task, TaskStep, TaskResult)
│   │   ├── handler/
│   │   ├── service/
│   │   │   └── statemachine/            # state machine задач
│   │   ├── repository/
│   │   ├── model/
│   │   └── dto/
│   │
│   ├── moderation/                      # Конвейеры модерации контента
│   │   ├── handler/
│   │   ├── service/                     # по одному сервису на вид модерации
│   │   ├── repository/
│   │   ├── model/
│   │   └── dto/
│   │
│   └── files/                           # Приём, парсинг, хранение файлов
│       ├── handler/
│       ├── service/
│       ├── repository/
│       ├── model/
│       └── dto/
│
└── infrastructure/                      # ── ИНФРАСТРУКТУРА (сквозная) ──
    ├── config/                          # загрузка конфига из env
    ├── db/                              # GORM engine, пул соединений, хелперы транзакций
    ├── redis/                           # Redis-клиент, конфигурация asynq
    ├── qdrant/                          # Qdrant-клиент
    ├── llm/                             # инициализация langchaingo chat-моделей / embeddings
    ├── httpserver/                      # сборка Gin-движка, общие middleware, единый обработчик ошибок
    └── logging/                         # настройка log/slog

migrations/                              # SQL up/down файлы golang-migrate
```

Миграции живут в корневом `migrations/` (стандартно для golang-migrate),
а не внутри `internal/infrastructure/`.

`internal/` вместо `pkg/` — весь код приложения приватный, публичных
переиспользуемых библиотек проект не предоставляет.

## Правила зависимостей

- **Строгий поток вниз внутри модуля:** `handler → service → repository`.
  Внутренние слои (`repository`, `model`) никогда не зависят от внешних
  (`handler`).
- **Без пропуска слоёв:** хендлеры (`handler/`) не обращаются к
  `repository` напрямую, только через `service`.
- **Изоляция модулей:** модули могут зависеть от корневого
  `internal/infrastructure/` (БД, Redis, Qdrant, LLM, конфиг), но не
  лезут во внутренности друг друга. Межмодульное взаимодействие —
  только через публичный интерфейс сервиса другого модуля.
- ✅ `internal/modules/tasks/service` вызывает `moderation.Classifier`
  (интерфейс, реализованный `internal/modules/moderation/service`)
- ❌ `internal/modules/tasks/repository` импортирует что-либо из
  `internal/modules/moderation/repository`

## Взаимодействие слоёв/модулей

- Gin-хендлеры (`handler/`) валидируют вход через binding-теги / DTO,
  вызывают один-два метода сервиса, формируют ответ.
- Сервисы (`service/`) содержат бизнес-логику и оркестрацию, включая
  langchaingo-цепочки и state machine; зависимости получают через
  конструктор (`NewDialogService(repo, llm, logger)`).
- Репозитории (`repository/`) — единственная точка доступа к БД через
  GORM; инкапсулируют построение запросов, пагинацию и маппинг в
  модели/DTO. Бизнес-логики в репозитории нет.
- Транзакции инициируются на уровне сервиса (`db.Transaction(ctx, ...)`),
  все операции внутри транзакции идут через репозитории.
- Фоновые обработчики (asynq handlers в `cmd/worker`) вызывают те же
  сервисы, что и HTTP-хендлеры — бизнес-логика не дублируется между
  HTTP- и worker-путём.

## Ключевые принципы

1. **Границы модулей:** каждый модуль инкапсулирует один домен.
   У модуля есть публичный интерфейс (сервисы) — остальные модули
   используют только его.
2. **Инверсия зависимостей через интерфейсы:** интерфейс репозитория
   объявляется в пакете `service` (потребитель), реализация — в пакете
   `repository`. Идиоматично для Go и упрощает будущий переход к
   Explicit Architecture.
3. **Domain awareness:** сервисы — оркестраторы (Application Services);
   при росте бизнес-правил их выносят в отдельные доменные
   типы/функции внутри модуля, а не разрастают сервис.
4. **Infrastructure минимальна:** `internal/infrastructure/` содержит
   только сквозные технические concerns — никакой бизнес-логики.

## Code Organization Note

- **Новый функционал:** весь новый код следует структуре модулей из
  этого документа.
- **Существующий код:** проект применяет структуру с первого коммита,
  без legacy-исключений.

## Примеры кода

### Модель и репозиторий (GORM)

```go
// internal/modules/dialog/model/dialog.go
package model

import "time"

type Dialog struct {
	ID        uint      `gorm:"primaryKey"`
	UserID    uint      `gorm:"index"`
	Title     string
	CreatedAt time.Time
	UpdatedAt time.Time
}
```

```go
// internal/modules/dialog/service/ports.go
package service

import (
	"context"

	"github.com/flying-develop/ai-app-go/internal/modules/dialog/model"
)

// DialogRepository — интерфейс объявлен на стороне потребителя (сервиса).
type DialogRepository interface {
	FindByID(ctx context.Context, id uint) (*model.Dialog, error)
	Save(ctx context.Context, d *model.Dialog) error
}
```

```go
// internal/modules/dialog/repository/dialog_repository.go
package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/flying-develop/ai-app-go/internal/modules/dialog/model"
)

type DialogRepository struct {
	db *gorm.DB
}

func NewDialogRepository(db *gorm.DB) *DialogRepository {
	return &DialogRepository{db: db}
}

func (r *DialogRepository) FindByID(ctx context.Context, id uint) (*model.Dialog, error) {
	var d model.Dialog
	err := r.db.WithContext(ctx).First(&d, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *DialogRepository) Save(ctx context.Context, d *model.Dialog) error {
	return r.db.WithContext(ctx).Save(d).Error
}
```

### Сервис и Gin-хендлер

```go
// internal/modules/dialog/service/dialog_service.go
package service

import (
	"context"
	"fmt"

	"github.com/tmc/langchaingo/llms"

	"github.com/flying-develop/ai-app-go/internal/modules/dialog/model"
)

type DialogService struct {
	repo DialogRepository
	llm  llms.Model
}

func NewDialogService(repo DialogRepository, llm llms.Model) *DialogService {
	return &DialogService{repo: repo, llm: llm}
}

func (s *DialogService) SendMessage(ctx context.Context, dialogID uint, text string) (*model.DialogMessage, error) {
	dialog, err := s.repo.FindByID(ctx, dialogID)
	if err != nil {
		return nil, fmt.Errorf("load dialog %d: %w", dialogID, err)
	}
	if dialog == nil {
		return nil, ErrDialogNotFound
	}
	// ... вызов s.llm, сохранение сообщения через s.repo ...
	return nil, nil
}
```

```go
// internal/modules/dialog/handler/dialog_handler.go
package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/flying-develop/ai-app-go/internal/modules/dialog/dto"
)

type DialogHandler struct {
	svc DialogService // интерфейс сервиса, объявленный в этом пакете
}

func (h *DialogHandler) SendMessage(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid dialog id"})
		return
	}

	var req dto.SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	msg, err := h.svc.SendMessage(c.Request.Context(), uint(id), req.Text)
	if err != nil {
		_ = c.Error(err) // единый обработчик ошибок в middleware разберёт
		return
	}
	c.JSON(http.StatusOK, dto.NewMessageResponse(msg))
}

func (h *DialogHandler) Register(rg *gin.RouterGroup) {
	rg.POST("/dialogs/:id/messages", h.SendMessage)
}
```

## Антипаттерны

- ❌ **Доступ к БД в обход репозитория** — `db.First(...)` в сервисе
  или хендлере.
- ❌ **Пропуск слоёв** — хендлер, вызывающий репозиторий напрямую,
  минуя сервис.
- ❌ **Восходящие зависимости** — `repository/` или `model/`,
  импортирующие что-то из `service/` или `handler/`.
- ❌ **Циклические зависимости между модулями** — `tasks` импортирует
  внутренности `moderation`, а `moderation` — внутренности `tasks`.
  Использовать общие интерфейсы/DTO.
- ❌ **God-сервис** — один сервис на все use case'ы нескольких доменов.
- ❌ **Бизнес-логика в репозитории** — репозиторий только строит
  запросы и маппит результат.
