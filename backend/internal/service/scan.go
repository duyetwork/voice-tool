package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"strings"

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
	gate *KeyGate
	// scrapeGate giữ nhịp RIÊNG cho Facebook/X/Instagram.
	//
	// Tách khỏi `gate` vì hai loại request khác hẳn nhau về mức độ bị soi: một
	// lần yt-dlp lấy video YouTube là request ẩn danh, còn một lần tải trang
	// Facebook mang theo cookies của một tài khoản thật. Nhịp của cái sau phải
	// chậm hơn, và quan trọng hơn là nó không được ăn theo cấu hình của cái
	// trước — chỉnh nhanh YouTube lên thì không được kéo Facebook theo.
	//
	// nil = chưa cấu hình: rơi về `gate`, tức là vẫn có nhịp chứ không thả nổi.
	scrapeGate *KeyGate
	stats      *FetchStats
}

// skippedExcerptLen — độ dài đoạn text lưu kèm mỗi bài bị bỏ qua.
//
// 300 ký tự: đủ để nhìn ra bài nói về cái gì, và đủ ngắn để một vòng quét bỏ
// qua 200 bài không biến bảng log thành nơi lưu nội dung của cả kênh.
const skippedExcerptLen = 300

type ScanDeps struct {
	Queries    *repository.Queries
	Platforms  domain.PlatformRegistry
	Enqueuer   domain.Enqueuer
	Logger     *slog.Logger
	Gate       *KeyGate
	ScrapeGate *KeyGate
	FetchStats *FetchStats
}

func NewScan(d ScanDeps) *Scan {
	return &Scan{
		q: d.Queries, platforms: d.Platforms, enq: d.Enqueuer, log: d.Logger,
		gate: d.Gate, scrapeGate: d.ScrapeGate, stats: d.FetchStats,
	}
}

// firstRunLimit chốt cửa sổ của VÒNG QUÉT ĐẦU TIÊN của một kênh.
//
// Vòng đầu và các vòng sau trả lời hai câu hỏi khác nhau, nên chúng không dùng
// chung một con số:
//
//   - vòng đầu  — "lấy về bao nhiêu bài CŨ của kênh này": backfill_limit;
//   - vòng sau  — "mỗi lần nhìn bao nhiêu bài mới nhất để dò bài mới":
//     scan_limit.
//
// Trước đây cả hai đều dùng scan_limit rồi mới cắt bớt, nên xin 200 bài cũ mà
// cửa sổ quét là 20 thì lặng lẽ chỉ được 20 — con số người dùng điền không có
// tác dụng và không có gì nói ra điều đó.
//
// Sàn 1 bài: ngay cả khi không lấy bài cũ nào (0), vòng đầu vẫn phải nhìn thấy
// bài mới nhất để ĐẶT MỐC đồng bộ. Không có mốc thì vòng thứ hai coi cả cửa sổ
// là bài mới và nuốt trọn đúng những bài vừa cố tình bỏ qua.
func firstRunLimit(backfill int32) int {
	if backfill < 1 {
		return 1
	}
	return int(backfill)
}

// noteBackfillShortfall ghi lại khi vòng quét ĐẦU không lấy đủ số bài cũ.
//
// Không phải lúc nào cũng là lỗi: ba nền tảng chạy bằng via đều có TRẦN CỨNG ở
// một lần gọi và không có đường phân trang — đo trực tiếp ngày 17/09/2026:
//
//	Facebook   chỉ những bài Facebook dựng sẵn trong HTML trang (1–10)
//	Instagram  12 bài mỗi lần gọi web_profile_info
//	X          ~100 tweet mỗi lần gọi syndication
//
// Xin nhiều hơn chừng đó thì phần thiếu KHÔNG có cách nào lấy được, và trước
// dòng log này thì triệu chứng duy nhất là một con số nhỏ hơn mong đợi trong
// bảng lịch sử quét, không kèm lời giải thích nào. YouTube/TikTok không dính
// vì yt-dlp phân trang được (--playlist-end).
func (s *Scan) noteBackfillShortfall(ctx context.Context, platform, url string, want, got int) {
	if got >= want {
		return
	}
	s.log.InfoContext(ctx, "vòng quét đầu lấy được ít hơn số bài cũ đã xin",
		"platform", platform, "channel", url, "xin", want, "được", got,
		"lý_do", "nền tảng chỉ trả chừng đó trong một lần gọi và không phân trang")
}

