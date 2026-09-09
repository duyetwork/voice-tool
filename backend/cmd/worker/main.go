// Command worker chạy Asynq consumer: Core Engine (voice:process,
// voice:publish), vòng quét liên tục của Breaking, và scheduled:scan.
//
// Chạy được NHIỀU process worker song song — mọi task đều idempotent
// (ClaimSourcePostForProcessing, asynq.Unique cho scan). Việc phát lịch nằm ở
// cmd/scheduler và CHỈ chạy 1 process.
package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"

	"github.com/strongbody/voice-tool/backend/internal/app"
	"github.com/strongbody/voice-tool/backend/internal/config"
	"github.com/strongbody/voice-tool/backend/internal/worker"
	"github.com/strongbody/voice-tool/backend/internal/worker/task"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("worker: %v", err)
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
	logger := application.Logger

	handler := worker.NewHandler(worker.HandlerDeps{
		Queries:      application.Queries,
		Engine:       application.Engine,
		Scan:         application.Scan,
		Maintenance:  application.Maintenance,
		Client:       application.AsynqClient,
		Logger:       logger,
		ScanInterval: cfg.BreakingScanInterval,
		Parallelism:  cfg.BreakingScanParallelism,
	})

	srv := asynq.NewServer(application.Redis, asynq.Config{
		Concurrency: cfg.WorkerConcurrcy,
		// Breaking ưu tiên cao nhất (F2 đổi lấy tốc độ), scheduled thấp nhất.
		Queues: map[string]int{
			task.QueueCritical: 6,
			task.QueueDefault:  3,
			task.QueueLow:      1,
		},
		RetryDelayFunc: asynq.RetryDelayFunc(task.RetryDelay),
		ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, t *asynq.Task, err error) {
			retried, _ := asynq.GetRetryCount(ctx)
			maxRetry, _ := asynq.GetMaxRetry(ctx)
			logger.ErrorContext(ctx, "task thất bại",
				"type", t.Type(), "error", err, "retried", retried, "max_retry", maxRetry)
		}),
		ShutdownTimeout: 30 * time.Second,
	})

	errCh := make(chan error, 1)
	go func() {
		logger.Info("worker bắt đầu", "concurrency", cfg.WorkerConcurrcy)
		if err := srv.Run(handler.Mux()); err != nil {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("nhận signal, đang tắt worker...")
	}

	srv.Shutdown()
	return nil
}
