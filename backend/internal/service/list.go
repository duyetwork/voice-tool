package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/pkg/validator"
	"github.com/strongbody/voice-tool/backend/internal/repository"
)

const (
	// minScanFrequency chặn cấu hình quét quá dày gây vượt rate-limit nền tảng
	// (specs mục 5, câu 3/4).
	//
	// 5 phút, không phải 1 phút: quét 1 phút/lần gần như không bao giờ bắt được
	// bài mới (kênh không đăng dày thế) nhưng lại nhân số request lên 5 lần, và
	// nền tảng nhìn vào chỉ thấy một IP máy chủ gọi liên tục — đúng cái kích
	// hoạt bot-check của YouTube và 429 của Facebook (xem infra/platform/
	// ytdlperr.go, nơi cả hai lỗi này đã phải có nhãn riêng vì đã gặp thật).
	// Bài mới vẫn được lấy đủ nhờ watermark last_synced_post_id, chỉ là biết
	// muộn hơn vài phút.
	minScanFrequency = 5 * time.Minute
	// minBreakingScanInterval thấp hơn vì Breaking ưu tiên tốc độ.
	minBreakingScanInterval = 15 * time.Second
	// maxRegexPatterns chặn 1 kênh có quá nhiều pattern (mỗi vòng quét phải
	// chạy hết tất cả pattern trên tất cả bài).
	maxRegexPatterns = 20
	// maxScanLimit khớp CHECK constraint trong migration.
	maxScanLimit = 200
	// maxBackfillLimit — trần số bài cũ lấy về ở vòng quét đầu. Bằng
	// maxScanLimit vì backfill chỉ chọn trong đúng cửa sổ mà vòng quét đó lấy
	// được; xin nhiều hơn scan_limit thì phần dư không tồn tại.
	maxBackfillLimit = 200
)

// ScanDefaults là chỉ số quét tối ưu của hệ thống, dùng khi kênh không cấu hình
// riêng. Lấy từ .env (xem config.Config).
type ScanDefaults struct {
	Interval       time.Duration
	Limit          int
	MaxPostsPerRun int
}

// List quản lý cả 2 danh sách kênh. Breaking và Scheduled là 2 thực thể độc lập
// (specs 0.1) nhưng dùng chung validate regex / nhận diện nền tảng.
type List struct {
	q               *repository.Queries
	platforms       domain.PlatformRegistry
	enq             domain.Enqueuer
	audit           *Audit
	defaultLanguage string
	scanDefaults    ScanDefaults
	modes           ModeGate
	log             *slog.Logger
}

func NewList(
	q *repository.Queries,
	platforms domain.PlatformRegistry,
	enq domain.Enqueuer,
	audit *Audit,
	defaultLanguage string,
	scanDefaults ScanDefaults,
	modes ModeGate,
	log *slog.Logger,
) *List {
	return &List{
		q: q, platforms: platforms, enq: enq, audit: audit,
		defaultLanguage: defaultLanguage, scanDefaults: scanDefaults,
		modes: modes, log: log,
	}
}

// ---------------------------------------------------------------------------
// Danh sách Breaking (F2)
// ---------------------------------------------------------------------------

type BreakingInput struct {
	SourceURL string
	// RegexPatterns nhận cả từ khoá/hashtag thô — service tự chuẩn hoá từng
	// phần tử về regex (business rule #3). Nhiều pattern kết hợp OR.
	RegexPatterns []string
	CollectMode   domain.CollectMode
	PromptID      *uuid.UUID
	Language      string
	AutoProcess   *bool
	AutoPublish   *bool
	RandomAuthor  *bool
	Status        string
	ScanLimit     *int32
	ScanInterval  *time.Duration
	// BackfillLimit: số bài CŨ lấy về ở vòng quét đầu tiên. nil = 0 = chỉ lấy
	// bài đăng sau khi thêm kênh.
	BackfillLimit *int32
	// MaxPostsPerRun: trần Bài Post tạo ra trong 1 vòng quét. nil = theo mặc
	// định hệ thống (MAX_POSTS_PER_RUN_DEFAULT, mặc định 0 = không giới hạn).
	MaxPostsPerRun *int32
	// LLMAPISetID: bộ API key dùng cho mode C của kênh này. Quét tự động không
	// có ai bấm nút để chọn bộ, nên bộ phải nằm sẵn trên kênh.
	LLMAPISetID *uuid.UUID
	// Schedule: khung giờ / thứ được phép quét. Zero value = 24/7.
	Schedule domain.ChannelSchedule
	// CountryID: quốc gia của kênh. Mọi Bài Post và Voice của kênh mang giá trị
	// này; nil = suy từ ngôn ngữ như trước.
	CountryID *int64
}

