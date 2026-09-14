package service

import (
	"context"
	"fmt"
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
}

func NewList(
	q *repository.Queries,
	platforms domain.PlatformRegistry,
	enq domain.Enqueuer,
	audit *Audit,
	defaultLanguage string,
	scanDefaults ScanDefaults,
	modes ModeGate,
) *List {
	return &List{
		q: q, platforms: platforms, enq: enq, audit: audit,
		defaultLanguage: defaultLanguage, scanDefaults: scanDefaults,
		modes: modes,
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
	Status        string
	ScanLimit     *int32
	ScanInterval  *time.Duration
	// LLMAPISetID: bộ API key dùng cho mode C của kênh này. Quét tự động không
	// có ai bấm nút để chọn bộ, nên bộ phải nằm sẵn trên kênh.
	LLMAPISetID *uuid.UUID
	// Schedule: khung giờ / thứ được phép quét. Zero value = 24/7.
	Schedule domain.ChannelSchedule
}

func (l *List) CreateBreaking(ctx context.Context, actor uuid.UUID, in BreakingInput) (repository.ListBreaking, error) {
	platform, contentType, err := l.detect(in.SourceURL)
	if err != nil {
		return repository.ListBreaking{}, err
	}
	if err := l.modes.Check(in.CollectMode); err != nil {
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

	list, err := l.q.CreateListBreaking(ctx, repository.CreateListBreakingParams{
		SourceUrl:       strings.TrimSpace(in.SourceURL),
		Platform:        platform,
		ContentType:     nilIfEmpty(contentType),
		CollectMode:     string(in.CollectMode),
		PromptID:        in.PromptID,
		RegexPatterns:   patterns,
		LanguageDefault: resolveLanguage(in.Language, "", l.defaultLanguage),
		// Breaking ưu tiên tốc độ -> auto_process mặc định bật (specs -1).
		AutoProcess:    boolOr(in.AutoProcess, true),
		AutoPublish:    boolOr(in.AutoPublish, false),
		Status:         statusOr(in.Status),
		ScanLimit:      l.scanLimitOr(in.ScanLimit),
		ScanInterval:   interval,
		CreatedBy:      actor,
		LlmApiSetID:    in.LLMAPISetID,
		Timezone:       timezoneOr(in.Schedule.Timezone),
		ActiveFromMin:  in.Schedule.FromMin,
		ActiveToMin:    in.Schedule.ToMin,
		ActiveWeekdays: in.Schedule.Weekdays,
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

type BreakingUpdate struct {
	SourceURL     *string
	RegexPatterns []string
	CollectMode   *string
	PromptID      *uuid.UUID
	Language      *string
	AutoProcess   *bool
	AutoPublish   *bool
	Status        *string
	ScanLimit     *int32
	ScanInterval  *time.Duration
	LLMAPISetID   *uuid.UUID
	// Schedule khác nil = thay toàn bộ cấu hình lịch. ClearWindow xử lý riêng
	// việc XOÁ khung giờ: trong Schedule, nil vừa có nghĩa "không sửa" vừa có
	// nghĩa "bỏ khung giờ", và chỉ một trong hai diễn giải được.
	Schedule    *domain.ChannelSchedule
	ClearWindow bool
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
		AutoPublish:     in.AutoPublish,
		Status:          in.Status,
		ScanLimit:       in.ScanLimit,
		LlmApiSetID:     in.LLMAPISetID,
		ClearWindow:     in.ClearWindow,
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

	after, err := l.q.UpdateListBreaking(ctx, params)
	if err != nil {
		return repository.ListBreaking{}, wrapNotFound(err, "list_breaking "+id.String())
	}

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
	if err := l.enq.EnqueueBreakingScan(ctx, id.String()); err != nil {
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
	LLMAPISetID    *uuid.UUID
	Schedule       domain.ChannelSchedule
}

func (l *List) CreateScheduled(ctx context.Context, actor uuid.UUID, in ScheduledInput) (repository.ListScheduled, error) {
	platform, contentType, err := l.detect(in.SourceURL)
	if err != nil {
		return repository.ListScheduled{}, err
	}
	if err := l.modes.Check(in.CollectMode); err != nil {
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

	list, err := l.q.CreateListScheduled(ctx, repository.CreateListScheduledParams{
		SourceUrl:       strings.TrimSpace(in.SourceURL),
		Platform:        platform,
		ContentType:     nilIfEmpty(contentType),
		CollectMode:     string(in.CollectMode),
		PromptID:        in.PromptID,
		ScanFrequency:   freq,
		LanguageDefault: resolveLanguage(in.Language, "", l.defaultLanguage),
		// F3 có thể gom bài để duyệt hàng loạt -> mặc định vẫn bật, tuỳ kênh tắt.
		AutoProcess:    boolOr(in.AutoProcess, true),
		AutoPublish:    boolOr(in.AutoPublish, false),
		Status:         statusOr(in.Status),
		ScanLimit:      l.scanLimitOr(in.ScanLimit),
		MaxPostsPerRun: l.maxPostsOr(in.MaxPostsPerRun),
		CreatedBy:      actor,
		LlmApiSetID:    in.LLMAPISetID,
		Timezone:       timezoneOr(in.Schedule.Timezone),
		ActiveFromMin:  in.Schedule.FromMin,
		ActiveToMin:    in.Schedule.ToMin,
		ActiveWeekdays: in.Schedule.Weekdays,
		FixedTimesMin:  in.Schedule.FixedTimes,
	})
	if err != nil {
		return repository.ListScheduled{}, fmt.Errorf("tạo list_scheduled: %w", err)
	}

	l.audit.Record(ctx, actor, domain.AuditCreate, domain.ObjectListScheduled, list.ID, map[string]any{
		"source_url":     list.SourceUrl,
		"platform":       list.Platform,
		"collect_mode":   list.CollectMode,
		"scan_frequency": in.ScanFrequency.String(),
		"scan_limit":     list.ScanLimit,
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
	LLMAPISetID    *uuid.UUID
	Schedule       *domain.ChannelSchedule
	ClearWindow    bool
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
		AutoPublish:     in.AutoPublish,
		Status:          in.Status,
		ScanLimit:       in.ScanLimit,
		MaxPostsPerRun:  in.MaxPostsPerRun,
		LlmApiSetID:     in.LLMAPISetID,
		ClearWindow:     in.ClearWindow,
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

	after, err := l.q.UpdateListScheduled(ctx, params)
	if err != nil {
		return repository.ListScheduled{}, wrapNotFound(err, "list_scheduled "+id.String())
	}

	l.audit.Record(ctx, actor, domain.AuditUpdate, domain.ObjectListScheduled, id, Diff(
		scheduledSnapshot(before), scheduledSnapshot(after)))
	return after, nil
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
		return "active"
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
	}
}

func scheduledSnapshot(l repository.ListScheduled) map[string]any {
	return map[string]any{
		"source_url": l.SourceUrl, "platform": l.Platform, "collect_mode": l.CollectMode,
		"prompt_id": l.PromptID, "scan_frequency": intervalDuration(l.ScanFrequency).String(),
		"language_default": l.LanguageDefault, "auto_process": l.AutoProcess,
		"auto_publish": l.AutoPublish, "status": l.Status,
		"scan_limit": l.ScanLimit, "max_posts_per_run": l.MaxPostsPerRun,
	}
}
