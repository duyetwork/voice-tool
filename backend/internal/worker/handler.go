package worker

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/repository"
	"github.com/strongbody/voice-tool/backend/internal/service"
	"github.com/strongbody/voice-tool/backend/internal/worker/task"
)

// Handler cài đặt các consumer của Asynq.
type Handler struct {
	q           *repository.Queries
	engine      *service.Engine
	scan        *service.Scan
	maintenance *service.Maintenance
	client      *asynq.Client
	log         *slog.Logger

	scanInterval time.Duration
	parallelism  int
}

type HandlerDeps struct {
	Queries      *repository.Queries
	Engine       *service.Engine
	Scan         *service.Scan
	Maintenance  *service.Maintenance
	Client       *asynq.Client
	Logger       *slog.Logger
	ScanInterval time.Duration
	Parallelism  int
}

func NewHandler(d HandlerDeps) *Handler {
	interval := d.ScanInterval
	if interval <= 0 {
		interval = time.Minute
	}
	parallelism := d.Parallelism
	if parallelism <= 0 {
		parallelism = 4
	}
	return &Handler{
		q: d.Queries, engine: d.Engine, scan: d.Scan, maintenance: d.Maintenance,
		client: d.Client, log: d.Logger,
		scanInterval: interval, parallelism: parallelism,
	}
}

// Mux đăng ký toàn bộ task type xử lý trong worker.
func (h *Handler) Mux() *asynq.ServeMux {
	mux := asynq.NewServeMux()
	mux.HandleFunc(task.TypeVoiceProcess, h.voiceProcess)
	mux.HandleFunc(task.TypeVoiceText, h.voiceText)
	mux.HandleFunc(task.TypePostMetadata, h.postMetadata)
	mux.HandleFunc(task.TypeVoicePublish, h.voicePublish)
	mux.HandleFunc(task.TypeBreakingDispatch, h.breakingDispatch)
	mux.HandleFunc(task.TypeBreakingScan, h.breakingScan)
	mux.HandleFunc(task.TypeScheduledScan, h.scheduledScan)
	mux.HandleFunc(task.TypeMaintenanceCleanup, h.maintenanceCleanup)
	mux.HandleFunc(task.TypeScrapeSweep, h.scrapeSweep)
	return mux
}

// ---------------------------------------------------------------------------
// voice:process / voice:publish
// ---------------------------------------------------------------------------

func (h *Handler) voiceProcess(ctx context.Context, t *asynq.Task) error {
	p, err := task.Decode[task.VoiceProcessPayload](t)
	if err != nil {
		return err
	}
	postID, actor, err := parseIDs(p.SourcePostID, p.ActorID)
	if err != nil {
		return err
	}
	// Payload cũ (enqueue trước khi có trạng thái processing) không có voice_id
	// -> uuid.Nil, engine sẽ tự tạo record.
	var voiceID uuid.UUID
	if p.VoiceID != "" {
		if voiceID, err = uuid.Parse(p.VoiceID); err != nil {
			return fmt.Errorf("%w: voice_id %q: %v", asynq.SkipRetry, p.VoiceID, err)
		}
	}

	h.log.InfoContext(ctx, "voice:process bắt đầu", "source_post_id", postID, "voice_id", voiceID)
	if err := h.engine.ProcessSourcePost(ctx, postID, actor, voiceID); err != nil {
		return skipIfPermanent(err)
	}
	return nil
}

// voiceText đọc đoạn text gõ tay -> Voice, không qua Bài Post.
func (h *Handler) voiceText(ctx context.Context, t *asynq.Task) error {
	p, err := task.Decode[task.VoiceTextPayload](t)
	if err != nil {
		return err
	}
	voiceID, actor, err := parseIDs(p.VoiceID, p.ActorID)
	if err != nil {
		return err
	}

	h.log.InfoContext(ctx, "voice:text bắt đầu", "voice_id", voiceID, "skip_rewrite", p.SkipRewrite)
	if err := h.engine.ProcessTextVoice(ctx, voiceID, actor, p.SkipRewrite); err != nil {
		return skipIfPermanent(err)
	}
	return nil
}

// postMetadata lấy metadata gốc của Bài Post — chạy ngay sau khi tạo, độc lập
// với việc tạo Voice.
func (h *Handler) postMetadata(ctx context.Context, t *asynq.Task) error {
	p, err := task.Decode[task.PostMetadataPayload](t)
	if err != nil {
		return err
	}
	postID, err := uuid.Parse(p.SourcePostID)
	if err != nil {
		return fmt.Errorf("%w: source_post_id %q: %v", asynq.SkipRetry, p.SourcePostID, err)
	}
	if err := h.engine.FetchPostMetadata(ctx, postID); err != nil {
		return skipIfPermanent(err)
	}
	return nil
}

