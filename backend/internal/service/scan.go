package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/pkg/validator"
	"github.com/strongbody/voice-tool/backend/internal/repository"
)

// uniqueViolation là mã lỗi Postgres khi trùng unique index dedup.
const uniqueViolation = "23505"

// Scan chứa logic quét của cả 2 danh sách, chạy trong worker.
type Scan struct {
	q         *repository.Queries
	platforms domain.PlatformRegistry
	enq       domain.Enqueuer
	log       *slog.Logger
}

func NewScan(q *repository.Queries, platforms domain.PlatformRegistry, enq domain.Enqueuer, log *slog.Logger) *Scan {
	return &Scan{q: q, platforms: platforms, enq: enq, log: log}
}

// ---------------------------------------------------------------------------
// breaking:scan — quét liên tục, chỉ lấy bài khớp regex (business rule #4)
// ---------------------------------------------------------------------------

type ScanResult struct {
	Fetched int
	Created int
	Skipped int
}

func (s *Scan) ScanBreaking(ctx context.Context, listID uuid.UUID) (ScanResult, error) {
	var res ScanResult

	list, err := s.q.GetListBreaking(ctx, listID)
	if err != nil {
		return res, domain.Permanent(wrapNotFound(err, "list_breaking "+listID.String()))
	}
	if list.Status != "active" {
		return res, nil
	}

	adapter, err := s.platforms.Get(domain.Platform(list.Platform))
	if err != nil {
		return res, domain.Permanent(err)
	}
	// Nhiều pattern kết hợp OR: khớp 1 pattern là lấy bài.
	patterns, err := compileAll(list.RegexPatterns)
	if err != nil {
		return res, domain.Permanent(err)
	}

	posts, err := adapter.FetchLatestPosts(ctx, list.SourceUrl, int(list.ScanLimit))
	if err != nil {
		return res, fmt.Errorf("quét kênh %s: %w", list.SourceUrl, err)
	}
	res.Fetched = len(posts)

	// Đánh mốc đã quét bất kể có bắt được bài nào hay không, để vòng dispatch
	// kế tiếp biết kênh này chưa tới hạn.
	defer func() {
		if err := s.q.TouchListBreakingScanned(ctx, list.ID); err != nil {
			s.log.WarnContext(ctx, "không cập nhật được last_scanned_at",
				"error", err, "list_id", list.ID)
		}
	}()

	for _, p := range posts {
		// Regex là tiêu chí duy nhất quyết định lấy hay bỏ (business rule #3).
		if !matchAny(patterns, p.Text) {
			res.Skipped++
			if err := s.q.CreateSkippedLog(ctx, repository.CreateSkippedLogParams{
				ListBreakingID: list.ID,
				PostIDExternal: p.PostID,
				Reason:         fmt.Sprintf("không khớp %d regex_patterns", len(patterns)),
			}); err != nil {
				s.log.WarnContext(ctx, "ghi skipped_log thất bại", "error", err, "post_id", p.PostID)
			}
			continue
		}

		created, err := s.createFromRemote(ctx, remoteInput{
			SourceType:  domain.SourceBreaking,
			ListID:      list.ID,
			Post:        p,
			Platform:    list.Platform,
			CollectMode: list.CollectMode,
			PromptID:    list.PromptID,
			Language:    list.LanguageDefault,
			CreatedBy:   list.CreatedBy,
			AutoProcess: list.AutoProcess,
		})
		if err != nil {
			s.log.ErrorContext(ctx, "breaking:scan tạo source_post thất bại",
				"error", err, "list_id", list.ID, "post_id", p.PostID)
			continue
		}
		if created {
			res.Created++
		}
	}
	return res, nil
}

// ---------------------------------------------------------------------------
// scheduled:scan — lấy toàn bộ bài mới hơn last_synced_post_id
// ---------------------------------------------------------------------------

