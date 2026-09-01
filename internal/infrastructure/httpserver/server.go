// Package httpserver собирает HTTP-движок Gin: общие middleware
// (recovery, структурный access-лог), служебные эндпоинты (/health) и
// точку монтирования доменных модулей.
package httpserver

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/flying-develop/ai-app-go/internal/infrastructure/config"
)

// New создаёт и настраивает Gin-движок.
//
// Доменные модули (dialog, rag, ...) регистрируют свои роуты поверх
// возвращённого движка на следующих вехах.
func New(cfg *config.Config, logger *slog.Logger) *gin.Engine {
	gin.SetMode(ginMode(cfg.LogLevel))

	engine := gin.New()
	engine.Use(accessLog(logger), gin.Recovery())

	engine.GET("/health", func(c *gin.Context) {
		logger.Debug("health check requested", slog.String("client_ip", c.ClientIP()))
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	return engine
}

// accessLog — middleware структурного лога каждого HTTP-запроса.
func accessLog(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		logger.Info("http request",
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", c.Writer.Status()),
			slog.Duration("latency", time.Since(start)),
			slog.String("client_ip", c.ClientIP()),
		)
	}
}

// ginMode выбирает режим Gin по уровню логирования: debug-режим только
// при LOG_LEVEL=DEBUG, иначе release.
func ginMode(logLevel string) string {
	if strings.EqualFold(strings.TrimSpace(logLevel), "DEBUG") {
		return gin.DebugMode
	}
	return gin.ReleaseMode
}
