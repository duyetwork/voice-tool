// Command scheduler chỉ làm 1 việc: phát task theo lịch.
//
//  1. scheduled:scan   — theo scan_frequency riêng từng kênh (đọc từ DB)
//  2. maintenance:cleanup — dọn skipped_log hằng ngày
//  3. breaking:dispatch — mồi vòng lặp quét liên tục của Breaking
//
// CHỈ CHẠY 1 PROCESS. Nhiều process scheduler = task bị nhân đôi vì mỗi
// PeriodicTaskManager tự enqueue theo cronspec của nó.
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
	"github.com/strongbody/voice-tool/backend/internal/worker/scheduler"
	"github.com/strongbody/voice-tool/backend/internal/worker/task"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("scheduler: %v", err)
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

	periodic, err := scheduler.New(
		application.Redis,
		scheduler.NewProvider(application.Queries, logger),
		cfg.SchedulerSyncInterval,
	)
	if err != nil {
		return err
	}

	// Mồi vòng lặp quét liên tục của Breaking (business rule #4). Sau lần này
	// task tự enqueue lại chính nó ở worker.
	if err := armBreakingDispatch(ctx, application, cfg.BreakingScanInterval); err != nil {
		logger.Error("không mồi được breaking:dispatch", "error", err)
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("scheduler bắt đầu",
			"sync_interval", cfg.SchedulerSyncInterval.String(),
			"breaking_scan_interval", cfg.BreakingScanInterval.String())
		if err := periodic.Run(); err != nil {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("nhận signal, đang tắt scheduler...")
	}

	periodic.Shutdown()
	return nil
}

func armBreakingDispatch(ctx context.Context, application *app.App, interval time.Duration) error {
	t, err := task.NewBreakingDispatch()
	if err != nil {
		return err
	}
	if interval <= 0 {
		interval = time.Minute
	}
	// Unique tránh mồi trùng khi scheduler restart trong lúc vòng cũ còn chạy.
	_, err = application.AsynqClient.EnqueueContext(ctx, t, asynq.Unique(interval))
	if err != nil && err != asynq.ErrDuplicateTask {
		return err
	}
	return nil
}
