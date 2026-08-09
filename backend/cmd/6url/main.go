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

	"github.com/forecho/6url/backend/internal/backlink"
	"github.com/forecho/6url/backend/internal/config"
	"github.com/forecho/6url/backend/internal/database"
	"github.com/forecho/6url/backend/internal/notifications"
	"github.com/forecho/6url/backend/internal/ratelimit"
	"github.com/forecho/6url/backend/internal/revalidation"
	"github.com/forecho/6url/backend/internal/securepayload"
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
	limiter, err := ratelimit.NewRedis(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.RedisKeyPrefix)
	if err != nil {
		log.Fatalf("rate limiter initialization failed: %v", err)
	}
	defer limiter.Close()
	sealer, sealerErr := securepayload.New(cfg.NotificationEncryptionKey)
	if sealerErr != nil && cfg.Environment == "production" {
		log.Fatalf("notification encryption initialization failed: %v", sealerErr)
	}

	rootCtx, cancelRoot := context.WithCancel(context.Background())
	defer cancelRoot()
	var sealerDependency *securepayload.Sealer
	backlinkChecker := backlink.Client{}
	if sealerErr == nil {
		sealerDependency = &sealer
		go notifications.NewWorker(client, cfg, sealer).Run(rootCtx)
		go (backlink.Worker{Monitor: backlink.Monitor{Client: client, Checker: backlinkChecker, TargetURL: cfg.AppBaseURL, Sealer: &sealer, SlackChannelID: cfg.SlackChannelID}}).Run(rootCtx)
	} else {
		log.Printf("notification worker disabled: %v", sealerErr)
	}
	if cfg.RevalidationWebhookURL != "" && cfg.RevalidationSecret != "" {
		go (revalidation.Worker{Client: client, Endpoint: cfg.RevalidationWebhookURL, Secret: cfg.RevalidationSecret}).Run(rootCtx)
	}

	httpServer := &http.Server{Addr: ":" + cfg.HTTPPort, Handler: server.New(server.Dependencies{Config: cfg, Ent: client, SQL: sqlDB, RateLimiter: limiter, PayloadSealer: sealerDependency, Backlinks: backlinkChecker}), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		log.Printf("6URL API listening on :%s", cfg.HTTPPort)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	cancelRoot()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}
