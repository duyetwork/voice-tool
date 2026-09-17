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

	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/repository"
	"github.com/strongbody/voice-tool/backend/internal/worker/task"
)

// cleanupCronspec: dọn skipped_log mỗi ngày lúc 03:15 (giờ của container).
const cleanupCronspec = "15 3 * * *"

// scrapeSweepCronspec: bảo trì via/proxy mỗi giờ, ở phút thứ 7.
//
// Mỗi giờ chứ không mỗi ngày vì cooldown của via mặc định là 6 tiếng: chạy theo
// ngày thì via nghỉ xong vẫn nằm ngoài vòng xoay gần trọn một ngày nữa.
//
// Phút 7 để không rơi trúng đầu giờ, nơi phần lớn lịch quét của các kênh được
// rải vào — hai việc cùng chạy một lúc thì việc dọn dẹp làm chậm đúng lúc vòng
// quét đang cần DB.
const scrapeSweepCronspec = "7 * * * *"

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
		specs, err := listCronSpecs(list)
		if err != nil {
			p.log.Warn("bỏ qua lịch không hợp lệ", "error", err, "list_id", list.ID)
			continue
		}
		t, err := task.NewScheduledScan(task.ScheduledScanPayload{ListID: list.ID.String()})
		if err != nil {
			p.log.Warn("tạo task scheduled:scan thất bại", "error", err, "list_id", list.ID)
			continue
		}
		for _, spec := range specs {
			configs = append(configs, &asynq.PeriodicTaskConfig{Cronspec: spec, Task: t})
		}
	}

	// Job dọn dẹp chạy hằng ngày.
	if cleanup, err := task.NewMaintenanceCleanup(); err == nil {
		configs = append(configs, &asynq.PeriodicTaskConfig{Cronspec: cleanupCronspec, Task: cleanup})
	} else {
		p.log.Warn("tạo task maintenance:cleanup thất bại", "error", err)
	}

	// Bảo trì hạ tầng via/proxy, mỗi giờ.
	if sweep, err := task.NewScrapeSweep(); err == nil {
		configs = append(configs, &asynq.PeriodicTaskConfig{Cronspec: scrapeSweepCronspec, Task: sweep})
	} else {
		p.log.Warn("tạo task scrape:sweep thất bại", "error", err)
	}

	return configs, nil
}

// listCronSpecs chốt lịch của 1 kênh. Trả về NHIỀU cronspec vì kênh chạy theo
// giờ cố định cần mỗi mốc một dòng cron (xem fixedTimeSpecs).
//
// Hai kiểu lịch, GIỜ CỐ ĐỊNH THẮNG:
//
//   - Có fixed_times_min -> cron thật theo giờ đồng hồ, kèm CRON_TZ để 08:00 là
//     08:00 ở múi giờ của kênh chứ không phải của container. Với kênh đăng theo
//     giờ cố định, đây vừa đúng hơn vừa rẻ hơn hẳn "mỗi N phút".
//   - Không có -> "@every N" như cũ.
//
// Khung giờ hoạt động (active_from/to) KHÔNG nằm ở đây mà ở handler: cron không
// diễn tả được khung vắt qua nửa đêm hay tần suất không chia hết cho 60 phút, và
// để hai nơi cùng quyết định thì sớm muộn chúng nói khác nhau. Riêng `weekdays`
// thì cron diễn tả được trọn vẹn, nên đưa vào spec luôn để không phải đánh thức
// worker vào ngày kênh nghỉ.
func listCronSpecs(list repository.ListScheduled) ([]string, error) {
	sched := domain.ChannelSchedule{
		Timezone:   list.Timezone,
		Weekdays:   list.ActiveWeekdays,
		FixedTimes: list.FixedTimesMin,
	}
	if len(sched.FixedTimes) > 0 {
		return fixedTimeSpecs(sched), nil
	}
	spec, err := cronSpec(list.ScanFrequency)
	if err != nil {
		return nil, err
	}
	return []string{spec}, nil
}

// fixedTimeSpecs dựng 1 dòng cron cho MỖI mốc giờ:
// "CRON_TZ=<tz> <phút> <giờ> * * <thứ>".
//
// Mỗi mốc một dòng chứ không gộp: robfig/cron (bộ phân tích của asynq) nhận
// danh sách ở cả trường phút lẫn trường giờ, nên gộp 08:00/12:30/18:00 thành
// "0,30 8,12,18" sẽ sinh ra cả 08:30 và 12:00 — hai lần quét không ai yêu cầu.
func fixedTimeSpecs(sched domain.ChannelSchedule) []string {
	dow := "*"
	if len(sched.Weekdays) > 0 {
		dow = ""
		for i, d := range sched.Weekdays {
			if i > 0 {
				dow += ","
			}
			dow += fmt.Sprint(d)
		}
	}

	tz := sched.Location().String()
	specs := make([]string, 0, len(sched.FixedTimes))
	for _, m := range sched.FixedTimes {
		specs = append(specs, fmt.Sprintf("CRON_TZ=%s %d %d * * %s", tz, m%60, m/60, dow))
	}
	return specs
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
