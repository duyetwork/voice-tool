package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/strongbody/voice-tool/backend/internal/config"
	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/pkg/secret"
	"github.com/strongbody/voice-tool/backend/internal/repository"
)

// ptr trả về con trỏ tới v — dùng cho các cột nullable của sqlc.
func ptr[T any](v T) *T { return &v }

// nilIfEmpty trả về nil cho string rỗng, để không ghi chuỗi rỗng vào cột nullable.
func nilIfEmpty(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}

// nilIfZero: 0 nghĩa là "không đo được" -> lưu NULL thay vì 0.
func nilIfZero[T int32 | int64 | int](v T) *T {
	if v == 0 {
		return nil
	}
	return &v
}

// firstNonEmpty lấy ứng viên đầu tiên có nội dung, giữ nguyên cả đoạn.
func firstNonEmpty(candidates ...string) string {
	for _, c := range candidates {
		if s := strings.TrimSpace(c); s != "" {
			return s
		}
	}
	return ""
}

// firstNonEmptyStr trả chuỗi đầu tiên có chữ.
func firstNonEmptyStr(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// mergeHashtags gộp hashtag người dùng gõ với hashtag lấy từ bài gốc.
//
// Gộp chứ không thay thế: hashtag của bài là thứ giúp bài tìm lại được trên
// multime, còn hashtag người dùng gõ là phần phân loại riêng của họ — bỏ bên
// nào cũng mất thông tin. So khớp không phân biệt hoa thường và dấu #, vì
// "#TinNong" với "tinnong" là cùng một thẻ.
func mergeHashtags(userInput string, postTags []string) string {
	out := make([]string, 0, len(postTags)+4)
	seen := map[string]bool{}

	add := func(tags []string) {
		for _, tag := range tags {
			key := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(tag), "#"))
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, "#"+key)
		}
	}

	add(config.SplitHashtags(userInput))
	add(postTags)
	return strings.Join(out, " ")
}

func deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

// wrapNotFound đổi pgx.ErrNoRows thành domain.ErrNotFound để handler map đúng 404.
func wrapNotFound(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %s", domain.ErrNotFound, what)
	}
	return err
}

