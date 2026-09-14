package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
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
	// gate giữ nhịp gọi yt-dlp theo từng nền tảng; stats đếm số lần bị chặn.
	gate  *PlatformGate
	stats *FetchStats
}

type ScanDeps struct {
	Queries    *repository.Queries
	Platforms  domain.PlatformRegistry
	Enqueuer   domain.Enqueuer
	Logger     *slog.Logger
	Gate       *PlatformGate
	FetchStats *FetchStats
}

func NewScan(d ScanDeps) *Scan {
	return &Scan{
		q: d.Queries, platforms: d.Platforms, enq: d.Enqueuer, log: d.Logger,
		gate: d.Gate, stats: d.FetchStats,
	}
}

// latestPosts gọi adapter qua PlatformGate: mỗi nền tảng 1 yt-dlp tại một thời
// điểm, có khoảng nghỉ giữa hai lần — và mọi lỗi bị chặn đều được đếm.
func (s *Scan) latestPosts(
	ctx context.Context,
	adapter domain.PlatformAdapter,
	platform, channelURL string,
	limit int,
) ([]domain.RemotePost, error) {
	release, err := s.gate.Acquire(ctx, platform)
	if err != nil {
		return nil, err
	}
	defer release()

	posts, err := adapter.FetchLatestPosts(ctx, channelURL, limit)
	if err != nil {
		s.stats.Record(ctx, platform, err)
		return nil, err
	}
	return posts, nil
}

// ---------------------------------------------------------------------------
// breaking:scan — quét liên tục, chỉ lấy bài khớp regex (business rule #4)
// ---------------------------------------------------------------------------

type ScanResult struct {
	Fetched int
	Created int
	Skipped int
}

// ScanBreaking bọc vòng quét thật để GHI LẠI kết quả lên chính kênh.
//
// Không gộp vào scanBreaking: hàm đó có nhiều đường thoát, và nhét việc ghi lỗi
// vào từng đường là kiểu code mà chỉ cần thêm một `return` nữa là hỏng lặng lẽ.
func (s *Scan) ScanBreaking(ctx context.Context, listID uuid.UUID) (ScanResult, error) {
	res, err := s.scanBreaking(ctx, listID)
	s.noteBreakingError(ctx, listID, err)
	return res, err
}

// noteBreakingError ghi lỗi vòng quét lên kênh, hoặc xoá lỗi cũ khi vòng này
// chạy sạch. Bản thân nó không bao giờ làm hỏng vòng quét: không ghi được thì
// chỉ mất phần hiển thị.
func (s *Scan) noteBreakingError(ctx context.Context, listID uuid.UUID, cause error) {
	var msg *string
	if cause != nil {
		m := domain.UserMessage(cause)
		msg = &m
	}
	if err := s.q.SetListBreakingScanError(ctx, repository.SetListBreakingScanErrorParams{
		ID: listID, LastError: msg,
	}); err != nil {
		s.log.WarnContext(ctx, "không ghi được last_error của kênh Breaking",
			"error", err, "list_id", listID)
	}
}

