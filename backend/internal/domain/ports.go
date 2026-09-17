package domain

import (
	"context"
	"math/rand/v2"
	"time"
)

// ---------------------------------------------------------------------------
// Platform Adapter — 1 file riêng cho mỗi nền tảng trong adapter/platform/
// ---------------------------------------------------------------------------

type PlatformAdapter interface {
	// Name trả về mã nền tảng ("youtube", "facebook"...).
	Name() Platform
	// DetectPlatform: URL này có thuộc nền tảng của adapter không.
	DetectPlatform(url string) bool
	// ExtractID: parse loại nội dung + ID bài đăng từ URL (specs 1.3).
	ExtractID(url string) (contentType string, postID string, err error)
	// FetchContent: lấy nội dung theo mode (A -> audio, B/C -> text) kèm toàn
	// bộ metadata gốc của bài (nội dung, hashtag, ảnh bìa) để Voice sinh
	// ra được điền sẵn.
	//
	// Nhận cả URL lẫn ID vì nhiều nền tảng (TikTok, Facebook, Instagram) không
	// dựng lại được URL chuẩn chỉ từ ID.
	FetchContent(ctx context.Context, ref PostRef, mode CollectMode) (FetchedContent, error)
	// FetchMetadata chỉ lấy metadata gốc (nội dung, ảnh bìa, tác giả),
	// không tải audio và không lấy phụ đề. Chạy ngay khi TẠO Bài Post để bảng
	// hiện được nội dung trước cả khi tạo Voice.
	FetchMetadata(ctx context.Context, ref PostRef) (PostMetadata, error)
	// FetchLatestPosts: liệt kê bài mới của 1 kênh — dùng cho breaking/scheduled scan.
	FetchLatestPosts(ctx context.Context, channelURL string, limit int) ([]RemotePost, error)
	// CheckChannelScan: nil nếu nền tảng này LIỆT KÊ được bài của một kênh,
	// ngược lại là lỗi đã kèm sẵn câu giải thích cho người dùng.
	//
	// Trả lỗi chứ không trả bool vì chỗ gọi luôn cần cả hai: "được hay không"
	// và "vì sao không" — hai nền tảng không quét được vì hai lý do khác nhau,
	// và người dùng cần biết nên làm gì thay thế.
	//
	// Tách khỏi FetchLatestPosts vì đây là câu hỏi phải trả lời được TRƯỚC khi
	// gọi, ở tầng API, lúc người dùng bấm Thêm kênh. yt-dlp không có extractor
	// nào cho dòng thời gian của X hay trang Facebook — thêm một kênh như thế
	// là tạo ra một job lặng lẽ hỏng mỗi vòng quét, retry ba lần, và không có gì
	// trên giao diện nói vì sao.
	//
	// Lấy bài LẺ từ URL vẫn chạy bình thường trên mọi nền tảng — đây chỉ là
	// giới hạn của việc quét cả kênh.
	CheckChannelScan() error
}

// PostRef trỏ tới 1 bài đăng cụ thể trên nền tảng nguồn.
type PostRef struct {
	URL    string
	PostID string
}

type FetchedContent struct {
	AudioFileURL string // Mode A: URL tạm hoặc object key trên storage
	AudioBytes   []byte // Mode A: khi adapter tải trực tiếp về bytes
	Text         string // Mode B/C: caption hoặc transcript
	ContentType  string
	Language     string // ngôn ngữ nguồn nếu nền tảng cung cấp

	// Meta là metadata gốc của bài, dùng để auto-fill Voice + form đăng bài.
	Meta PostMetadata
}

// PostMetadata gom mọi thứ cần để đăng lại 1 bài lên multime.ai mà không phải
// gõ tay: nội dung bài, hashtag, ảnh bìa.
//
// Không có trường mô tả riêng: multime chỉ hiển thị `title`, nên Title mang
// TOÀN BỘ nội dung bài (trừ hashtag) — xem platform.PostContent.
type PostMetadata struct {
	Title        string
	Hashtags     []string
	ThumbnailURL string
	AuthorName   string
	PostedAt     *time.Time
	// Language là ngôn ngữ nền tảng khai báo — nguồn cho chế độ auto-detect.
	Language string
}

// RemotePost — 1 bài đăng thô lấy từ nền tảng, trước khi thành SourcePost.
type RemotePost struct {
	PostID      string
	URL         string
	ContentType string
	Text        string // caption/title — dùng để so khớp regex ở breaking scan
	Meta        PostMetadata
}

// PlatformRegistry — auto-detect nền tảng từ URL (specs 1.3).
type PlatformRegistry interface {
	Resolve(url string) (PlatformAdapter, error)
	Get(p Platform) (PlatformAdapter, error)
}