// skipRoundReason nhận ra những lỗi mà cách xử lý đúng là BỎ QUA VÒNG NÀY, chứ
// không phải ghi kênh lỗi. Trả chuỗi rỗng = lỗi thật, xử lý như thường.
//
// Hai lỗi đó không nói gì về kênh đang quét, và cả hai đều tự khỏi theo thời
// gian — nên biến chúng thành lỗi của kênh là sai ở cả ba mặt: bảng kênh hiện
// đỏ cho một kênh không có vấn đề gì, asynq retry ba lần, và mẻ quét của các
// kênh còn lại bị kéo theo.
//
//   - ErrNoViaAvailable: hạn mức ngày đã cạn hoặc cả đàn via đang nghỉ.
//   - FetchBlockRateLimit: nền tảng bảo "chờ vài phút". Retry ngay chính là
//     thứ nó vừa yêu cầu đừng làm — ba lần thử dồn trong ~36 giây (đã thấy
//     thật với Instagram) chỉ kéo dài thêm khoảng phạt.
//
// Lỗi vẫn được đếm vào fetch_error_stat ở latestPosts trước khi tới đây, nên
// tab "Bị chặn" vẫn thấy — bỏ qua ở đây là bỏ qua việc RETRY, không phải bỏ
// qua việc ghi nhận.
func skipRoundReason(err error) string {
	if errors.Is(err, domain.ErrNoViaAvailable) {
		return "hết via khả dụng"
	}
	if kind, ok := domain.FetchBlockKindOf(err); ok && kind == domain.FetchBlockRateLimit {
		return "nền tảng đang giới hạn tần suất, chờ lượt sau"
	}
	return ""
}