func (s *Scan) ScanScheduled(ctx context.Context, listID uuid.UUID) (ScanResult, error) {
	var res ScanResult

	list, err := s.q.GetListScheduled(ctx, listID)
	if err != nil {
		return res, domain.Permanent(wrapNotFound(err, "list_scheduled "+listID.String()))
	}
	if list.Status != "active" {
		return res, nil
	}

	adapter, err := s.platforms.Get(domain.Platform(list.Platform))
	if err != nil {
		return res, domain.Permanent(err)
	}

	posts, err := adapter.FetchLatestPosts(ctx, list.SourceUrl, int(list.ScanLimit))
	if err != nil {
		return res, fmt.Errorf("quét kênh %s: %w", list.SourceUrl, err)
	}

	// Adapter trả về mới nhất trước; chỉ lấy phần mới hơn mốc đã sync.
	fresh := newerThan(posts, list.LastSyncedPostID)
	res.Fetched = len(fresh)

	// max_posts_per_run: trần số bài xử lý 1 vòng, chặn nổ chi phí AI khi kênh
	// đăng ồ ạt. Phần dư để vòng sau xử lý tiếp (mốc sync vẫn tiến dần).
	maxPerRun := len(fresh)
	if list.MaxPostsPerRun != nil && int(*list.MaxPostsPerRun) < maxPerRun {
		maxPerRun = int(*list.MaxPostsPerRun)
		// Cắt phần CŨ nhất để xử lý trước — giữ đúng thứ tự thời gian.
		fresh = fresh[len(fresh)-maxPerRun:]
		res.Fetched = maxPerRun
	}

	// Xử lý từ cũ đến mới để mốc last_synced_post_id luôn tiến liên tục.
	var lastOK *string
	for i := len(fresh) - 1; i >= 0; i-- {
		p := fresh[i]

		created, err := s.createFromRemote(ctx, remoteInput{
			SourceType:  domain.SourceScheduled,
			ListID:      list.ID,
			Post:        p,
			Platform:    list.Platform,
			CollectMode: list.CollectMode,
			PromptID:    list.PromptID,
			Language:    list.LanguageDefault,
			CreatedBy:   list.CreatedBy,
			AutoProcess: list.AutoProcess,
		})

		switch {
		case err == nil:
			if created {
				res.Created++
			}
			lastOK = ptr(p.PostID)

		case domain.IsPermanent(err):
			// Lỗi vĩnh viễn (bài bị xoá...) -> VẪN tiến mốc để không kẹt mãi
			// ở bài này (business rule #6).
			res.Skipped++
			lastOK = ptr(p.PostID)
			s.log.WarnContext(ctx, "scheduled:scan bỏ qua bài lỗi vĩnh viễn",
				"error", err, "list_id", list.ID, "post_id", p.PostID)

		default:
			// Lỗi tạm thời -> KHÔNG tiến mốc, dừng vòng này để lần sau retry
			// đúng bài đó.
			s.log.ErrorContext(ctx, "scheduled:scan lỗi tạm thời, dừng vòng quét",
				"error", err, "list_id", list.ID, "post_id", p.PostID)
			if lastOK != nil {
				if serr := s.q.SetLastSyncedPostID(ctx, repository.SetLastSyncedPostIDParams{
					ID: list.ID, LastSyncedPostID: lastOK,
				}); serr != nil {
					return res, fmt.Errorf("cập nhật last_synced_post_id: %w", serr)
				}
			}
			return res, err
		}
	}

	if lastOK != nil {
		if err := s.q.SetLastSyncedPostID(ctx, repository.SetLastSyncedPostIDParams{
			ID: list.ID, LastSyncedPostID: lastOK,
		}); err != nil {
			return res, fmt.Errorf("cập nhật last_synced_post_id: %w", err)
		}
	} else if err := s.q.TouchListScheduledScanned(ctx, list.ID); err != nil {
		s.log.WarnContext(ctx, "không cập nhật được last_scanned_at",
			"error", err, "list_id", list.ID)
	}
	return res, nil
}

// compileAll compile toàn bộ pattern của 1 kênh.
func compileAll(patterns []string) ([]*regexp.Regexp, error) {
	if len(patterns) == 0 {
		return nil, fmt.Errorf("%w: kênh không có regex_patterns", domain.ErrRegexInvalid)
	}
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := validator.Compile(p)
		if err != nil {
			return nil, err
		}
		out = append(out, re)
	}
	return out, nil
}