// clampPage chuẩn hoá limit/offset của các endpoint list.
func clampPage(limit, offset int32) (int32, int32) {
	switch {
	case limit <= 0:
		limit = 20
	case limit > 200:
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// Chiều sắp xếp của các endpoint list.
const (
	SortAsc  = "asc"
	SortDesc = "desc"
)

// normalizeSort chuẩn hoá tham số sắp xếp trước khi đưa vào SQL.
//
// `sort` chỉ được nhận nếu nằm trong whitelist của chính bảng đó — SQL dùng
// giá trị này trong CASE WHEN, và whitelist ở đây là thứ đảm bảo không có cột
// lạ nào lọt vào. Chiều mặc định là desc (mới nhất trước) như cũ.
func normalizeSort(sort, dir string, allowed ...string) (string, string) {
	sort = strings.ToLower(strings.TrimSpace(sort))
	if !slices.Contains(allowed, sort) {
		sort = ""
	}
	if strings.ToLower(strings.TrimSpace(dir)) == SortAsc {
		return sort, SortAsc
	}
	return sort, SortDesc
}

// enqueueVoiceProcess tạo sẵn 1 record Voice ở trạng thái `processing` rồi mới
// đẩy job — nhờ vậy bảng Voice hiện ngay dòng "Đang xử lý" thay vì trống trơn
// cho tới khi worker chạy xong.
//
// Metadata gốc của Bài Post được điền sẵn để dòng đó có nội dung đọc được ngay;
// worker sẽ ghi đè bằng metadata fetch mới (xem query FinishVoice).
// Enqueue lỗi thì xoá record, không để lại dòng treo ở "đang xử lý" mãi.
// VoiceSeed là metadata người dùng điền sẵn ở màn tạo Voice, trước cả khi
// worker chạm vào bài gốc.
//
// Tồn tại vì màn đó giờ là MỘT bước: người dùng điền tiêu đề/hashtag/author rồi
// bấm Đăng, không ngồi chờ fetch xong mới điền. Giá trị ở đây luôn thắng giá
// trị lấy từ bài gốc — worker chỉ điền vào ô còn trống (xem Engine.buildVoice).
type VoiceSeed struct {
	Title    string
	Hashtag  string
	Language string
	ImageURL string
	// ImageUploaded: ảnh nằm trong storage của tool -> xoá sau khi đăng.
	ImageUploaded bool
	// NoImage: người dùng chủ động chọn "không có ảnh". Khác với để trống, vì
	// để trống thì worker điền ảnh bìa của bài gốc vào.
	NoImage      bool
	AuthorID     *int64
	AuthorEmail  *string
	AuthorGender *string
	// AuthorCountryID: quốc gia đã lọc lúc chọn author. Đi theo voice vì việc
	// BỐC tài khoản diễn ra ở bước đăng, lúc đó không còn form nào để hỏi lại.
	AuthorCountryID *int64
	// PublishWhenReady: tạo xong audio thì đăng luôn, không cần bấm nút nữa.
	PublishWhenReady bool
	// LLMAPISetID: Bộ API key dùng để viết lại nội dung (chỉ mode C).
	//
	// Đi theo VOICE chứ không đọc lại từ kênh lúc worker chạy: kênh có thể bị
	// sửa hoặc đổi bộ giữa lúc bài nằm trong hàng đợi, và lúc đó voice phải chạy
	// bằng đúng bộ đã chọn khi tạo — không thì hoá đơn rơi vào nhầm nhóm.
	LLMAPISetID *uuid.UUID
	// TTSConfig: cấu hình giọng đọc mở ra chỉnh ở mục "Cấu hình giọng đọc".
	//
	// nil = để mặc định. Đi theo voice vì worker chạy sau, lúc đó form đã đóng
	// — và vì cùng một API key vẫn phải đọc mỗi bài một giọng khác nhau được.
	TTSConfig *domain.VoiceStyle
}

// ttsConfigJSON đổi cấu hình giọng sang JSONB để lưu vào cột voice.tts_config.
//
// Trả về lỗi thay vì lặng lẽ bỏ qua giá trị sai: người dùng chọn một cao độ mà
// nhà cung cấp không có thì phải biết ngay lúc bấm nút, không phải nghe xong
// file mới thấy giọng chẳng giống thứ mình chọn.
func ttsConfigJSON(style *domain.VoiceStyle) ([]byte, error) {
	if style == nil {
		return nil, nil
	}
	return domain.MarshalVoiceStyle(*style)
}

func enqueueVoiceProcess(
	ctx context.Context,
	q *repository.Queries,
	enq domain.Enqueuer,
	post repository.SourcePost,
	actor uuid.UUID,
	seed VoiceSeed,
) (repository.Voice, error) {
	ttsConfig, err := ttsConfigJSON(seed.TTSConfig)
	if err != nil {
		return repository.Voice{}, err
	}

	// Ảnh: ưu tiên ảnh người dùng đưa; họ chọn "không có ảnh" thì để trống hẳn;
	// còn lại mới lấy ảnh bìa bài gốc.
	image := post.ThumbnailUrl
	switch {
	case seed.ImageURL != "":
		image = &seed.ImageURL
	case seed.NoImage:
		image = nil
	}

	voice, err := q.CreateVoice(ctx, repository.CreateVoiceParams{
		SourcePostID:  &post.ID,
		Language:      firstNonEmptyStr(seed.Language, post.Language),
		PublishStatus: domain.PublishProcessing,
		Title: nilIfEmpty(firstNonEmptyStr(
			domain.VoiceTitle(seed.Title), domain.VoiceTitle(deref(post.Title)))),
		Hashtag:          nilIfEmpty(mergeHashtags(seed.Hashtag, post.Hashtags)),
		ImageUrl:         image,
		CreatedBy:        actor,
		AuthorID:         seed.AuthorID,
		AuthorEmail:      seed.AuthorEmail,
		AuthorGender:     seed.AuthorGender,
		AuthorCountryID:  seed.AuthorCountryID,
		ImageUploaded:    seed.ImageURL != "" && seed.ImageUploaded,
		NoImage:          seed.NoImage,
		PublishWhenReady: seed.PublishWhenReady,
		LlmApiSetID:      seed.LLMAPISetID,
		TtsConfig:        ttsConfig,
	})
	if err != nil {
		return repository.Voice{}, fmt.Errorf("tạo voice processing: %w", err)
	}

	if err := enq.EnqueueVoiceProcess(ctx, post.ID.String(), actor.String(), voice.ID.String()); err != nil {
		if _, delErr := q.DeleteVoice(ctx, voice.ID); delErr != nil {
			// Không xoá được thì dòng đó treo ở "đang xử lý" — log để còn dọn tay.
			err = errors.Join(err, fmt.Errorf("dọn voice processing %s: %w", voice.ID, delErr))
		}
		return repository.Voice{}, err
	}
	return voice, nil
}

// maskSecret hiện 4 ký tự cuối của 1 API key đã mã hoá, phần còn lại là dấu
// chấm — đủ để người dùng đối chiếu xem đã dán đúng key nào.
//
// Giải mã hỏng thì coi như CHƯA CÓ key (chuỗi rỗng), không bao giờ để lộ
// ciphertext: ciphertext ra ngoài thì việc mã hoá chỉ còn là hình thức.
func maskSecret(box *secret.Box, encrypted string) string {
	raw, err := box.Decrypt(encrypted)
	if err != nil || raw == "" {
		return ""
	}
	runes := []rune(raw)
	if len(runes) <= 4 {
		return strings.Repeat("•", len(runes))
	}
	return "••••" + string(runes[len(runes)-4:])
}

// nullable đổi chuỗi rỗng thành NULL. Cột tuỳ chọn trong DB nên phân biệt
// "không có" với "có nhưng rỗng" — chuỗi rỗng lưu vào chỉ làm câu truy vấn nào
// cũng phải kiểm tra hai trạng thái thay vì một.
func nullable(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

// excerpt cắt text về tối đa n KÝ TỰ (rune, không phải byte — tiếng Việt 1 chữ
// có dấu là nhiều byte, cắt theo byte thì ra ký tự vỡ ở cuối).
func excerpt(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n])) + "…"
}