// latestPosts gọi adapter qua PlatformGate: mỗi nền tảng 1 yt-dlp tại một thời
// điểm, có khoảng nghỉ giữa hai lần — và mọi lỗi bị chặn đều được đếm.
func (s *Scan) latestPosts(
	ctx context.Context,
	adapter domain.PlatformAdapter,
	platform, channelURL string,
	limit int,
) ([]domain.RemotePost, error) {
	release, err := s.gateFor(platform).Acquire(ctx, platform)
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

// gateFor chọn nhịp cho nền tảng này: nền tảng chạy bằng via đi đường riêng.
func (s *Scan) gateFor(platform string) *KeyGate {
	if s.scrapeGate != nil && domain.NeedsVia(domain.Platform(platform)) {
		return s.scrapeGate
	}
	return s.gate
}

// ---------------------------------------------------------------------------
// breaking:scan — quét liên tục, chỉ lấy bài khớp regex (business rule #4)
// ---------------------------------------------------------------------------

type ScanResult struct {
	Fetched int
	Created int
	// Voices: số Voice vòng quét này đẩy vào hàng đợi. Đếm riêng khỏi Created vì
	// kênh tắt auto_process vẫn tạo Bài Post mà không tạo voice nào — gộp hai số
	// làm một thì bảng lịch sử không phân biệt được "kênh không bắt được bài" với
	// "kênh bắt được bài nhưng không ai bảo nó đọc".
	Voices  int
	Skipped int
	// RequestedLimit: số bài vòng này XIN nền tảng, sau khi đã ép về trần của
	// nền tảng đó. Đi vào lịch sử quét để cột "lấy được" có mẫu số — không có
	// nó thì "được 12" là một con số không nói lên điều gì.
	RequestedLimit int
}

// add cộng kết quả của 1 bài vào tổng của vòng quét.
func (r *ScanResult) add(out remoteOutcome) {
	if out.Created {
		r.Created++
	}
	if out.Voiced {
		r.Voices++
	}
}

// ScanBreaking bọc vòng quét thật để GHI LẠI kết quả — lên chính kênh (lỗi gần
// nhất) và vào bảng lịch sử (scan_run).
//
// Không gộp vào scanBreaking: hàm đó có nhiều đường thoát, và nhét việc ghi lỗi
// vào từng đường là kiểu code mà chỉ cần thêm một `return` nữa là hỏng lặng lẽ.
func (s *Scan) ScanBreaking(
	ctx context.Context, listID uuid.UUID, trig ScanTrigger,
) (ScanResult, error) {
	runID := s.startRun(ctx, breakingOwner(listID), trig)
	res, err := s.scanBreaking(ctx, listID)
	s.finishRun(ctx, runID, res, err)
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

	limit := int(list.ScanLimit)
	if list.BackfillDoneAt == nil {
		limit = firstRunLimit(list.BackfillLimit)
	}
	// Ép về trần của nền tảng NGAY TẠI ĐÂY, không chỉ ở form: kênh thêm từ
	// trước khi có trần vẫn đang giữ con số cũ, và xin 50 ở một nơi chỉ trả về
	// 12 thì số ghi vào lịch sử là một lời hứa không ai giữ được.
	limit = domain.ClampChannelLimit(domain.Platform(list.Platform), limit)
	res.RequestedLimit = limit

	posts, err := s.latestPosts(ctx, adapter, list.Platform, list.SourceUrl, limit)
	if err == nil && list.BackfillDoneAt == nil {
		s.noteBackfillShortfall(ctx, list.Platform, list.SourceUrl, limit, len(posts))
	}
	if err != nil {
		if why := skipRoundReason(err); why != "" {
			s.log.WarnContext(ctx, "bỏ qua vòng quét — "+why,
				"list_id", list.ID, "platform", list.Platform)
			// GHI LẠI LÀ LỖI, không phải thành công-với-0-bài.
			//
			// Trước đây vòng bị bỏ qua trả về nil, nên lịch sử quét hiện
			// "thành công, lấy được 0 bài" mà không có một chữ nào nói vì sao —
			// và đó đúng là thứ người vận hành báo lại: "Instagram đã quét xong
			// nhưng không lấy ra được bài nào". Một vòng không làm được việc của
			// nó là một vòng hỏng, dù lỗi không phải của kênh.
			//
			// Permanent: nói một lần rồi thôi. Không để asynq thử lại ba lần
			// cho một lỗi mà chính nền tảng vừa bảo hãy chờ — retry ngay là
			// cách làm khoảng phạt dài thêm.
			return res, domain.Permanent(err)
		}
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
			// Ghi kèm URL và ĐOẠN TEXT đã đem so: không có hai thứ đó thì log
			// chỉ là một danh sách ID, và không ai nhìn ID mà kết luận được
			// regex quá chặt hay bài thật sự không liên quan.
			if err := s.q.CreateSkippedLog(ctx, repository.CreateSkippedLogParams{
				ListBreakingID: list.ID,
				PostIDExternal: p.PostID,
				Reason:         fmt.Sprintf("không khớp %d regex_patterns", len(patterns)),
				PostUrl:        nullable(p.URL),
				TextExcerpt:    nullable(excerpt(p.Text, skippedExcerptLen)),
			}); err != nil {
				s.log.WarnContext(ctx, "ghi skipped_log thất bại", "error", err, "post_id", p.PostID)
			}
			continue
		}

		out, err := s.createFromRemote(ctx, remoteInput{
			SourceType:   domain.SourceBreaking,
			ListID:       list.ID,
			Post:         p,
			Platform:     list.Platform,
			CollectMode:  list.CollectMode,
			PromptID:     list.PromptID,
			Language:     list.LanguageDefault,
			CreatedBy:    list.CreatedBy,
			AutoProcess:  list.AutoProcess,
			LLMAPISetID:  list.LlmApiSetID,
			AutoPublish:  list.AutoPublish,
			RandomAuthor: list.RandomAuthor,
			CountryID:    list.CountryID,
		})
		if err != nil {
			s.log.ErrorContext(ctx, "breaking:scan tạo source_post thất bại",
				"error", err, "list_id", list.ID, "post_id", p.PostID)
			continue
		}
		res.add(out)
	}
	return res, nil
}

// ---------------------------------------------------------------------------
// scheduled:scan — lấy toàn bộ bài mới hơn last_synced_post_id
// ---------------------------------------------------------------------------