// matchAny: nhiều pattern kết hợp OR (specs mục 5, câu 2).
func matchAny(patterns []*regexp.Regexp, text string) bool {
	for _, re := range patterns {
		if re.MatchString(text) {
			return true
		}
	}
	return false
}

// newerThan trả về các bài đứng trước mốc marker (tức mới hơn). Không tìm thấy
// marker (kênh đăng nhiều bài giữa 2 lần quét, hoặc lần quét đầu) -> lấy hết.
func newerThan(posts []domain.RemotePost, marker *string) []domain.RemotePost {
	if marker == nil || *marker == "" {
		return posts
	}
	for i, p := range posts {
		if p.PostID == *marker {
			return posts[:i]
		}
	}
	return posts
}

// ---------------------------------------------------------------------------

type remoteInput struct {
	SourceType  domain.SourceType
	ListID      uuid.UUID
	Post        domain.RemotePost
	Platform    string
	CollectMode string
	PromptID    *uuid.UUID
	Language    string
	CreatedBy   uuid.UUID
	AutoProcess bool
}

// createFromRemote tạo Bài Post từ 1 bài thô. created=false nghĩa là bài đã có
// trong hệ thống — không phải lỗi.
//
// Dedup theo (platform, post_id_extracted) trên TOÀN hệ thống, không theo URL
// và không theo từng danh sách: cùng 1 bài nằm trong cả Breaking lẫn Định kỳ
// thì vẫn chỉ vào hệ thống 1 lần, và cùng 1 bài có nhiều dạng URL vẫn là 1.
func (s *Scan) createFromRemote(ctx context.Context, in remoteInput) (bool, error) {
	if in.Post.PostID != "" {
		_, err := s.q.FindSourcePostByPostID(ctx, repository.FindSourcePostByPostIDParams{
			Platform:        in.Platform,
			PostIDExtracted: &in.Post.PostID,
		})
		switch {
		case err == nil:
			return false, nil
		case !errors.Is(err, pgx.ErrNoRows):
			return false, fmt.Errorf("kiểm tra trùng bài post: %w", err)
		}
	}

	params := repository.CreateSourcePostParams{
		SourceType:      string(in.SourceType),
		SourceUrl:       in.Post.URL,
		Platform:        in.Platform,
		ContentType:     nilIfEmpty(in.Post.ContentType),
		PostIDExtracted: nilIfEmpty(in.Post.PostID),
		ExtractedText:   nilIfEmpty(in.Post.Text),
		CollectMode:     in.CollectMode,
		PromptID:        in.PromptID,
		// List-level language; Post-level override do user sửa tay sau
		// (business rule #9).
		Language:  in.Language,
		Status:    domain.PostStatusNew,
		CreatedBy: in.CreatedBy,
		// Metadata gốc lấy được ngay từ vòng quét — bảng Bài Post hiển thị
		// được tiêu đề/ảnh bìa trước cả khi tạo Voice.
		Title:        nilIfEmpty(in.Post.Meta.Title),
		Hashtags:     in.Post.Meta.Hashtags,
		ThumbnailUrl: nilIfEmpty(in.Post.Meta.ThumbnailURL),
		AuthorName:   nilIfEmpty(in.Post.Meta.AuthorName),
		PostedAt:     in.Post.Meta.PostedAt,
	}
	switch in.SourceType {
	case domain.SourceBreaking:
		params.ListBreakingID = &in.ListID
	case domain.SourceScheduled:
		params.ListScheduledID = &in.ListID
	}

	post, err := s.q.CreateSourcePost(ctx, params)
	if err != nil {
		// Unique index vẫn là lưới an toàn: 2 worker quét song song có thể cùng
		// vượt qua bước kiểm tra ở trên rồi cùng insert.
		if isUniqueViolation(err) {
			return false, nil
		}
		return false, fmt.Errorf("tạo source_post: %w", err)
	}

	if in.AutoProcess {
		if _, err := enqueueVoiceProcess(ctx, s.q, s.enq, post, in.CreatedBy, VoiceSeed{}); err != nil {
			return true, fmt.Errorf("enqueue voice:process: %w", err)
		}
	}
	return true, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}
