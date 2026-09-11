// Command api chạy HTTP API. Handler chỉ CRUD + enqueue job; mọi xử lý nặng
// nằm ở cmd/worker (business rule #10).
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/strongbody/voice-tool/backend/internal/app"
	"github.com/strongbody/voice-tool/backend/internal/config"
	transporthttp "github.com/strongbody/voice-tool/backend/internal/transport/http"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("api: %v", err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	application, err := app.New(ctx, cfg)
	if err != nil {
		return err
	}
	defer application.Close()

	router := transporthttp.NewRouter(transporthttp.RouterDeps{
		Config:     cfg,
		Logger:     application.Logger,
		Tokens:     application.Tokens,
		Auth:       application.Auth,
		SourcePost: application.SourcePost,
		Voice:      application.Voice,
		List:       application.List,
		Catalog:    application.Catalog,
		AIEngine:   application.AIEngine,
		Audit:      application.Audit,
		User:       application.User,

		MultimeUsers: application.MultimeUsers,
		Platforms:    application.Platforms,
		Modes:        application.Modes,
	})

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		application.Logger.Info("api đang lắng nghe", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		application.Logger.Info("nhận signal, đang tắt api...")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
