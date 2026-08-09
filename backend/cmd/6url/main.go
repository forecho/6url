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

	"github.com/forecho/6url/backend/internal/config"
	"github.com/forecho/6url/backend/internal/database"
	"github.com/forecho/6url/backend/internal/server"
)

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}
	client, sqlDB, err := database.Open(cfg)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer client.Close()
	defer sqlDB.Close()

	httpServer := &http.Server{Addr: ":" + cfg.HTTPPort, Handler: server.New(server.Dependencies{Config: cfg, Ent: client, SQL: sqlDB}), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		log.Printf("6URL API listening on :%s", cfg.HTTPPort)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}