func (s *Scan) scanBreaking(ctx context.Context, listID uuid.UUID) (ScanResult, error) {
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

	posts, err := s.latestPosts(ctx, adapter, list.Platform, list.SourceUrl, int(list.ScanLimit))
	if err != nil {
		return res, fmt.Errorf("quét kênh %s: %w", list.SourceUrl, err)
	}

	// Vòng quét ĐẦU TIÊN của kênh: giữ lại đúng backfill_limit bài cũ nhất-định
	// và ghi nhớ phần bị loại. Kênh Breaking không có mốc đồng bộ — mỗi vòng nó
	// xét lại cùng một cửa sổ — nên "bỏ qua" mà không nhớ thì vòng sau chính
	// những bài ấy lại hiện ra như vừa mới đăng.
	//
	// Điều kiện `len(posts) > 0`: một vòng quét không lấy được bài nào (kênh
	// mới tinh, hoặc nền tảng trả rỗng) không phải là bằng chứng rằng chẳng có
	// bài cũ nào để lấy. Đánh dấu xong ở đây là nuốt mất hạn mức bài cũ mà
	// người dùng vừa chọn, và không có đường nào lấy lại.
	switch {
	case list.BackfillDoneAt == nil && len(posts) > 0:
		kept, dropped := splitBackfill(posts, int(list.BackfillLimit))
		posts = kept
		excluded := postIDs(dropped)
		if err := s.q.MarkListBreakingBackfilled(ctx, repository.MarkListBreakingBackfilledParams{
			ID: list.ID, ExcludedIds: excluded,
		}); err != nil {
			// Không ghi được thì DỪNG: chạy tiếp nghĩa là vòng sau sẽ coi đây
			// vẫn là lần đầu và lấy lại toàn bộ bài cũ.
			return res, fmt.Errorf("đánh dấu backfill list_breaking: %w", err)
		}
		s.log.InfoContext(ctx, "vòng quét đầu của kênh Breaking",
			"list_id", list.ID, "backfill_limit", list.BackfillLimit,
			"lấy", len(posts), "bỏ_qua", len(excluded))

	case list.BackfillDoneAt != nil && len(list.BackfillExcludedIds) > 0:
		posts = excludeIDs(posts, list.BackfillExcludedIds)
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

	// max_posts_per_run đếm số bài TẠO RA, không phải số bài xét: kênh Breaking
	// có thể duyệt qua 200 bài mà chỉ 3 bài khớp regex, và trần này sinh ra để
	// chặn chi phí AI chứ không phải chặn công đọc.
	maxCreate := math.MaxInt
	if list.MaxPostsPerRun != nil {
		maxCreate = int(*list.MaxPostsPerRun)
	}

	for _, p := range posts {
		if res.Created >= maxCreate {
			s.log.InfoContext(ctx, "chạm trần max_posts_per_run, phần còn lại để vòng sau",
				"list_id", list.ID, "trần", maxCreate, "còn_lại", len(posts)-res.Created-res.Skipped)
			break
		}
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
			LLMAPISetID: list.LlmApiSetID,
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

// ScanScheduled — đối xứng với ScanBreaking, xem lý do ở đó.
func (s *Scan) ScanScheduled(ctx context.Context, listID uuid.UUID) (ScanResult, error) {
	res, err := s.scanScheduled(ctx, listID)
	var msg *string
	if err != nil {
		m := domain.UserMessage(err)
		msg = &m
	}
	if serr := s.q.SetListScheduledScanError(ctx, repository.SetListScheduledScanErrorParams{
		ID: listID, LastError: msg,
	}); serr != nil {
		s.log.WarnContext(ctx, "không ghi được last_error của kênh Định kỳ",
			"error", serr, "list_id", listID)
	}
	return res, err
}

func (s *Scan) scanScheduled(ctx context.Context, listID uuid.UUID) (ScanResult, error) {
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

	posts, err := s.latestPosts(ctx, adapter, list.Platform, list.SourceUrl, int(list.ScanLimit))
	if err != nil {
		return res, fmt.Errorf("quét kênh %s: %w", list.SourceUrl, err)
	}

	// Adapter trả về mới nhất trước; chỉ lấy phần mới hơn mốc đã sync.
	fresh := newerThan(posts, list.LastSyncedPostID)

	// Vòng quét ĐẦU TIÊN: chưa có mốc đồng bộ nên `fresh` đang là TOÀN BỘ cửa
	// sổ quét — tức là bài đã đăng từ trước khi kênh được thêm vào. Chỉ lấy
	// đúng số bài cũ người dùng đã chọn.
	//
	// Không cần nhớ phần bị loại như kênh Breaking: mốc last_synced_post_id
	// dưới đây được đẩy lên bài mới nhất, nên chúng nằm lại phía sau mốc vĩnh
	// viễn.
	//
	// `len(posts) > 0` vì lý do như ở ScanBreaking: vòng quét rỗng không phải
	// bằng chứng rằng không có bài cũ nào để lấy.
	if list.BackfillDoneAt == nil && len(posts) > 0 {
		kept, dropped := splitBackfill(fresh, int(list.BackfillLimit))
		if len(dropped) > 0 {
			// Không tạo Bài Post cho phần bỏ qua, nhưng vẫn phải đẩy mốc vượt
			// qua chúng — nếu không, vòng sau chúng lại là "bài mới".
			newest := posts[0].PostID
			if err := s.q.SetLastSyncedPostID(ctx, repository.SetLastSyncedPostIDParams{
				ID: list.ID, LastSyncedPostID: &newest,
			}); err != nil {
				return res, fmt.Errorf("đẩy mốc sync qua phần bài cũ bỏ qua: %w", err)
			}
			// Mốc vừa nhảy tới bài mới nhất nên phần GIỮ LẠI cũng đã nằm sau
			// mốc; xử lý chúng ngay trong vòng này, đúng như người dùng chọn.
		}
		if err := s.q.MarkListScheduledBackfilled(ctx, list.ID); err != nil {
			return res, fmt.Errorf("đánh dấu backfill list_scheduled: %w", err)
		}
		s.log.InfoContext(ctx, "vòng quét đầu của kênh Định kỳ",
			"list_id", list.ID, "backfill_limit", list.BackfillLimit,
			"lấy", len(kept), "bỏ_qua", len(dropped))
		fresh = kept
	}

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
			LLMAPISetID: list.LlmApiSetID,
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

// splitBackfill cắt danh sách bài của VÒNG QUÉT ĐẦU thành phần lấy và phần bỏ.
//
// `posts` theo thứ tự mới nhất trước, nên `limit` bài cũ được phép lấy chính là
// `limit` phần tử đầu: chúng là những bài gần thời điểm thêm kênh nhất, tức là
// phần "bài cũ" mà người dùng còn quan tâm.
func splitBackfill(posts []domain.RemotePost, limit int) (kept, excluded []domain.RemotePost) {
	if limit >= len(posts) {
		return posts, nil
	}
	if limit <= 0 {
		return nil, posts
	}
	return posts[:limit], posts[limit:]
}

// postIDs rút id của từng bài. Bài không có id thì bỏ: không nhớ được nó, và
// dedup cũng không dựa vào nó.
func postIDs(posts []domain.RemotePost) []string {
	out := make([]string, 0, len(posts))
	for _, p := range posts {
		if p.PostID != "" {
			out = append(out, p.PostID)
		}
	}
	return out
}

// excludeIDs bỏ khỏi danh sách những bài đã bị loại ở vòng quét đầu.
func excludeIDs(posts []domain.RemotePost, ids []string) []domain.RemotePost {
	if len(ids) == 0 {
		return posts
	}
	skip := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		skip[id] = struct{}{}
	}
	out := posts[:0:0]
	for _, p := range posts {
		if _, bad := skip[p.PostID]; bad {
			continue
		}
		out = append(out, p)
	}
	return out
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
	// LLMAPISetID: bộ API gán cho chính kênh này.
	//
	// Mode C tự động không có ai bấm nút để chọn bộ, nên bộ phải nằm sẵn trên
	// kênh — không gán thì luồng tự động không chạy được mode C.
	LLMAPISetID *uuid.UUID
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

	// Metadata đầy đủ: vòng quét chạy `yt-dlp --flat-playlist`, và chế độ đó chỉ
	// trả id + tiêu đề — không có ảnh bìa, tác giả, ngày đăng, hashtag. Nên bài
	// lấy từ kênh vào bảng với metadata nghèo hơn hẳn bài tạo tay từ URL, dù là
	// cùng một bài.
	//
	// Chữa bằng đúng task mà luồng tạo tay đang dùng (post:metadata) thay vì bỏ
	// --flat-playlist: bỏ cờ đó nghĩa là yt-dlp mở từng bài ngay trong vòng quét
	// — một kênh scan_limit=20 thành 20 request nặng mỗi vòng, kể cả với những
	// bài sẽ bị loại vì trùng hoặc không khớp regex. Ở đây chỉ những bài THẬT SỰ
	// vào hệ thống mới tốn một request, và nó chạy trong worker qua PlatformGate.
	if err := s.enq.EnqueuePostMetadata(ctx, post.ID.String()); err != nil {
		// Không chặn: bài vẫn chạy Voice được, chỉ là bảng thiếu phần điền sẵn.
		s.log.WarnContext(ctx, "không enqueue được post:metadata cho bài từ kênh",
			"error", err, "source_post_id", post.ID)
	}

	if in.AutoProcess {
		seed := VoiceSeed{LLMAPISetID: in.LLMAPISetID}
		if _, err := enqueueVoiceProcess(ctx, s.q, s.enq, post, in.CreatedBy, seed); err != nil {
			return true, fmt.Errorf("enqueue voice:process: %w", err)
		}
	}
	return true, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}
