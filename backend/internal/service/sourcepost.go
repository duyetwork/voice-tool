package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/repository"
)

// SourcePost là service tầng 2 — "Bài Post". Mọi Voice đều phải đi qua đây,
// không có đường tắt URL -> Voice (business rule #1).
type SourcePost struct {
	q               *repository.Queries
	platforms       domain.PlatformRegistry
	enq             domain.Enqueuer
	audit           *Audit
	log             *slog.Logger
	defaultLanguage string
	enabledModes    []domain.CollectMode
}

func NewSourcePost(
	q *repository.Queries,
	platforms domain.PlatformRegistry,
	enq domain.Enqueuer,
	audit *Audit,
	log *slog.Logger,
	defaultLanguage string,
	enabledModes []domain.CollectMode,
) *SourcePost {
	return &SourcePost{
		q: q, platforms: platforms, enq: enq, audit: audit, log: log,
		defaultLanguage: defaultLanguage, enabledModes: enabledModes,
	}
}

// DuplicatePostError: đã có Bài Post cho đúng ID bài đăng này.
//
// Không tự bỏ qua và cũng không tự tạo thêm — trả bài đã có để UI hỏi người
// dùng "bỏ qua hay vẫn tạo mới". Bài cũ KHÔNG bị sửa gì.
type DuplicatePostError struct {
	Existing repository.SourcePost
}

func (e *DuplicatePostError) Error() string {
	return fmt.Sprintf("%v: bài đăng này đã có trong hệ thống (tạo lúc %s)",
		domain.ErrDuplicate, e.Existing.CreatedAt.Format("02/01/2006 15:04"))
}

func (e *DuplicatePostError) Unwrap() error { return domain.ErrDuplicate }

// CreateInput là input tạo Bài Post thủ công (luồng F1).
type CreateInput struct {
	SourceURL   string
	CollectMode domain.CollectMode
	PromptID    *uuid.UUID
	Language    string // override post-level, rỗng thì lấy mặc định hệ thống
	AutoProcess bool   // F1 mặc định bật (specs -1)
	// Platform ép nền tảng khi URL không tự nhận diện được (link rút gọn,
	// domain lạ). Rỗng = tự nhận diện từ URL.
	Platform string
	// AllowDuplicate: người dùng đã xem thông báo trùng và chọn vẫn tạo mới.
	AllowDuplicate bool
}

