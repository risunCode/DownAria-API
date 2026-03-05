package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"downaria-api/internal/app"
	"downaria-api/internal/core/config"
	"downaria-api/internal/shared/logger"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load(".env.local", ".env")

	cfg := config.Load()
	if cfg.WebInternalSharedSecret == "" {
		log.Fatal("WEB_INTERNAL_SHARED_SECRET is required and must not be empty")
	}
	application := app.New(cfg)
	sigCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	go func() {
		<-sigCtx.Done()
		logger.Info("Shutdown signal received, initiating graceful shutdown...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := application.Stop(shutdownCtx); err != nil {
			logger.Error("Graceful shutdown finished with errors", "error", err)
		} else {
			logger.Info("Graceful shutdown completed successfully")
		}
	}()

	logger.Info("Starting DownAria-API", "port", cfg.Port)
	if err := application.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("Server failed to start", "error", err)
		os.Exit(1)
	}
}