// ---------------------------------------------------------------------------
// AI Adapters — 1 file riêng mỗi provider trong adapter/ai/
// ---------------------------------------------------------------------------

type TTSProvider interface {
	Name() string
	Synthesize(ctx context.Context, req SpeechRequest) (audioFile []byte, err error)
	SupportedLanguages() []string
}

// SpeechRequest gom mọi thứ quyết định file audio sinh ra.
//
// Là struct chứ không phải danh sách tham số vì cấu hình giọng còn dài ra:
// 3voices đã có accent, và mỗi lần thêm một tham số mà chữ ký hàm đổi theo thì
// mọi provider (kể cả mock trong test) phải sửa cùng lúc.
type SpeechRequest struct {
	// Text là lời đọc — đúng đoạn chữ sẽ nghe thấy trong file.
	Text string
	// Language rỗng = để nhà cung cấp tự nhận diện từ chính nội dung.
	Language string
	// Style là cấu hình giọng người dùng chọn ở form. Rỗng = giọng mặc định
	// của nhà cung cấp (hoặc giọng đã lưu, nếu credential có VoiceID).
	Style VoiceStyle
}

// TTSCredential là thông tin của 1 AI Engine (bản ghi ai_engine) đủ để dựng
// provider TTS: mỗi user dùng API key của chính mình.
type TTSCredential struct {
	Provider string
	APIKey   string
	// VoiceID: giọng đã lưu bên nhà cung cấp. Rỗng = để nhà cung cấp tự sinh
	// giọng theo mô tả mặc định.
	VoiceID string
}

// TTSFactory dựng provider TTS từ credential của 1 engine cụ thể.
//
// Tồn tại vì API key không còn là hằng số trong .env mà là dữ liệu: mỗi user
// tự khai key của mình ở màn AI Engine, và worker phải đọc bằng key của đúng
// người sở hữu voice.
type TTSFactory interface {
	For(cred TTSCredential) (TTSProvider, error)
}

type STTProvider interface {
	Name() string
	Transcribe(ctx context.Context, audioFile []byte, language string) (text string, err error)
}

// LLMProvider là 1 cặp (nhà cung cấp, model) đã gắn sẵn API key. Nó KHÔNG biết
// gì về bộ API, chuỗi dự phòng hay hạn mức — việc đó thuộc về LLMRouter đứng
// trên nó (xem service/llmrouter.go).
//
// Mọi lỗi trả ra từ đây phải được phân loại bằng domain.LLMFail*: adapter là
// nơi duy nhất đọc được mã lỗi của nhà mình, và router không có cách nào tự
// đoán "429 vì hết quota" khác với "400 vì nội dung bị chặn".
type LLMProvider interface {
	Name() string
	Generate(ctx context.Context, promptContent, sourceText string) (
		resultText string, usage LLMUsage, err error)
	// GenerateBatch viết lại NHIỀU mẩu text trong 1 request, dùng structured
	// output theo JSON schema của từng nhà.
	//
	// Bắt buộc trả về đúng len(items) phần tử theo đúng thứ tự, hoặc trả lỗi —
	// router không có cách nào đoán mẩu nào ứng với kết quả nào, và đoán sai ở
	// đây nghĩa là voice của bài A đọc nội dung của bài B.
	GenerateBatch(ctx context.Context, promptContent string, items []string) (
		[]string, LLMUsage, error)
}

// ---------------------------------------------------------------------------
// Destination Adapter — multime.ai
// ---------------------------------------------------------------------------

// MultimeClient đăng Voice lên multime.ai.
//
// API thật (POST /v1/seller/voice-posts/upload) upload audio và tạo bài đăng
// trong CÙNG 1 request multipart — không phải 2 bước.
type MultimeClient interface {
	PublishVoice(
		ctx context.Context,
		creds MultimeCredentials,
		audio []byte,
		post VoicePostInput,
	) (multimePostURL string, err error)
}

// MultimeAuthenticator là cổng đăng nhập strongbody/multime. Hệ thống dùng
// chung tài khoản đó: đăng nhập bằng nó, và đăng voice cũng bằng nó.
type MultimeAuthenticator interface {
	Login(ctx context.Context, email, password string) (MultimeSession, error)
	// RefreshAccessToken đổi refresh token thành access token mới.
	RefreshAccessToken(ctx context.Context, refreshToken string) (string, error)
}