// ScanScheduled — đối xứng với ScanBreaking, xem lý do ở đó.
func (s *Scan) ScanScheduled(
	ctx context.Context, listID uuid.UUID, trig ScanTrigger,
) (ScanResult, error) {
	runID := s.startRun(ctx, scheduledOwner(listID), trig)
	res, err := s.scanScheduled(ctx, listID)
	s.finishRun(ctx, runID, res, err)

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

	// Vòng đầu hỏi "bao nhiêu bài cũ", vòng sau hỏi "nhìn bao nhiêu bài mới
	// nhất" — xem firstRunLimit.
	limit := int(list.ScanLimit)
	if list.BackfillDoneAt == nil {
		limit = firstRunLimit(list.BackfillLimit)
	}
	// Ép về trần của nền tảng NGAY TẠI ĐÂY, không chỉ ở form: kênh thêm từ
	// trước khi có trần vẫn đang giữ con số cũ, và xin 50 ở một nơi chỉ trả về
	// 12 thì số ghi vào lịch sử là một lời hứa không ai giữ được.
	limit = domain.ClampChannelLimit(domain.Platform(list.Platform), limit)
	res.RequestedLimit = limit

	posts, err := s.latestPosts(ctx, adapter, list.Platform, list.SourceUrl, limit)
	if err == nil && list.BackfillDoneAt == nil {
		s.noteBackfillShortfall(ctx, list.Platform, list.SourceUrl, limit, len(posts))
	}
	if err != nil {
		if why := skipRoundReason(err); why != "" {
			s.log.WarnContext(ctx, "bỏ qua vòng quét — "+why,
				"list_id", list.ID, "platform", list.Platform)
			// GHI LẠI LÀ LỖI, không phải thành công-với-0-bài.
			//
			// Trước đây vòng bị bỏ qua trả về nil, nên lịch sử quét hiện
			// "thành công, lấy được 0 bài" mà không có một chữ nào nói vì sao —
			// và đó đúng là thứ người vận hành báo lại: "Instagram đã quét xong
			// nhưng không lấy ra được bài nào". Một vòng không làm được việc của
			// nó là một vòng hỏng, dù lỗi không phải của kênh.
			//
			// Permanent: nói một lần rồi thôi. Không để asynq thử lại ba lần
			// cho một lỗi mà chính nền tảng vừa bảo hãy chờ — retry ngay là
			// cách làm khoảng phạt dài thêm.
			return res, domain.Permanent(err)
		}
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

		out, err := s.createFromRemote(ctx, remoteInput{
			SourceType:   domain.SourceScheduled,
			ListID:       list.ID,
			Post:         p,
			Platform:     list.Platform,
			CollectMode:  list.CollectMode,
			PromptID:     list.PromptID,
			Language:     list.LanguageDefault,
			CreatedBy:    list.CreatedBy,
			AutoProcess:  list.AutoProcess,
			LLMAPISetID:  list.LlmApiSetID,
			AutoPublish:  list.AutoPublish,
			RandomAuthor: list.RandomAuthor,
			CountryID:    list.CountryID,
		})

		switch {
		case err == nil:
			res.add(out)
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
	// AutoPublish: tạo xong audio thì đăng thẳng lên multime.
	AutoPublish bool
	// RandomAuthor: bốc tài khoản đứng tên bài đăng, lọc theo quốc gia suy ra
	// từ Language.
	RandomAuthor bool
	// CountryID: quốc gia của kênh. Mọi Bài Post và Voice của kênh mang giá
	// trị này; nil = kênh chưa chọn, và lúc đó quay về suy từ Language như cũ.
	CountryID *int64
}

// remoteOutcome là những gì 1 bài thô đã tạo ra trong hệ thống.
//
// Hai cờ chứ không một: bài vào được hệ thống mà không sinh voice là chuyện
// bình thường (kênh tắt auto_process), và lịch sử quét phải đếm riêng hai con
// số đó — xem ScanResult.
type remoteOutcome struct {
	Created bool
	Voiced  bool
}

// createFromRemote tạo Bài Post từ 1 bài thô. Created=false nghĩa là bài đã có
// trong hệ thống — không phải lỗi.
//
// Dedup theo (platform, post_id_extracted) trên TOÀN hệ thống, không theo URL
// và không theo từng danh sách: cùng 1 bài nằm trong cả Breaking lẫn Định kỳ
// thì vẫn chỉ vào hệ thống 1 lần, và cùng 1 bài có nhiều dạng URL vẫn là 1.
func (s *Scan) createFromRemote(ctx context.Context, in remoteInput) (remoteOutcome, error) {
	if in.Post.PostID != "" {
		_, err := s.q.FindSourcePostByPostID(ctx, repository.FindSourcePostByPostIDParams{
			Platform:        in.Platform,
			PostIDExtracted: &in.Post.PostID,
		})
		switch {
		case err == nil:
			return remoteOutcome{}, nil
		case !errors.Is(err, pgx.ErrNoRows):
			return remoteOutcome{}, fmt.Errorf("kiểm tra trùng bài post: %w", err)
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
		// Quốc gia chốt ngay lúc bài được tạo, không đọc ngược lên kênh lúc cần:
		// kênh có thể bị xoá (list_*_id ON DELETE SET NULL) trong khi bài vẫn còn,
		// và một lần sửa kênh về sau không được viết lại lịch sử.
		CountryID: in.CountryID,
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
			return remoteOutcome{}, nil
		}
		return remoteOutcome{}, fmt.Errorf("tạo source_post: %w", err)
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

	if !in.AutoProcess {
		return remoteOutcome{Created: true}, nil
	}

	seed := VoiceSeed{
		LLMAPISetID:      in.LLMAPISetID,
		PublishWhenReady: in.AutoPublish,
		// Quốc gia của kênh đi theo MỌI voice của nó, kể cả khi không bốc author
		// tự động: đó chính là thứ lọc danh bạ tài khoản ở bước đăng, và người
		// vào sửa tay một voice của kênh cũng phải thấy đúng nhóm tài khoản ấy.
		AuthorCountryID: in.CountryID,
	}
	// Không có ai ngồi chọn author cho voice sinh ra từ kênh, nên
	// author_gender của chúng luôn NULL — và ensureAuthor ở bước đăng trả
	// lỗi vĩnh viễn vì thiếu đúng thứ đó. Nghĩa là trước cờ này, "tự đăng"
	// của kênh chỉ tạo ra voice hỏng ở bước cuối.
	if in.RandomAuthor {
		seed.AuthorGender = ptr(string(domain.RandomGender()))
		// Suy từ ngôn ngữ chỉ còn là ĐƯỜNG LÙI cho kênh chưa chọn quốc gia:
		// nó đoán sai ở đúng những chỗ hay gặp nhất (tiếng Anh ra cả chục
		// nước, 'auto' thì không ra nước nào).
		if seed.AuthorCountryID == nil {
			seed.AuthorCountryID = s.countryForLanguage(ctx, in.Language)
		}
	}
	if _, err := enqueueVoiceProcess(ctx, s.q, s.enq, post, in.CreatedBy, seed); err != nil {
		return remoteOutcome{Created: true}, fmt.Errorf("enqueue voice:process: %w", err)
	}
	return remoteOutcome{Created: true, Voiced: true}, nil
}

// countryForLanguage chốt quốc gia để lọc danh bạ tài khoản, suy ra từ ngôn ngữ
// của kênh.
//
// nil ở mọi nhánh hỏng — kể cả khi query lỗi: bốc tài khoản KHÔNG lọc quốc gia
// vẫn ra một tài khoản dùng được, còn chặn việc tạo voice chỉ vì tra cứu danh
// mục hỏng thì mất cả bài. Ngôn ngữ 'auto' cũng rơi vào đây: chưa chốt tiếng
// thì không có cơ sở nào để chọn nước.
func (s *Scan) countryForLanguage(ctx context.Context, language string) *int64 {
	names := domain.CountriesForLanguage(language)
	if len(names) == 0 {
		return nil
	}
	keys := make([]string, 0, len(names))
	for _, n := range names {
		keys = append(keys, strings.ToLower(strings.TrimSpace(n)))
	}
	id, err := s.q.PickCountryForLanguage(ctx, keys)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			s.log.WarnContext(ctx, "không tra được quốc gia theo ngôn ngữ, bốc author không lọc nước",
				"error", err, "language", language)
		}
		return nil
	}
	return &id
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}
