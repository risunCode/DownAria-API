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
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := application.Stop(shutdownCtx); err != nil {
			log.Printf("graceful shutdown finished with errors: %v", err)
		}
	}()

	log.Printf("starting DownAria-API on :%s", cfg.Port)
	if err := application.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server failed: %v", err)
	}
}