// MultimeDirectory tra danh bạ tài khoản bên Strongbody.
//
// Tồn tại vì tác giả của bài đăng KHÔNG nhất thiết là người bấm nút đăng: một
// biên tập viên có thể phải đưa voice lên dưới tên một tài khoản khác. Danh
// sách phải lấy từ Strongbody chứ không phải từ app_user của tool, vì tài
// khoản đích có thể chưa bao giờ đăng nhập vào tool này.
type MultimeDirectory interface {
	// RandomUser bốc ngẫu nhiên 1 tài khoản theo giới tính, lọc thêm theo quốc
	// gia nếu countryID > 0.
	//
	// Người dùng không chọn đích danh ai: họ chỉ chọn giới tính (và quốc gia)
	// của giọng đứng tên bài, còn là ai thì để hệ thống rải đều — nên đây là
	// "bốc" chứ không phải "tìm".
	RandomUser(ctx context.Context, token string, gender Gender, countryID int64) (MultimeUser, error)
	// Countries liệt kê quốc gia để người dùng chọn.
	Countries(ctx context.Context, token string) ([]MultimeCountry, error)
	// VoiceHashtags lấy 1 trang danh mục hashtag của voice bên MultiMe.
	//
	// Trả kèm tổng số để người gọi biết còn bao nhiêu trang: danh mục này có
	// ~94.000 mục nên không lấy một lượt được.
	VoiceHashtags(ctx context.Context, token string, page, limit int) ([]MultimeHashtag, int, error)
}

// MultimeHashtag là 1 hashtag của voice bên MultiMe.
//
// Bên đó hashtag KHÔNG phải một thực thể riêng: nó là `category` có
// `type = 'voice'` (strongbody-api: entity.CategoryTypeVoice). Vì thế mới có
// những trường nghe như của danh mục — `slug`, `ordering`, `is_featured`.
//
// KHÔNG có trường ngôn ngữ. Đã kiểm tra tận entity.Category, và
// category_translation_service.go ghi rõ "Voice hashtags are not translated by
// this cron" — nên không tồn tại quan hệ hashtag ↔ ngôn ngữ để mà lấy về.
type MultimeHashtag struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	// NormalizedName là dạng đã chuẩn hoá của MultiMe, dùng để tìm kiếm.
	NormalizedName string `json:"normalized_name"`
	Slug           string `json:"slug"`
	// VoiceTagKind: system | user | campaign. `system` là tag MultiMe tuyển
	// chọn — thứ đáng hiện trước trong ô chọn.
	VoiceTagKind string `json:"voice_tag_kind"`
	IsFeatured   bool   `json:"is_featured"`
	Ordering     int64  `json:"ordering"`
}

// MultimeCountry là 1 quốc gia trong danh mục của Strongbody.
type MultimeCountry struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Code string `json:"code,omitempty"`
}

// Gender là giới tính tài khoản Strongbody. Đúng 3 giá trị strongbody-api nhận
// (`v:"in:male,female,other"` ở api/user/v1).
type Gender string

const (
	GenderMale   Gender = "male"
	GenderFemale Gender = "female"
	GenderOther  Gender = "other"
)

func (g Gender) Valid() bool {
	switch g {
	case GenderMale, GenderFemale, GenderOther:
		return true
	}
	return false
}

// RandomGender bốc nam/nữ cho kênh bật "Random author".
//
// Bỏ `other` khỏi vòng bốc: danh bạ Strongbody gần như không có tài khoản nào
// mang giới tính đó, nên bốc trúng nó nghĩa là danh sách rỗng và voice hỏng ở
// bước đăng — ngẫu nhiên trong hai giá trị dùng được vẫn là ngẫu nhiên.
func RandomGender() Gender {
	if rand.IntN(2) == 0 {
		return GenderMale
	}
	return GenderFemale
}

// MultimeUser là một tài khoản Strongbody chọn được làm tác giả bài đăng.
type MultimeUser struct {
	// ID chính là author_id gửi kèm khi đăng voice.
	ID       int64  `json:"id"`
	Email    string `json:"email"`
	Gender   string `json:"gender"`
	FullName string `json:"full_name"`
	Avatar   string `json:"avatar_url,omitempty"`
}

// MultimeSession là kết quả đăng nhập.
type MultimeSession struct {
	// UserID bên strongbody, đồng thời là author_id khi đăng voice.
	UserID       int64
	Email        string
	FullName     string
	AvatarURL    string
	AccessToken  string
	RefreshToken string
}

// MultimeCredentials là thông tin dùng để đăng voice thay cho 1 user cụ thể.
type MultimeCredentials struct {
	AccessToken string
	AuthorID    int64
}

