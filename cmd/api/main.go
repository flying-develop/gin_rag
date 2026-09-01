// Команда api — точка входа сервиса.
//
// Без аргументов запускает HTTP-сервер (Gin) с graceful shutdown по
// SIGINT/SIGTERM. Поддерживает подкоманды:
//
//	api healthcheck      — GET /health к локальному серверу, exit 0/1
//	                       (используется в HEALTHCHECK Docker-образа, где нет curl)
//	api migrate up|down  — применить / откатить миграции БД
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/flying-develop/ai-app-go/internal/infrastructure/config"
	"github.com/flying-develop/ai-app-go/internal/infrastructure/db"
	"github.com/flying-develop/ai-app-go/internal/infrastructure/httpserver"
	"github.com/flying-develop/ai-app-go/internal/infrastructure/logging"
)

// shutdownTimeout — сколько ждём завершения активных запросов при остановке.
const shutdownTimeout = 10 * time.Second

func main() {
	if err := dispatch(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// dispatch выбирает подкоманду по аргументам.
func dispatch(args []string) error {
	switch {
	case len(args) >= 1 && args[0] == "healthcheck":
		return healthcheck()
	case len(args) >= 2 && args[0] == "migrate":
		return runMigrate(args[1])
	default:
		return run()
	}
}

// run собирает HTTP-приложение и блокируется до сигнала остановки.
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := logging.Setup(cfg.LogLevel)

	// Контекст, отменяемый по SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Подключение к БД. На этой вехе ни один эндпоинт от БД не зависит,
	// поэтому недоступная БД при старте не фатальна — только ERROR-лог.
	gormDB, err := db.Open(ctx, cfg, logger)
	if err != nil {
		logger.Error("database unavailable at startup", slog.String("error", err.Error()))
	} else {
		defer func() {
			if cerr := db.Close(gormDB); cerr != nil {
				logger.Error("database close failed", slog.String("error", cerr.Error()))
			}
		}()
	}

	engine := httpserver.New(cfg, logger)

	srv := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Ошибка запуска ListenAndServe приходит по этому каналу.
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("server starting",
			slog.String("addr", srv.Addr),
			slog.String("app", cfg.AppName),
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}

	logger.Info("server stopped")
	return nil
}

// runMigrate применяет или откатывает миграции БД и завершает процесс.
func runMigrate(direction string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logging.Setup(cfg.LogLevel)

	if direction != db.DirectionUp && direction != db.DirectionDown {
		return fmt.Errorf("usage: api migrate up|down (получено %q)", direction)
	}
	return db.Migrate(cfg, direction)
}

// healthcheck делает GET /health к локальному серверу и возвращает ошибку,
// если сервис недоступен или ответил не 200.
func healthcheck() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + cfg.HTTPPort + "/health")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}
