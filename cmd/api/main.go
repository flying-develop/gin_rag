// Команда api — точка входа HTTP-сервиса.
//
// Здесь собираются зависимости (конфиг, логгер, HTTP-движок), запускается
// сервер и обрабатывается graceful shutdown по SIGINT/SIGTERM.
//
// Подкоманда `healthcheck` используется в HEALTHCHECK Docker-образа
// (в distroless-образе нет curl/wget): процесс делает GET /health и
// завершается с кодом 0 при 200 и 1 в противном случае.
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
	"github.com/flying-develop/ai-app-go/internal/infrastructure/httpserver"
	"github.com/flying-develop/ai-app-go/internal/infrastructure/logging"
)

// shutdownTimeout — сколько ждём завершения активных запросов при остановке.
const shutdownTimeout = 10 * time.Second

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := healthcheck(); err != nil {
			fmt.Fprintln(os.Stderr, "healthcheck:", err)
			os.Exit(1)
		}
		return
	}

	if err := run(); err != nil {
		slog.Error("fatal", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

// run собирает приложение и блокируется до сигнала остановки.
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := logging.Setup(cfg.LogLevel)

	engine := httpserver.New(cfg, logger)

	srv := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Контекст, отменяемый по SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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