func (l *List) CreateBreaking(ctx context.Context, actor uuid.UUID, in BreakingInput) (repository.ListBreaking, error) {
	platform, contentType, err := l.detect(in.SourceURL)
	if err != nil {
		return repository.ListBreaking{}, err
	}
	if err := l.modes.Check(ctx, in.CollectMode); err != nil {
		return repository.ListBreaking{}, err
	}
	if err := validateMode(in.CollectMode, in.PromptID); err != nil {
		return repository.ListBreaking{}, err
	}
	patterns, err := normalizePatterns(in.RegexPatterns)
	if err != nil {
		return repository.ListBreaking{}, err
	}
	interval, err := optionalBreakingInterval(in.ScanInterval)
	if err != nil {
		return repository.ListBreaking{}, err
	}
	if err := in.Schedule.Validate(); err != nil {
		return repository.ListBreaking{}, err
	}
	if err := validateBackfillLimit(in.BackfillLimit); err != nil {
		return repository.ListBreaking{}, err
	}
	list, err := l.q.CreateListBreaking(ctx, repository.CreateListBreakingParams{
		SourceUrl:       strings.TrimSpace(in.SourceURL),
		Platform:        platform,
		ContentType:     nilIfEmpty(contentType),
		CollectMode:     string(in.CollectMode),
		PromptID:        in.PromptID,
		RegexPatterns:   patterns,
		LanguageDefault: resolveLanguage(in.Language, "", l.defaultLanguage),
		// Breaking ưu tiên tốc độ -> auto_process mặc định bật (specs -1).
		AutoProcess: boolOr(in.AutoProcess, true),
		AutoPublish: boolOr(in.AutoPublish, false),
		// Bốc tài khoản đứng tên bài: không ghi giá trị này thì voice của kênh
		// không có author và hỏng ở bước đăng — không có ai ngồi chọn tay cho
		// chúng (xem createFromRemote).
		RandomAuthor:   boolOr(in.RandomAuthor, false),
		Status:         statusOr(in.Status),
		ScanLimit:      l.scanLimitOr(in.ScanLimit),
		ScanInterval:   interval,
		BackfillLimit:  backfillOr(in.BackfillLimit),
		MaxPostsPerRun: l.maxPostsOr(in.MaxPostsPerRun),
		CreatedBy:      actor,
		LlmApiSetID:    in.LLMAPISetID,
		Timezone:       timezoneOr(in.Schedule.Timezone),
		ActiveFromMin:  in.Schedule.FromMin,
		ActiveToMin:    in.Schedule.ToMin,
		ActiveWeekdays: in.Schedule.Weekdays,
		CountryID:      in.CountryID,
	})
	if err != nil {
		return repository.ListBreaking{}, fmt.Errorf("tạo list_breaking: %w", err)
	}

	l.audit.Record(ctx, actor, domain.AuditCreate, domain.ObjectListBreaking, list.ID, map[string]any{
		"source_url":     list.SourceUrl,
		"platform":       list.Platform,
		"collect_mode":   list.CollectMode,
		"regex_patterns": list.RegexPatterns,
		"scan_limit":     list.ScanLimit,
		"backfill_limit": list.BackfillLimit,
		"country_id":     list.CountryID,
	})
	return list, nil
}

func (l *List) GetBreaking(ctx context.Context, id uuid.UUID) (repository.ListBreaking, error) {
	list, err := l.q.GetListBreaking(ctx, id)
	if err != nil {
		return repository.ListBreaking{}, wrapNotFound(err, "list_breaking "+id.String())
	}
	return list, nil
}

// ChannelFilter là bộ lọc chung của 2 bảng danh sách kênh.
type ChannelFilter struct {
	Status    *string
	Search    *string
	Platform  *string
	CreatedBy *uuid.UUID
	Limit     int32
	Offset    int32
	// Sort: cột thời gian để sắp xếp — `created_at` (mặc định) hoặc
	// `last_scanned_at`. Dir: `asc` | `desc` (mặc định).
	Sort string
	Dir  string
}

