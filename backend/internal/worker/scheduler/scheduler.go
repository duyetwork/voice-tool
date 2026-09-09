// Package scheduler đăng ký lịch chạy vào Asynq PeriodicTaskManager.
//
// Cấu hình lấy trực tiếp từ bảng list_scheduled nên khi user sửa
// scan_frequency, lịch tự cập nhật ở lần sync kế tiếp (business rule #5).
//
// LƯU Ý VẬN HÀNH: chỉ chạy ĐÚNG 1 process scheduler. Mỗi PeriodicTaskManager
// tự enqueue theo cronspec của nó, nên 2 process = task bị nhân đôi.
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strongbody/voice-tool/backend/internal/repository"
	"github.com/strongbody/voice-tool/backend/internal/worker/task"
)

// cleanupCronspec: dọn skipped_log mỗi ngày lúc 03:15 (giờ của container).
const cleanupCronspec = "15 3 * * *"

// Provider cài đặt asynq.PeriodicTaskConfigProvider.
type Provider struct {
	q   *repository.Queries
	log *slog.Logger
}

var _ asynq.PeriodicTaskConfigProvider = (*Provider)(nil)

func NewProvider(q *repository.Queries, log *slog.Logger) *Provider {
	return &Provider{q: q, log: log}
}

func (p *Provider) GetConfigs() ([]*asynq.PeriodicTaskConfig, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	lists, err := p.q.ListActiveListScheduleds(ctx)
	if err != nil {
		return nil, fmt.Errorf("đọc list_scheduled active: %w", err)
	}

	configs := make([]*asynq.PeriodicTaskConfig, 0, len(lists)+1)
	for _, list := range lists {
		spec, err := cronSpec(list.ScanFrequency)
		if err != nil {
			p.log.Warn("bỏ qua lịch không hợp lệ", "error", err, "list_id", list.ID)
			continue
		}
		t, err := task.NewScheduledScan(task.ScheduledScanPayload{ListID: list.ID.String()})
		if err != nil {
			p.log.Warn("tạo task scheduled:scan thất bại", "error", err, "list_id", list.ID)
			continue
		}
		configs = append(configs, &asynq.PeriodicTaskConfig{Cronspec: spec, Task: t})
	}

	// Job dọn dẹp chạy hằng ngày.
	if cleanup, err := task.NewMaintenanceCleanup(); err == nil {
		configs = append(configs, &asynq.PeriodicTaskConfig{Cronspec: cleanupCronspec, Task: cleanup})
	} else {
		p.log.Warn("tạo task maintenance:cleanup thất bại", "error", err)
	}

	return configs, nil
}

// New tạo PeriodicTaskManager. Gọi Run() trong goroutine riêng ở cmd/scheduler.
func New(redis asynq.RedisConnOpt, p *Provider, syncInterval time.Duration) (*asynq.PeriodicTaskManager, error) {
	if syncInterval <= 0 {
		syncInterval = 30 * time.Second
	}
	return asynq.NewPeriodicTaskManager(asynq.PeriodicTaskManagerOpts{
		RedisConnOpt:               redis,
		PeriodicTaskConfigProvider: p,
		SyncInterval:               syncInterval,
	})
}

// cronSpec đổi INTERVAL của Postgres sang cronspec. Asynq hỗ trợ cú pháp
// "@every <duration>" nên tần suất tự do (vd 37 phút) vẫn dùng được
// (specs mục 5, câu 4).
func cronSpec(freq pgtype.Interval) (string, error) {
	d := intervalDuration(freq)
	if d < time.Minute {
		return "", fmt.Errorf("scan_frequency %s nhỏ hơn 1 phút", d)
	}
	return "@every " + d.String(), nil
}

func intervalDuration(i pgtype.Interval) time.Duration {
	if !i.Valid {
		return 0
	}
	const day = 24 * time.Hour
	return time.Duration(i.Microseconds)*time.Microsecond +
		time.Duration(i.Days)*day +
		time.Duration(i.Months)*30*day
}