// VoicePostInput là payload tạo Voice Post trên multime.ai.
type VoicePostInput struct {
	FileName string
	MimeType string

	// AuthorID là tài khoản Strongbody đứng tên bài đăng — bắt buộc, và do
	// người dùng chọn chứ không suy ra từ người bấm nút đăng.
	AuthorID int64

	// Title bắt buộc — API từ chối nếu rỗng — và là TOÀN BỘ phần chữ của bài
	// đăng: 1 dòng, tối đa MaxVoiceTitleRunes ký tự (xem VoiceTitle).
	Title string

	// Language map sang field `lang`. Để rỗng thì multime tự nhận diện từ audio
	// và tự điền, nên không nên đoán rồi gửi bừa.
	Language string
	// SourceLang là gợi ý cho pipeline speech-to-text; rỗng = auto-detect.
	SourceLang string

	// Hashtags/CategoryIDs: API bắt buộc có ÍT NHẤT 1 trong 2.
	Hashtags    []string
	CategoryIDs []int64

	Visibility       string
	IsPublicDownload bool

	// ImageBytes là ảnh bìa (API nhận file, không nhận URL).
	ImageBytes []byte
	ImageName  string

	DurationSeconds int
}

// ---------------------------------------------------------------------------
// Audio prober — đọc metadata kỹ thuật của file audio (ffprobe)
// ---------------------------------------------------------------------------

type AudioProber interface {
	Probe(ctx context.Context, data []byte) (AudioInfo, error)
}

type AudioInfo struct {
	DurationSeconds int
	SampleRate      int
	Codec           string
	MimeType        string
	SizeBytes       int64
}

// ---------------------------------------------------------------------------
// Object Storage — S3-compatible (MinIO dev / AWS S3 prod)
// ---------------------------------------------------------------------------

type Storage interface {
	Put(ctx context.Context, key string, data []byte, contentType string) (url string, err error)
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
	// KeyFromURL: lấy lại object key từ URL đã lưu trong voice_file_url.
	KeyFromURL(url string) string
}

// ---------------------------------------------------------------------------
// Queue — API chỉ enqueue, worker mới xử lý (business rule #10)
// ---------------------------------------------------------------------------

// Lịch quét của Danh sách Định kỳ do cmd/scheduler đọc trực tiếp từ DB, nên ở
// đây không cần API đăng ký/huỷ lịch.
type Enqueuer interface {
	EnqueueVoiceProcess(ctx context.Context, sourcePostID, actorID, voiceID string) error
	// EnqueueVoiceText đọc đoạn text gõ tay đã lưu trên chính Voice — luồng
	// này không có Bài Post nào để trỏ tới.
	EnqueueVoiceText(ctx context.Context, voiceID, actorID string, skipRewrite bool) error
	// EnqueuePostMetadata lấy metadata gốc của Bài Post vừa tạo. Tách khỏi
	// voice:process vì nó chạy trong worker (chỉ worker có yt-dlp) nhưng không
	// được để API phải chờ.
	EnqueuePostMetadata(ctx context.Context, sourcePostID string) error
	EnqueueVoicePublish(ctx context.Context, voiceID, actorID string) error
	// actorID là người BẤM NÚT quét, rỗng = vòng chạy theo lịch. Đi kèm payload
	// chứ không tra lại từ DB: lịch sử quét phải trả lời được "ai quét", và lúc
	// worker chạy thì không còn request nào để hỏi.
	EnqueueBreakingScan(ctx context.Context, listID, actorID string) error
	// EnqueueScheduledScan đẩy MỘT vòng quét ngoài lịch cho 1 kênh Định kỳ.
	//
	// Dùng khi người dùng vừa bật lại một kênh đang tắt: kênh tắt thì scheduler
	// bỏ qua mọi vòng, nên không có câu này thì phải chờ trọn một chu kỳ nữa.
	// Vẫn đi qua đúng handler như vòng theo lịch, nên khung giờ của kênh vẫn
	// được tôn trọng.
	EnqueueScheduledScan(ctx context.Context, listID, actorID string) error
}

// ChannelScan mô tả khả năng quét CẢ KÊNH của một nền tảng.
//
// Nhận diện được URL của một nền tảng KHÔNG có nghĩa là quét được kênh của nó:
// yt-dlp lấy từng bài X / Facebook / Instagram bình thường, nhưng không có
// extractor nào liệt kê được dòng thời gian của chúng.
type ChannelScan struct {
	Platform Platform `json:"platform"`
	Enabled  bool     `json:"enabled"`
	// Reason chỉ có khi Enabled=false: vì sao không, và nên làm gì thay thế.
	Reason string `json:"reason,omitempty"`
}