// ListBreaking hỗ trợ filter/search bằng regex trên source_url (chức năng B2).
// Trả kèm tổng số bản ghi khớp bộ lọc để bảng phân trang được.
func (l *List) ListBreaking(ctx context.Context, f ChannelFilter) ([]repository.ListListBreakingsRow, int64, error) {
	limit, offset := clampPage(f.Limit, f.Offset)
	if f.Search != nil {
		if err := validator.Validate(*f.Search); err != nil {
			return nil, 0, err
		}
	}
	sort, dir := normalizeSort(f.Sort, f.Dir, "created_at", "last_scanned_at")
	items, err := l.q.ListListBreakings(ctx, repository.ListListBreakingsParams{
		Status:    f.Status,
		Search:    f.Search,
		Platform:  f.Platform,
		CreatedBy: f.CreatedBy,
		Sort:      sort,
		Dir:       dir,
		Lim:       limit,
		Off:       offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list list_breaking: %w", err)
	}
	total, err := l.q.CountListBreakings(ctx, repository.CountListBreakingsParams{
		Status:    f.Status,
		Search:    f.Search,
		Platform:  f.Platform,
		CreatedBy: f.CreatedBy,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count list_breaking: %w", err)
	}
	return items, total, nil
}

// ---------------------------------------------------------------------------
// Bài bị bỏ qua (B4)
// ---------------------------------------------------------------------------

// SkippedEntry là 1 dòng "bài đã xét rồi bỏ" của một kênh Breaking.
type SkippedEntry struct {
	PostIDExternal string    `json:"post_id_external"`
	PostURL        string    `json:"post_url"`
	TextExcerpt    string    `json:"text_excerpt"`
	Reason         string    `json:"reason"`
	CheckedAt      time.Time `json:"checked_at"`
}

// SkippedSummary là cả tab "Bài bị bỏ qua" trong modal chi tiết kênh.
type SkippedSummary struct {
	Items []SkippedEntry `json:"items"`
	Total int64          `json:"total"`
	// Last7Days: số bài bỏ qua trong 7 ngày gần nhất. Tổng ở trên bị chặn bởi
	// hạn lưu log nên nó KHÔNG phải tổng từ đầu — con số theo cửa sổ cố định là
	// thứ duy nhất so sánh giữa hai kênh được.
	Last7Days int64 `json:"last_7_days"`
	Limit     int32 `json:"limit"`
	Offset    int32 `json:"offset"`
}

// SkippedLogs liệt kê những bài mà vòng quét đã xét rồi BỎ, kèm đoạn text đã
// đem so với regex.
//
// Đây là câu trả lời duy nhất cho "regex của kênh này có quá chặt không". Bảng
// kênh chỉ nói được bao nhiêu Bài Post đã tạo ra; nó im lặng hoàn toàn về số
// bài trượt sát nút, mà đó mới là thứ cần nhìn khi một kênh đang chạy nhưng
// chẳng ra bài nào.
//
// Log chỉ giữ theo SKIPPED_LOG_RETENTION (mặc định 7 ngày) — đây là ảnh chụp
// hiện tại, không phải sổ sách.
func (l *List) SkippedLogs(ctx context.Context, id uuid.UUID, limit, offset int32) (SkippedSummary, error) {
	// Kênh không tồn tại phải ra 404, không phải một tab rỗng trông như "kênh
	// này chưa bỏ qua bài nào".
	if _, err := l.GetBreaking(ctx, id); err != nil {
		return SkippedSummary{}, err
	}

	limit, offset = clampPage(limit, offset)
	rows, err := l.q.ListSkippedLogs(ctx, repository.ListSkippedLogsParams{
		ListBreakingID: id, Lim: limit, Off: offset,
	})
	if err != nil {
		return SkippedSummary{}, fmt.Errorf("list skipped_log: %w", err)
	}
	total, err := l.q.CountSkippedLogs(ctx, id)
	if err != nil {
		return SkippedSummary{}, fmt.Errorf("count skipped_log: %w", err)
	}
	last7, err := l.q.CountSkippedLogsSince(ctx, repository.CountSkippedLogsSinceParams{
		ListBreakingID: id, Since: time.Now().Add(-7 * 24 * time.Hour),
	})
	if err != nil {
		return SkippedSummary{}, fmt.Errorf("count skipped_log 7 ngày: %w", err)
	}

	items := make([]SkippedEntry, 0, len(rows))
	for _, r := range rows {
		items = append(items, SkippedEntry{
			PostIDExternal: r.PostIDExternal,
			PostURL:        deref(r.PostUrl),
			TextExcerpt:    deref(r.TextExcerpt),
			Reason:         r.Reason,
			CheckedAt:      r.CheckedAt,
		})
	}
	return SkippedSummary{
		Items: items, Total: total, Last7Days: last7, Limit: limit, Offset: offset,
	}, nil
}

type BreakingUpdate struct {
	SourceURL     *string
	RegexPatterns []string
	CollectMode   *string
	PromptID      *uuid.UUID
	Language      *string
	AutoProcess   *bool
	AutoPublish   *bool
	RandomAuthor  *bool
	Status        *string
	ScanLimit     *int32
	ScanInterval  *time.Duration
	BackfillLimit *int32
	// MaxPostsPerRun / ClearMaxPosts: cùng cách xử lý với khung giờ — NULL vừa
	// nghĩa "không sửa" vừa nghĩa "bỏ trần", nên việc BỎ phải có cờ riêng.
	MaxPostsPerRun *int32
	ClearMaxPosts  bool
	LLMAPISetID    *uuid.UUID
	// Schedule khác nil = thay toàn bộ cấu hình lịch. ClearWindow xử lý riêng
	// việc XOÁ khung giờ: trong Schedule, nil vừa có nghĩa "không sửa" vừa có
	// nghĩa "bỏ khung giờ", và chỉ một trong hai diễn giải được.
	Schedule    *domain.ChannelSchedule
	ClearWindow bool
	// CountryID / SetCountry: quốc gia của kênh. Cần cờ riêng vì nil ở CountryID
	// vừa có nghĩa "không sửa" vừa có nghĩa "gỡ quốc gia".
	CountryID  *int64
	SetCountry bool
}

func (l *List) UpdateBreaking(ctx context.Context, actor, id uuid.UUID, in BreakingUpdate) (repository.ListBreaking, error) {
	before, err := l.GetBreaking(ctx, id)
	if err != nil {
		return repository.ListBreaking{}, err
	}

	params := repository.UpdateListBreakingParams{
		ID:              id,
		CollectMode:     in.CollectMode,
		PromptID:        in.PromptID,
		LanguageDefault: in.Language,
		AutoProcess:     in.AutoProcess,
		// Đăng đi theo việc tạo — xem CreateScheduled.
		AutoPublish:    in.AutoProcess,
		Status:         in.Status,
		ScanLimit:      in.ScanLimit,
		BackfillLimit:  in.BackfillLimit,
		MaxPostsPerRun: in.MaxPostsPerRun,
		ClearMaxPosts:  in.ClearMaxPosts,
		LlmApiSetID:    in.LLMAPISetID,
		ClearWindow:    in.ClearWindow,
		RandomAuthor:   in.RandomAuthor,
		CountryID:      in.CountryID,
		SetCountry:     in.SetCountry,
	}
	if in.Schedule != nil {
		if err := in.Schedule.Validate(); err != nil {
			return repository.ListBreaking{}, err
		}
		params.Timezone = nilIfEmpty(in.Schedule.Timezone)
		params.ActiveFromMin = in.Schedule.FromMin
		params.ActiveToMin = in.Schedule.ToMin
		params.ActiveWeekdays = in.Schedule.Weekdays
	}

	if in.SourceURL != nil {
		platform, contentType, err := l.detect(*in.SourceURL)
		if err != nil {
			return repository.ListBreaking{}, err
		}
		params.SourceUrl = in.SourceURL
		params.Platform = &platform
		params.ContentType = nilIfEmpty(contentType)
	}
	if in.RegexPatterns != nil {
		patterns, err := normalizePatterns(in.RegexPatterns)
		if err != nil {
			return repository.ListBreaking{}, err
		}
		params.RegexPatterns = patterns
	}
	if in.CollectMode != nil {
		mode := domain.CollectMode(*in.CollectMode)
		promptID := before.PromptID
		if in.PromptID != nil {
			promptID = in.PromptID
		}
		if err := validateMode(mode, promptID); err != nil {
			return repository.ListBreaking{}, err
		}
	}
	if in.ScanInterval != nil {
		interval, err := optionalBreakingInterval(in.ScanInterval)
		if err != nil {
			return repository.ListBreaking{}, err
		}
		params.ScanInterval = interval
	}
	if in.ScanLimit != nil {
		if err := validateScanLimit(*in.ScanLimit); err != nil {
			return repository.ListBreaking{}, err
		}
	}
	if err := validateBackfillLimit(in.BackfillLimit); err != nil {
		return repository.ListBreaking{}, err
	}
	after, err := l.q.UpdateListBreaking(ctx, params)
	if err != nil {
		return repository.ListBreaking{}, wrapNotFound(err, "list_breaking "+id.String())
	}

	l.cascadeBreakingLanguage(ctx, before, after)

	l.audit.Record(ctx, actor, domain.AuditUpdate, domain.ObjectListBreaking, id, Diff(
		breakingSnapshot(before), breakingSnapshot(after)))
	return after, nil
}

func (l *List) DeleteBreaking(ctx context.Context, actor, id uuid.UUID) error {
	rows, err := l.q.DeleteListBreaking(ctx, id)
	if err != nil {
		return fmt.Errorf("xoá list_breaking: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("%w: list_breaking %s", domain.ErrNotFound, id)
	}
	l.audit.Record(ctx, actor, domain.AuditDelete, domain.ObjectListBreaking, id, nil)
	return nil
}

// RunBreaking trigger 1 vòng quét thủ công (dùng để test cấu hình regex).
func (l *List) RunBreaking(ctx context.Context, actor, id uuid.UUID) error {
	if _, err := l.GetBreaking(ctx, id); err != nil {
		return err
	}
	if err := l.enq.EnqueueBreakingScan(ctx, id.String(), actor.String()); err != nil {
		return err
	}
	l.audit.Record(ctx, actor, domain.AuditRun, domain.ObjectListBreaking, id, nil)
	return nil
}

// ---------------------------------------------------------------------------
// Danh sách Định kỳ (F3)
// ---------------------------------------------------------------------------

type ScheduledInput struct {
	SourceURL      string
	CollectMode    domain.CollectMode
	PromptID       *uuid.UUID
	ScanFrequency  time.Duration
	Language       string
	AutoProcess    *bool
	AutoPublish    *bool
	Status         string
	ScanLimit      *int32
	MaxPostsPerRun *int32
	// BackfillLimit: số bài CŨ lấy về ở vòng quét đầu tiên. nil = 0 = chỉ lấy
	// bài đăng sau khi thêm kênh.
	BackfillLimit *int32
	LLMAPISetID   *uuid.UUID
	Schedule      domain.ChannelSchedule
	// RandomAuthor: bốc tài khoản đứng tên bài đăng cho từng voice của kênh,
	// lọc theo quốc gia của kênh (hoặc suy từ Language nếu kênh chưa chọn).
	RandomAuthor *bool
	// CountryID — xem BreakingInput.
	CountryID *int64
}

func (l *List) CreateScheduled(ctx context.Context, actor uuid.UUID, in ScheduledInput) (repository.ListScheduled, error) {
	platform, contentType, err := l.detect(in.SourceURL)
	if err != nil {
		return repository.ListScheduled{}, err
	}
	if err := l.modes.Check(ctx, in.CollectMode); err != nil {
		return repository.ListScheduled{}, err
	}
	if err := validateMode(in.CollectMode, in.PromptID); err != nil {
		return repository.ListScheduled{}, err
	}
	if err := in.Schedule.Validate(); err != nil {
		return repository.ListScheduled{}, err
	}
	// Giờ chạy cố định THAY THẾ "mỗi N phút", nên khi có nó thì tần suất không
	// còn phải vượt sàn: kênh chạy đúng 3 mốc trong ngày không đụng gì tới
	// rate-limit, mà bắt nhập kèm một tần suất hợp lệ chỉ là thủ tục vô nghĩa.
	freq, err := scheduledInterval(in.ScanFrequency, in.Schedule)
	if err != nil {
		return repository.ListScheduled{}, err
	}
	if err := validateBackfillLimit(in.BackfillLimit); err != nil {
		return repository.ListScheduled{}, err
	}
	list, err := l.q.CreateListScheduled(ctx, repository.CreateListScheduledParams{
		SourceUrl:       strings.TrimSpace(in.SourceURL),
		Platform:        platform,
		ContentType:     nilIfEmpty(contentType),
		CollectMode:     string(in.CollectMode),
		PromptID:        in.PromptID,
		ScanFrequency:   freq,
		LanguageDefault: resolveLanguage(in.Language, "", l.defaultLanguage),
		// F3 có thể gom bài để duyệt hàng loạt -> mặc định vẫn bật, tuỳ kênh tắt.
		AutoProcess: boolOr(in.AutoProcess, true),
		// Đăng đi theo việc tạo, không còn là lựa chọn riêng: "tự tạo voice" mà
		// không đăng thì bài nằm lại ở nháp và vẫn phải vào bấm tay từng cái —
		// tức là không tự động. Giao diện vì thế chỉ còn một ô.
		AutoPublish:    boolOr(in.AutoProcess, true),
		RandomAuthor:   boolOr(in.RandomAuthor, false),
		Status:         statusOr(in.Status),
		ScanLimit:      l.scanLimitOr(in.ScanLimit),
		MaxPostsPerRun: l.maxPostsOr(in.MaxPostsPerRun),
		BackfillLimit:  backfillOr(in.BackfillLimit),
		CreatedBy:      actor,
		LlmApiSetID:    in.LLMAPISetID,
		Timezone:       timezoneOr(in.Schedule.Timezone),
		ActiveFromMin:  in.Schedule.FromMin,
		ActiveToMin:    in.Schedule.ToMin,
		ActiveWeekdays: in.Schedule.Weekdays,
		FixedTimesMin:  in.Schedule.FixedTimes,
		CountryID:      in.CountryID,
	})
	if err != nil {
		return repository.ListScheduled{}, fmt.Errorf("tạo list_scheduled: %w", err)
	}

	// Quét NGAY, không chờ vòng đầu theo lịch: "Số bài cũ của kênh" là thứ
	// người dùng vừa điền và mong thấy kết quả — kênh đặt tần suất 6 tiếng mà
	// im lặng 6 tiếng sau khi thêm thì không ai phân biệt được với kênh hỏng.
	// Vẫn đi qua đúng handler nên khung giờ của kênh vẫn được tôn trọng.
	if list.Status == statusActive {
		// actor rỗng: đây là vòng quét hệ thống tự chạy ngay sau khi thêm kênh,
		// không phải một lần "Quét thử" ai đó bấm — lịch sử quét phải nói đúng
		// điều đó.
		if err := l.enq.EnqueueScheduledScan(ctx, list.ID.String(), ""); err != nil {
			l.log.WarnContext(ctx, "không đẩy được vòng quét đầu cho kênh vừa thêm",
				"error", err, "list_scheduled_id", list.ID)
		}
	}

	l.audit.Record(ctx, actor, domain.AuditCreate, domain.ObjectListScheduled, list.ID, map[string]any{
		"source_url":     list.SourceUrl,
		"platform":       list.Platform,
		"collect_mode":   list.CollectMode,
		"scan_frequency": in.ScanFrequency.String(),
		"scan_limit":     list.ScanLimit,
		"backfill_limit": list.BackfillLimit,
		"country_id":     list.CountryID,
	})
	return list, nil
}

func (l *List) GetScheduled(ctx context.Context, id uuid.UUID) (repository.ListScheduled, error) {
	list, err := l.q.GetListScheduled(ctx, id)
	if err != nil {
		return repository.ListScheduled{}, wrapNotFound(err, "list_scheduled "+id.String())
	}
	return list, nil
}

func (l *List) ListScheduled(ctx context.Context, f ChannelFilter) ([]repository.ListListScheduledsRow, int64, error) {
	limit, offset := clampPage(f.Limit, f.Offset)
	if f.Search != nil {
		if err := validator.Validate(*f.Search); err != nil {
			return nil, 0, err
		}
	}
	sort, dir := normalizeSort(f.Sort, f.Dir, "created_at", "last_scanned_at")
	items, err := l.q.ListListScheduleds(ctx, repository.ListListScheduledsParams{
		Status:    f.Status,
		Search:    f.Search,
		Platform:  f.Platform,
		CreatedBy: f.CreatedBy,
		Sort:      sort,
		Dir:       dir,
		Lim:       limit,
		Off:       offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list list_scheduled: %w", err)
	}
	total, err := l.q.CountListScheduleds(ctx, repository.CountListScheduledsParams{
		Status:    f.Status,
		Search:    f.Search,
		Platform:  f.Platform,
		CreatedBy: f.CreatedBy,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count list_scheduled: %w", err)
	}
	return items, total, nil
}

type ScheduledUpdate struct {
	SourceURL      *string
	CollectMode    *string
	PromptID       *uuid.UUID
	ScanFrequency  *time.Duration
	Language       *string
	AutoProcess    *bool
	AutoPublish    *bool
	Status         *string
	ScanLimit      *int32
	MaxPostsPerRun *int32
	// ClearMaxPosts: cùng cách xử lý với khung giờ — NULL ở max_posts_per_run
	// vừa nghĩa "không sửa" vừa nghĩa "bỏ trần", nên việc BỎ phải có cờ riêng.
	ClearMaxPosts bool
	BackfillLimit *int32
	LLMAPISetID   *uuid.UUID
	Schedule      *domain.ChannelSchedule
	ClearWindow   bool
	RandomAuthor  *bool
	// CountryID / SetCountry — xem BreakingUpdate.
	CountryID  *int64
	SetCountry bool
}

func (l *List) UpdateScheduled(ctx context.Context, actor, id uuid.UUID, in ScheduledUpdate) (repository.ListScheduled, error) {
	before, err := l.GetScheduled(ctx, id)
	if err != nil {
		return repository.ListScheduled{}, err
	}

	params := repository.UpdateListScheduledParams{
		ID:              id,
		CollectMode:     in.CollectMode,
		PromptID:        in.PromptID,
		LanguageDefault: in.Language,
		AutoProcess:     in.AutoProcess,
		// Đăng đi theo việc tạo — xem CreateScheduled.
		AutoPublish:    in.AutoProcess,
		Status:         in.Status,
		ScanLimit:      in.ScanLimit,
		MaxPostsPerRun: in.MaxPostsPerRun,
		ClearMaxPosts:  in.ClearMaxPosts,
		BackfillLimit:  in.BackfillLimit,
		LlmApiSetID:    in.LLMAPISetID,
		ClearWindow:    in.ClearWindow,
		RandomAuthor:   in.RandomAuthor,
		CountryID:      in.CountryID,
		SetCountry:     in.SetCountry,
		// pgtype.Interval zero value = NULL -> COALESCE giữ giá trị cũ.
	}
	if in.Schedule != nil {
		if err := in.Schedule.Validate(); err != nil {
			return repository.ListScheduled{}, err
		}
		params.Timezone = nilIfEmpty(in.Schedule.Timezone)
		params.ActiveFromMin = in.Schedule.FromMin
		params.ActiveToMin = in.Schedule.ToMin
		params.ActiveWeekdays = in.Schedule.Weekdays
		params.FixedTimesMin = in.Schedule.FixedTimes
	}

	if in.SourceURL != nil {
		platform, contentType, err := l.detect(*in.SourceURL)
		if err != nil {
			return repository.ListScheduled{}, err
		}
		params.SourceUrl = in.SourceURL
		params.Platform = &platform
		params.ContentType = nilIfEmpty(contentType)
	}
	if in.CollectMode != nil {
		mode := domain.CollectMode(*in.CollectMode)
		promptID := before.PromptID
		if in.PromptID != nil {
			promptID = in.PromptID
		}
		if err := validateMode(mode, promptID); err != nil {
			return repository.ListScheduled{}, err
		}
	}
	if in.ScanFrequency != nil {
		freq, err := toInterval(*in.ScanFrequency)
		if err != nil {
			return repository.ListScheduled{}, err
		}
		params.ScanFrequency = freq
	}
	if in.ScanLimit != nil {
		if err := validateScanLimit(*in.ScanLimit); err != nil {
			return repository.ListScheduled{}, err
		}
	}
	if err := validateBackfillLimit(in.BackfillLimit); err != nil {
		return repository.ListScheduled{}, err
	}
	after, err := l.q.UpdateListScheduled(ctx, params)
	if err != nil {
		return repository.ListScheduled{}, wrapNotFound(err, "list_scheduled "+id.String())
	}

	l.cascadeScheduledLanguage(ctx, before, after)
	l.kickIfReactivated(ctx, before.Status, after)

	l.audit.Record(ctx, actor, domain.AuditUpdate, domain.ObjectListScheduled, id, Diff(
		scheduledSnapshot(before), scheduledSnapshot(after)))
	return after, nil
}

// cascadeScheduledLanguage đẩy ngôn ngữ vừa chốt ở kênh xuống các Bài Post của
// kênh và các Voice CHƯA có file của chúng.
//
// Vì sao cần: ngôn ngữ được chốt MỘT LẦN lúc bài được tạo (resolveLanguage đọc
// language_default tại thời điểm đó). Sửa ở kênh mà không lan xuống thì mọi bài
// đã nằm trong hàng đợi vẫn đọc bằng tiếng cũ — đúng thứ người dùng vừa sửa để
// tránh, và họ không có cách nào sửa hàng loạt bằng tay.
//
// Ranh giới dừng ở Voice ĐÃ CÓ FILE: nhãn ngôn ngữ ở đó mô tả một file audio có
// thật, đổi nhãn không đọc lại được file. Muốn đổi tiếng của voice đã ra file
// thì phải tạo lại nó.
//
// 'auto' không lan: nó nghĩa là "chưa chốt, để hệ thống tự nhận diện" chứ không
// phải một ngôn ngữ — ghi đè lựa chọn cụ thể của từng bài bằng nó là mất thông
// tin.
func (l *List) cascadeScheduledLanguage(ctx context.Context, before, after repository.ListScheduled) {
	lang := after.LanguageDefault
	if lang == before.LanguageDefault || domain.IsAutoLanguage(lang) {
		return
	}
	posts, err := l.q.CascadeLanguageFromListScheduled(ctx,
		repository.CascadeLanguageFromListScheduledParams{Language: lang, ListID: &after.ID})
	if err != nil {
		l.log.WarnContext(ctx, "không lan được ngôn ngữ xuống Bài Post của kênh",
			"error", err, "list_scheduled_id", after.ID)
		return
	}
	voices, err := l.q.CascadeVoiceLanguageFromListScheduled(ctx,
		repository.CascadeVoiceLanguageFromListScheduledParams{Language: lang, ListID: &after.ID})
	if err != nil {
		l.log.WarnContext(ctx, "không lan được ngôn ngữ xuống Voice của kênh",
			"error", err, "list_scheduled_id", after.ID)
		return
	}
	l.log.InfoContext(ctx, "lan ngôn ngữ từ kênh Định kỳ xuống bài và voice",
		"list_scheduled_id", after.ID, "language", lang, "bài", posts, "voice", voices)
}

// kickIfReactivated cho kênh vừa được BẬT LẠI quét ngay, không chờ hết chu kỳ.
//
// Kênh tắt thì scheduler bỏ qua mọi vòng của nó. Bật lại mà không làm gì thêm
// nghĩa là phải chờ trọn một chu kỳ nữa — với kênh đặt tần suất 6 tiếng thì đó
// là 6 tiếng im lặng ngay sau một thao tác mà người dùng hiểu là "cho chạy lại".
//
// Vẫn đi qua ĐÚNG hàng đợi và đúng handler như mọi vòng quét khác, nên khung
// giờ của kênh vẫn được tôn trọng: bật lại lúc 3h sáng trong khi kênh chỉ quét
// 6h–23h thì task này chạy rồi tự bỏ qua, và vòng theo lịch lúc 6h vẫn tới.
//
// Xoá last_scanned_at đi kèm: nó là mốc "kênh đã chạy tới đâu", để nguyên thì
// bảng hiển thị một lần quét cũ như thể vừa mới chạy.
// statusActive là giá trị cột `status` của kênh đang chạy. Xem statusOr().
const statusActive = "active"

func (l *List) kickIfReactivated(ctx context.Context, before string, after repository.ListScheduled) {
	if before == after.Status || after.Status != statusActive {
		return
	}
	if err := l.q.TouchListScheduledDue(ctx, after.ID); err != nil {
		l.log.WarnContext(ctx, "không xoá được mốc quét của kênh vừa bật lại",
			"error", err, "list_scheduled_id", after.ID)
	}
	// actor rỗng: bật lại kênh là một thao tác sửa cấu hình, vòng quét theo sau
	// vẫn là vòng của hệ thống — xem CreateScheduled.
	if err := l.enq.EnqueueScheduledScan(ctx, after.ID.String(), ""); err != nil {
		l.log.WarnContext(ctx, "không đẩy được vòng quét ngay cho kênh vừa bật lại",
			"error", err, "list_scheduled_id", after.ID)
		return
	}
	l.log.InfoContext(ctx, "kênh vừa bật lại — quét ngay", "list_scheduled_id", after.ID)
}

// cascadeBreakingLanguage — đối xứng với cascadeScheduledLanguage. Hai loại kênh
// dùng chung một form cấu hình, nên ngôn ngữ mà lan ở loại này và không lan ở
// loại kia là một cái bẫy chứ không phải một lựa chọn.
func (l *List) cascadeBreakingLanguage(ctx context.Context, before, after repository.ListBreaking) {
	lang := after.LanguageDefault
	if lang == before.LanguageDefault || domain.IsAutoLanguage(lang) {
		return
	}
	posts, err := l.q.CascadeLanguageFromListBreaking(ctx,
		repository.CascadeLanguageFromListBreakingParams{Language: lang, ListID: &after.ID})
	if err != nil {
		l.log.WarnContext(ctx, "không lan được ngôn ngữ xuống Bài Post của kênh",
			"error", err, "list_breaking_id", after.ID)
		return
	}
	voices, err := l.q.CascadeVoiceLanguageFromListBreaking(ctx,
		repository.CascadeVoiceLanguageFromListBreakingParams{Language: lang, ListID: &after.ID})
	if err != nil {
		l.log.WarnContext(ctx, "không lan được ngôn ngữ xuống Voice của kênh",
			"error", err, "list_breaking_id", after.ID)
		return
	}
	l.log.InfoContext(ctx, "lan ngôn ngữ từ kênh Breaking xuống bài và voice",
		"list_breaking_id", after.ID, "language", lang, "bài", posts, "voice", voices)
}

// RunScheduled trigger 1 vòng quét thủ công cho kênh Định kỳ.
//
// Đối xứng với RunBreaking, và cần vì cùng một lý do: kênh Định kỳ có thể đặt
// tần suất 6 tiếng, nên sau khi sửa cấu hình thì không có cách nào thử xem nó
// chạy đúng chưa ngoài việc ngồi chờ. Vẫn đi qua đúng handler như vòng theo
// lịch, nên khung giờ của kênh vẫn được tôn trọng.
func (l *List) RunScheduled(ctx context.Context, actor, id uuid.UUID) error {
	if _, err := l.GetScheduled(ctx, id); err != nil {
		return err
	}
	if err := l.enq.EnqueueScheduledScan(ctx, id.String(), actor.String()); err != nil {
		return err
	}
	l.audit.Record(ctx, actor, domain.AuditRun, domain.ObjectListScheduled, id, nil)
	return nil
}

func (l *List) DeleteScheduled(ctx context.Context, actor, id uuid.UUID) error {
	rows, err := l.q.DeleteListScheduled(ctx, id)
	if err != nil {
		return fmt.Errorf("xoá list_scheduled: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("%w: list_scheduled %s", domain.ErrNotFound, id)
	}
	l.audit.Record(ctx, actor, domain.AuditDelete, domain.ObjectListScheduled, id, nil)
	return nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// detect nhận diện nền tảng từ URL; không nhận ra thì báo lỗi, không đoán mò.
func (l *List) detect(rawURL string) (platform, contentType string, err error) {
	rawURL = domain.NormalizeSourceURL(rawURL)
	if rawURL == "" {
		return "", "", fmt.Errorf("%w: source_url là bắt buộc", domain.ErrInvalidInput)
	}
	adapter, err := l.platforms.Resolve(rawURL)
	if err != nil {
		return "", "", err
	}
	// Từ chối NGAY nền tảng không liệt kê được bài của kênh, thay vì nhận kênh
	// rồi để nó hỏng lặng lẽ mỗi vòng quét. Câu trả lời là tĩnh (giới hạn của
	// yt-dlp) nên không cần gọi mạng để biết.
	if err := adapter.CheckChannelScan(); err != nil {
		return "", "", err
	}
	// URL kênh (không phải 1 bài cụ thể) sẽ không parse ra content type — bỏ qua.
	ct, _, _ := adapter.ExtractID(rawURL)
	return string(adapter.Name()), ct, nil
}

func (l *List) scanLimitOr(v *int32) int32 {
	if v != nil && *v > 0 {
		return *v
	}
	return int32(l.scanDefaults.Limit)
}

func (l *List) maxPostsOr(v *int32) *int32 {
	if v != nil {
		if *v <= 0 {
			return nil // 0/âm = không giới hạn
		}
		return v
	}
	if l.scanDefaults.MaxPostsPerRun <= 0 {
		return nil
	}
	return ptr(int32(l.scanDefaults.MaxPostsPerRun))
}

// normalizePatterns chuẩn hoá từng pattern về regex và loại trùng lặp.
func normalizePatterns(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("%w: cần ít nhất 1 regex pattern", domain.ErrInvalidInput)
	}
	if len(raw) > maxRegexPatterns {
		return nil, fmt.Errorf("%w: tối đa %d pattern cho 1 kênh",
			domain.ErrInvalidInput, maxRegexPatterns)
	}

	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		if strings.TrimSpace(r) == "" {
			continue
		}
		pattern, err := validator.NormalizePattern(r)
		if err != nil {
			return nil, err
		}
		if _, dup := seen[pattern]; dup {
			continue
		}
		seen[pattern] = struct{}{}
		out = append(out, pattern)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: cần ít nhất 1 regex pattern", domain.ErrInvalidInput)
	}
	return out, nil
}

func validateMode(mode domain.CollectMode, promptID *uuid.UUID) error {
	if !mode.Valid() {
		return fmt.Errorf("%w: collect_mode phải là A, B hoặc C", domain.ErrInvalidInput)
	}
	if mode.NeedsPrompt() && promptID == nil {
		return domain.ErrPromptRequired
	}
	return nil
}

// backfillOr: nil = 0 = KHÔNG lấy bài cũ.
//
// Mặc định là 0 chứ không phải scan_limit: thêm một kênh mà lập tức sinh ra 20
// voice từ bài đăng cũ là thứ không ai chủ động chọn, chỉ là hệ quả của việc
// vòng quét đầu không có mốc đồng bộ nào để so.
func backfillOr(v *int32) int32 {
	if v == nil || *v < 0 {
		return 0
	}
	return *v
}

func validateBackfillLimit(v *int32) error {
	if v == nil {
		return nil
	}
	if *v < 0 || *v > maxBackfillLimit {
		return fmt.Errorf("%w: số bài cũ lấy về phải trong khoảng 0..%d",
			domain.ErrInvalidInput, maxBackfillLimit)
	}
	return nil
}

func validateScanLimit(v int32) error {
	if v < 1 || v > maxScanLimit {
		return fmt.Errorf("%w: scan_limit phải trong khoảng 1..%d", domain.ErrInvalidInput, maxScanLimit)
	}
	return nil
}

// timezoneOr điền múi giờ mặc định khi form không gửi gì — cột là NOT NULL, và
// chuỗi rỗng ở đó sẽ làm time.LoadLocation rơi về UTC, đúng cái sai cần tránh.
func timezoneOr(tz string) string {
	if tz = strings.TrimSpace(tz); tz != "" {
		return tz
	}
	return domain.DefaultTimezone
}

// scheduledInterval: có giờ chạy cố định thì scan_frequency không còn được
// dùng, nên chỉ cần một giá trị hợp lệ để thoả cột NOT NULL.
func scheduledInterval(d time.Duration, sched domain.ChannelSchedule) (pgtype.Interval, error) {
	if len(sched.FixedTimes) > 0 && d < minScanFrequency {
		d = minScanFrequency
	}
	return toInterval(d)
}

func toInterval(d time.Duration) (pgtype.Interval, error) {
	if d < minScanFrequency {
		return pgtype.Interval{}, fmt.Errorf("%w: scan_frequency tối thiểu %s",
			domain.ErrInvalidInput, minScanFrequency)
	}
	return pgtype.Interval{Microseconds: d.Microseconds(), Valid: true}, nil
}

// optionalBreakingInterval: nil = kênh dùng khoảng nghỉ mặc định của hệ thống.
func optionalBreakingInterval(d *time.Duration) (pgtype.Interval, error) {
	if d == nil {
		return pgtype.Interval{}, nil
	}
	if *d < minBreakingScanInterval {
		return pgtype.Interval{}, fmt.Errorf("%w: scan_interval tối thiểu %s",
			domain.ErrInvalidInput, minBreakingScanInterval)
	}
	return pgtype.Interval{Microseconds: d.Microseconds(), Valid: true}, nil
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

func boolOr(p *bool, fallback bool) bool {
	if p == nil {
		return fallback
	}
	return *p
}

func statusOr(s string) string {
	if strings.TrimSpace(s) == "" {
		return statusActive
	}
	return s
}

func breakingSnapshot(l repository.ListBreaking) map[string]any {
	return map[string]any{
		"source_url": l.SourceUrl, "platform": l.Platform, "collect_mode": l.CollectMode,
		"prompt_id": l.PromptID, "regex_patterns": l.RegexPatterns,
		"language_default": l.LanguageDefault, "auto_process": l.AutoProcess,
		"auto_publish": l.AutoPublish, "status": l.Status,
		"scan_limit": l.ScanLimit, "scan_interval": intervalDuration(l.ScanInterval).String(),
		"backfill_limit": l.BackfillLimit, "max_posts_per_run": l.MaxPostsPerRun,
	}
}

func scheduledSnapshot(l repository.ListScheduled) map[string]any {
	return map[string]any{
		"source_url": l.SourceUrl, "platform": l.Platform, "collect_mode": l.CollectMode,
		"prompt_id": l.PromptID, "scan_frequency": intervalDuration(l.ScanFrequency).String(),
		"language_default": l.LanguageDefault, "auto_process": l.AutoProcess,
		"auto_publish": l.AutoPublish, "status": l.Status,
		"scan_limit": l.ScanLimit, "max_posts_per_run": l.MaxPostsPerRun,
		"backfill_limit": l.BackfillLimit,
	}
}