// Create tạo Bài Post từ 1 URL: tự nhận diện nền tảng + parse ID bài đăng.
func (s *SourcePost) Create(ctx context.Context, actor uuid.UUID, in CreateInput) (repository.SourcePost, error) {
	// Bóc link bọc redirect + bỏ tham số tracking trước khi nhận diện nền tảng
	// (link share từ app mobile hay ở dạng này).
	in.SourceURL = domain.NormalizeSourceURL(in.SourceURL)
	if in.SourceURL == "" {
		return repository.SourcePost{}, fmt.Errorf("%w: source_url là bắt buộc", domain.ErrInvalidInput)
	}
	if !in.CollectMode.Valid() {
		return repository.SourcePost{}, fmt.Errorf("%w: collect_mode phải là A, B hoặc C", domain.ErrInvalidInput)
	}
	if err := ModeEnabled(in.CollectMode, s.enabledModes); err != nil {
		return repository.SourcePost{}, err
	}
	if in.CollectMode.NeedsPrompt() && in.PromptID == nil {
		return repository.SourcePost{}, domain.ErrPromptRequired
	}

	adapter, err := s.adapterFor(in.SourceURL, in.Platform)
	if err != nil {
		return repository.SourcePost{}, err
	}
	contentType, postID, err := adapter.ExtractID(in.SourceURL)
	if err != nil {
		return repository.SourcePost{}, err
	}

	// Chống trùng theo ID bài đăng, không theo URL: cùng 1 bài Facebook có cả
	// dạng /watch?v=<id> và /reel/<id>. Không tự bỏ qua cũng không tự tạo thêm
	// — trả bài đã có để người dùng chọn.
	if postID != "" && !in.AllowDuplicate {
		existing, err := s.q.FindSourcePostByPostID(ctx,
			repository.FindSourcePostByPostIDParams{
				Platform:        string(adapter.Name()),
				PostIDExtracted: &postID,
			})
		switch {
		case err == nil:
			return repository.SourcePost{}, &DuplicatePostError{Existing: existing}
		case !errors.Is(err, pgx.ErrNoRows):
			return repository.SourcePost{}, fmt.Errorf("kiểm tra trùng bài post: %w", err)
		}
	}

	post, err := s.q.CreateSourcePost(ctx, repository.CreateSourcePostParams{
		SourceType:      string(domain.SourceF1),
		SourceUrl:       in.SourceURL,
		Platform:        string(adapter.Name()),
		ContentType:     nilIfEmpty(contentType),
		PostIDExtracted: nilIfEmpty(postID),
		CollectMode:     string(in.CollectMode),
		PromptID:        in.PromptID,
		Language:        resolveLanguage(in.Language, "", s.defaultLanguage),
		Status:          domain.PostStatusNew,
		CreatedBy:       actor,
	})
	if err != nil {
		return repository.SourcePost{}, fmt.Errorf("tạo source_post: %w", err)
	}

	s.audit.Record(ctx, actor, domain.AuditCreate, domain.ObjectSourcePost, post.ID, map[string]any{
		"source_url":   post.SourceUrl,
		"platform":     post.Platform,
		"collect_mode": post.CollectMode,
		"language":     post.Language,
	})

	// Lấy metadata gốc ngay khi tạo bài, độc lập với việc tạo Voice: bảng Bài
	// Post có tiêu đề/mô tả/ảnh để duyệt trước khi tốn chi phí AI. Chạy trong
	// worker vì chỉ image worker có yt-dlp, và để API không phải chờ mạng.
	if err := s.enq.EnqueuePostMetadata(ctx, post.ID.String()); err != nil {
		// Không chặn: bài vẫn chạy Voice được, chỉ là chưa có phần điền sẵn.
		s.log.WarnContext(ctx, "không enqueue được post:metadata",
			"error", err, "source_post_id", post.ID)
	}

	if in.AutoProcess {
		if err := s.Run(ctx, actor, post.ID); err != nil {
			return post, err
		}
	}
	return post, nil
}

func (s *SourcePost) Get(ctx context.Context, id uuid.UUID) (repository.SourcePost, error) {
	post, err := s.q.GetSourcePost(ctx, id)
	if err != nil {
		return repository.SourcePost{}, wrapNotFound(err, "source_post "+id.String())
	}
	return post, nil
}

// ListFilter gom mọi bộ lọc của bảng Bài Post: nguồn, trạng thái, nền tảng,
// hình thức thu thập, ngôn ngữ, người tạo và khoảng ngày tạo (prompt.md mục 8).
type ListFilter struct {
	SourceType      *string
	Status          *string
	Platform        *string
	CollectMode     *string
	Language        *string
	CreatedBy       *uuid.UUID
	ListBreakingID  *uuid.UUID
	ListScheduledID *uuid.UUID
	CreatedFrom     *time.Time
	CreatedTo       *time.Time
	Limit           int32
	Offset          int32
	// Dir: chiều sắp xếp theo created_at — `asc` | `desc` (mặc định).
	Dir string
}