func (h *Handler) voicePublish(ctx context.Context, t *asynq.Task) error {
	p, err := task.Decode[task.VoicePublishPayload](t)
	if err != nil {
		return err
	}
	voiceID, actor, err := parseIDs(p.VoiceID, p.ActorID)
	if err != nil {
		return err
	}

	h.log.InfoContext(ctx, "voice:publish bắt đầu", "voice_id", voiceID)
	if err := h.engine.PublishVoice(ctx, voiceID, actor); err != nil {
		return skipIfPermanent(err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// breaking:dispatch / breaking:scan — quét liên tục, không cron
// ---------------------------------------------------------------------------

// breakingDispatch phát task quét cho các kênh ĐÃ TỚI HẠN rồi tự enqueue lại
// chính nó (business rule #4). Kênh nào có scan_interval riêng thì theo kênh,
// không có thì theo BREAKING_SCAN_INTERVAL của hệ thống.
func (h *Handler) breakingDispatch(ctx context.Context, _ *asynq.Task) error {
	defer h.rearmDispatch(ctx)

	lists, err := h.q.ListDueListBreakings(ctx, pgtype.Interval{
		Microseconds: h.scanInterval.Microseconds(),
		Valid:        true,
	})
	if err != nil {
		return fmt.Errorf("đọc danh sách breaking tới hạn: %w", err)
	}
	if len(lists) == 0 {
		return nil
	}

	// Giới hạn số kênh quét song song để không đụng rate-limit nền tảng.
	sem := make(chan struct{}, h.parallelism)
	var wg sync.WaitGroup

	now := time.Now()
	var outside int

	for _, list := range lists {
		// Ngoài khung giờ của kênh thì không quét. Lọc ở ĐÂY chứ không trong
		// ScanBreaking: nút "Chạy ngay" trên UI enqueue thẳng breaking:scan, và
		// đó là yêu cầu tường minh của người dùng — im lặng bỏ qua nó thì họ bấm
		// mà không thấy gì xảy ra, cũng không biết vì sao.
		if !breakingSchedule(list).Allows(now) {
			outside++
			continue
		}

		t, err := task.NewBreakingScan(task.BreakingScanPayload{ListID: list.ID.String()})
		if err != nil {
			h.log.ErrorContext(ctx, "tạo task breaking:scan thất bại", "error", err, "list_id", list.ID)
			continue
		}

		wg.Add(1)
		go func(listID uuid.UUID, t *asynq.Task) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// Unique theo khoảng nghỉ: vòng quét trước còn đang chạy thì không
			// dồn thêm task cho cùng 1 kênh.
			//
			// ProcessIn rải lệch từng kênh trong cửa sổ jitter: không có nó thì
			// mọi kênh tới hạn cùng vòng dispatch sẽ bắn trong cùng một giây, và
			// nền tảng chỉ thấy một IP gọi dồn dập.
			if _, err := h.client.EnqueueContext(ctx, t,
				asynq.Unique(h.scanInterval),
				asynq.ProcessIn(service.ScanJitter(listID.String(), h.jitterWindow())),
			); err != nil {
				if err == asynq.ErrDuplicateTask || err == asynq.ErrTaskIDConflict {
					return
				}
				h.log.ErrorContext(ctx, "enqueue breaking:scan thất bại", "error", err, "list_id", listID)
			}
		}(list.ID, t)
	}
	wg.Wait()

	h.log.DebugContext(ctx, "breaking:dispatch xong",
		"due_lists", len(lists), "ngoài_khung_giờ", outside)
	return nil
}

// jitterWindow: rải lệch trong tối đa 1/4 khoảng nghỉ, trần 30 giây. Rộng hơn
// thì kênh "breaking" mất đúng cái nó có giá trị nhất là độ nhanh.
func (h *Handler) jitterWindow() time.Duration {
	window := h.scanInterval / 4
	return min(window, 30*time.Second)
}

// breakingSchedule dựng khung giờ của 1 kênh Breaking. Kênh không cấu hình gì
// thì mọi trường là zero value = quét 24/7, đúng hành vi trước migration 000017.
func breakingSchedule(l repository.ListBreaking) domain.ChannelSchedule {
	return domain.ChannelSchedule{
		Timezone: l.Timezone,
		FromMin:  l.ActiveFromMin,
		ToMin:    l.ActiveToMin,
		Weekdays: l.ActiveWeekdays,
	}
}

func (h *Handler) rearmDispatch(ctx context.Context) {
	t, err := task.NewBreakingDispatch()
	if err != nil {
		h.log.ErrorContext(ctx, "tạo task breaking:dispatch thất bại", "error", err)
		return
	}
	// context của task đã hết hạn sau khi handler trả về -> dùng context mới.
	enqueueCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()

	if _, err := h.client.EnqueueContext(enqueueCtx, t,
		asynq.ProcessIn(h.scanInterval),
		asynq.Unique(h.scanInterval),
	); err != nil && err != asynq.ErrDuplicateTask {
		h.log.ErrorContext(ctx, "không re-arm được breaking:dispatch", "error", err)
	}
}

func (h *Handler) breakingScan(ctx context.Context, t *asynq.Task) error {
	p, err := task.Decode[task.BreakingScanPayload](t)
	if err != nil {
		return err
	}
	listID, err := uuid.Parse(p.ListID)
	if err != nil {
		return fmt.Errorf("%w: list_id không hợp lệ", asynq.SkipRetry)
	}

	res, err := h.scan.ScanBreaking(ctx, listID, service.ScanTriggerFromActorID(p.ActorID))
	if err != nil {
		return skipIfPermanent(err)
	}
	h.log.InfoContext(ctx, "breaking:scan xong",
		"list_id", listID, "fetched", res.Fetched, "created", res.Created,
		"voices", res.Voices, "skipped", res.Skipped)
	return nil
}

// ---------------------------------------------------------------------------
// scheduled:scan — chạy theo scan_frequency riêng từng kênh
// ---------------------------------------------------------------------------

func (h *Handler) scheduledScan(ctx context.Context, t *asynq.Task) error {
	p, err := task.Decode[task.ScheduledScanPayload](t)
	if err != nil {
		return err
	}
	listID, err := uuid.Parse(p.ListID)
	if err != nil {
		return fmt.Errorf("%w: list_id không hợp lệ", asynq.SkipRetry)
	}

	// Ngoài khung giờ của kênh thì bỏ vòng này. Chặn ở handler chứ không ở
	// cronspec vì cron không diễn tả được mọi cấu hình (khung giờ vắt qua nửa
	// đêm, tần suất không chia hết cho 60 phút) — và một chỗ chặn thì không có
	// cửa cho hai chỗ nói khác nhau.
	list, err := h.q.GetListScheduled(ctx, listID)
	if err != nil {
		return fmt.Errorf("đọc list_scheduled %s: %w", listID, err)
	}
	sched := scheduledSchedule(list)
	if !sched.Allows(time.Now()) {
		h.log.DebugContext(ctx, "scheduled:scan bỏ qua — ngoài khung giờ của kênh",
			"list_id", listID, "schedule", sched.Describe())
		return nil
	}

	res, err := h.scan.ScanScheduled(ctx, listID, service.ScanTriggerFromActorID(p.ActorID))
	if err != nil {
		return skipIfPermanent(err)
	}
	h.log.InfoContext(ctx, "scheduled:scan xong",
		"list_id", listID, "fetched", res.Fetched, "created", res.Created,
		"voices", res.Voices, "skipped", res.Skipped)
	return nil
}

// scheduledSchedule dựng khung giờ của 1 kênh Định kỳ.
func scheduledSchedule(l repository.ListScheduled) domain.ChannelSchedule {
	return domain.ChannelSchedule{
		Timezone:   l.Timezone,
		FromMin:    l.ActiveFromMin,
		ToMin:      l.ActiveToMin,
		Weekdays:   l.ActiveWeekdays,
		FixedTimes: l.FixedTimesMin,
	}
}

// ---------------------------------------------------------------------------
// maintenance:cleanup
// ---------------------------------------------------------------------------

func (h *Handler) maintenanceCleanup(ctx context.Context, _ *asynq.Task) error {
	if h.maintenance == nil {
		return nil
	}
	if _, err := h.maintenance.CleanupSkippedLogs(ctx); err != nil {
		return err
	}
	// Bảng trên hỏng thì dừng luôn ở đây: job này chạy lại mỗi ngày, và dọn
	// muộn một ngày không phải vấn đề — dữ liệu chỉ phình thêm một ngày.
	if _, err := h.maintenance.CleanupAIUsage(ctx); err != nil {
		return err
	}
	if _, err := h.maintenance.CleanupScanRuns(ctx); err != nil {
		return err
	}
	return nil
}

// scrapeSweep bảo trì hạ tầng via/proxy — xem Maintenance.SweepScrapeInfra.
func (h *Handler) scrapeSweep(ctx context.Context, _ *asynq.Task) error {
	if h.maintenance == nil {
		return nil
	}
	return h.maintenance.SweepScrapeInfra(ctx)
}

// ---------------------------------------------------------------------------

func parseIDs(objectID, actorID string) (uuid.UUID, uuid.UUID, error) {
	id, err := uuid.Parse(objectID)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("%w: object id không hợp lệ", asynq.SkipRetry)
	}
	actor, err := uuid.Parse(actorID)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("%w: actor id không hợp lệ", asynq.SkipRetry)
	}
	return id, actor, nil
}

// skipIfPermanent chặn retry với lỗi vĩnh viễn (URL chết, cấu hình sai...).
func skipIfPermanent(err error) error {
	if domain.IsPermanent(err) {
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}
	return err
}