func (s *SourcePost) List(ctx context.Context, f ListFilter) ([]repository.ListSourcePostsRow, int64, error) {
	limit, offset := clampPage(f.Limit, f.Offset)
	_, dir := normalizeSort("", f.Dir)

	items, err := s.q.ListSourcePosts(ctx, repository.ListSourcePostsParams{
		SourceType:      f.SourceType,
		Status:          f.Status,
		Platform:        f.Platform,
		CollectMode:     f.CollectMode,
		Language:        f.Language,
		CreatedBy:       f.CreatedBy,
		ListBreakingID:  f.ListBreakingID,
		ListScheduledID: f.ListScheduledID,
		CreatedFrom:     f.CreatedFrom,
		CreatedTo:       f.CreatedTo,
		Dir:             dir,
		Lim:             limit,
		Off:             offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list source_post: %w", err)
	}

	total, err := s.q.CountSourcePosts(ctx, repository.CountSourcePostsParams{
		SourceType:      f.SourceType,
		Status:          f.Status,
		Platform:        f.Platform,
		CollectMode:     f.CollectMode,
		Language:        f.Language,
		CreatedBy:       f.CreatedBy,
		ListBreakingID:  f.ListBreakingID,
		ListScheduledID: f.ListScheduledID,
		CreatedFrom:     f.CreatedFrom,
		CreatedTo:       f.CreatedTo,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count source_post: %w", err)
	}
	return items, total, nil
}

// adapterFor chọn adapter theo nền tảng người dùng chỉ định, hoặc tự nhận diện
// từ URL khi để trống (specs 1.3).
func (s *SourcePost) adapterFor(rawURL, platform string) (domain.PlatformAdapter, error) {
	platform = strings.ToLower(strings.TrimSpace(platform))
	if platform == "" {
		return s.platforms.Resolve(rawURL)
	}
	return s.platforms.Get(domain.Platform(platform))
}

// UpdateInput cho phép sửa cấu hình Bài Post trước khi chạy (chức năng P1).
type UpdateInput struct {
	CollectMode *string
	PromptID    *uuid.UUID
	Language    *string
}

func (s *SourcePost) Update(ctx context.Context, actor, id uuid.UUID, in UpdateInput) (repository.SourcePost, error) {
	before, err := s.Get(ctx, id)
	if err != nil {
		return repository.SourcePost{}, err
	}
	if before.Status == domain.PostStatusProcessing {
		return repository.SourcePost{}, fmt.Errorf("%w: bài post đang xử lý, không sửa được", domain.ErrInvalidInput)
	}

	if in.CollectMode != nil {
		mode := domain.CollectMode(*in.CollectMode)
		if !mode.Valid() {
			return repository.SourcePost{}, fmt.Errorf("%w: collect_mode phải là A, B hoặc C", domain.ErrInvalidInput)
		}
		if err := ModeEnabled(mode, s.enabledModes); err != nil {
			return repository.SourcePost{}, err
		}
		promptID := before.PromptID
		if in.PromptID != nil {
			promptID = in.PromptID
		}
		if mode.NeedsPrompt() && promptID == nil {
			return repository.SourcePost{}, domain.ErrPromptRequired
		}
	}

	after, err := s.q.UpdateSourcePost(ctx, repository.UpdateSourcePostParams{
		ID:          id,
		CollectMode: in.CollectMode,
		PromptID:    in.PromptID,
		Language:    in.Language,
	})
	if err != nil {
		return repository.SourcePost{}, wrapNotFound(err, "source_post "+id.String())
	}

	s.audit.Record(ctx, actor, domain.AuditUpdate, domain.ObjectSourcePost, id, Diff(
		map[string]any{"collect_mode": before.CollectMode, "prompt_id": before.PromptID, "language": before.Language},
		map[string]any{"collect_mode": after.CollectMode, "prompt_id": after.PromptID, "language": after.Language},
	))
	return after, nil
}

func (s *SourcePost) Delete(ctx context.Context, actor, id uuid.UUID) error {
	rows, err := s.q.DeleteSourcePost(ctx, id)
	if err != nil {
		return fmt.Errorf("xoá source_post: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("%w: source_post %s", domain.ErrNotFound, id)
	}
	s.audit.Record(ctx, actor, domain.AuditDelete, domain.ObjectSourcePost, id, nil)
	return nil
}

// Run đẩy Bài Post vào queue voice:process. API không tự chạy Core Engine
// (business rule #10).
func (s *SourcePost) Run(ctx context.Context, actor, id uuid.UUID) error {
	post, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if post.Status == domain.PostStatusProcessing {
		return fmt.Errorf("%w: bài post đang được xử lý", domain.ErrInvalidInput)
	}

	if _, err := enqueueVoiceProcess(ctx, s.q, s.enq, post, actor); err != nil {
		return err
	}
	s.audit.Record(ctx, actor, domain.AuditRun, domain.ObjectSourcePost, id, nil)
	return nil
}
