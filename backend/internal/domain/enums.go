package domain

import "strings"

// CollectMode — 3 hình thức thu thập voice (specs 1.2).
type CollectMode string

const (
	// ModeExtract (A): tải video/audio gốc, tách trực tiếp track giọng nói.
	ModeExtract CollectMode = "A"
	// ModeTextToVoice (B): lấy caption/transcript gốc -> TTS đọc nguyên văn.
	ModeTextToVoice CollectMode = "B"
	// ModePromptToVoice (C): text gốc -> LLM viết lại theo Prompt mẫu -> TTS.
	ModePromptToVoice CollectMode = "C"
)

// LanguageAuto: không chốt ngôn ngữ, để nền tảng nguồn / multime.ai tự nhận
// diện. Đây là mặc định của hệ thống — đoán sai ngôn ngữ tệ hơn là không đoán.
const LanguageAuto = "auto"

// IsAutoLanguage cho biết giá trị ngôn ngữ có phải chế độ tự nhận diện không.
func IsAutoLanguage(lang string) bool {
	return lang == "" || lang == LanguageAuto
}

func (m CollectMode) Valid() bool {
	switch m {
	case ModeExtract, ModeTextToVoice, ModePromptToVoice:
		return true
	}
	return false
}

// NeedsText: mode B/C cần text nguồn (caption hoặc transcript qua STT).
func (m CollectMode) NeedsText() bool { return m == ModeTextToVoice || m == ModePromptToVoice }

// NeedsPrompt: chỉ mode C dùng Prompt mẫu.
func (m CollectMode) NeedsPrompt() bool { return m == ModePromptToVoice }

// NeedsTTS: mode A không đi qua TTS.
func (m CollectMode) NeedsTTS() bool { return m.NeedsText() }

// AllCollectModes liệt kê 3 hình thức thu thập theo thứ tự A, B, C.
var AllCollectModes = []CollectMode{ModeExtract, ModeTextToVoice, ModePromptToVoice}

// SourceType — nguồn sinh ra SourcePost.
type SourceType string

const (
	SourceF1        SourceType = "F1"
	SourceBreaking  SourceType = "BREAKING"
	SourceScheduled SourceType = "SCHEDULED"
)

// Vòng đời Bài Post.
const (
	PostStatusNew        = "new"
	PostStatusProcessing = "processing"
	PostStatusProcessed  = "processed"
	PostStatusFailed     = "failed"
)

// Vòng đời Voice (specs 4).
const (
	// PublishProcessing: record đã tạo, worker đang tải/tạo audio. Chưa có
	// file nên chưa đăng được — tồn tại để người dùng thấy voice ngay khi bấm
	// tạo, thay vì bảng trống cho tới lúc job xong.
	PublishProcessing = "processing"
	PublishDraft      = "draft"
	PublishReady      = "ready"
	PublishPublished  = "published"
	PublishFailed     = "failed"
)

// Platform — nền tảng nguồn được hỗ trợ.
type Platform string

const (
	PlatformYouTube   Platform = "youtube"
	PlatformFacebook  Platform = "facebook"
	PlatformTikTok    Platform = "tiktok"
	PlatformInstagram Platform = "instagram"
	PlatformX         Platform = "x"
)

// AllPlatforms liệt kê nền tảng hệ thống nhận diện được.
var AllPlatforms = []Platform{
	PlatformYouTube, PlatformFacebook, PlatformTikTok, PlatformInstagram, PlatformX,
}

// ContentType — loại nội dung trong 1 nền tảng.
const (
	ContentVideo = "video"
	ContentShort = "short"
	ContentReel  = "reel"
	ContentPost  = "post"
	ContentStory = "story"
	ContentTweet = "tweet"
)

// MinPublishDurationSeconds là độ dài audio tối thiểu multime.ai chấp nhận.
const MinPublishDurationSeconds = 15

// MaxVoiceTitleRunes giới hạn độ dài tiêu đề Voice gửi lên multime.ai.
//
// Đây là ràng buộc THẬT của nơi đăng, không phải con số tự đặt: form đăng voice
// của multime (`multime-ai/src/components/Studio/UploadVoice.tsx`) và form sửa
// (`MyVoices.tsx`) đều đặt `maxLength={200}` cho ô tiêu đề. Ô đó là `<input>`
// một dòng và là trường bắt buộc — `POST /v1/seller/voice-posts/upload` từ chối
// title rỗng.
//
// Trường `caption` của API tồn tại nhưng chính multime không dùng khi đăng
// (form luôn gửi caption rỗng), nên tiêu đề là TOÀN BỘ phần chữ của bài đăng:
// phải lọt trong 200 ký tự, trên 1 dòng.
const MaxVoiceTitleRunes = 200

// MaxPostTitleRunes chặn tiêu đề Bài Post phình vô hạn.
//
// Tiêu đề Bài Post là toàn bộ nội dung bài (trừ hashtag) nên dài hơn tiêu đề
// Voice rất nhiều — caption Facebook có thể vài nghìn ký tự. Giới hạn này chỉ
// để một bài bất thường không thổi bay bảng danh sách; phần cắt đi không ảnh
// hưởng việc đăng vì Voice chỉ lấy 200 ký tự đầu.
const MaxPostTitleRunes = 5000

// AuditAction — hành động được ghi Audit Log (business rule #8).
type AuditAction string

const (
	AuditCreate  AuditAction = "create"
	AuditUpdate  AuditAction = "update"
	AuditDelete  AuditAction = "delete"
	AuditRun     AuditAction = "run"
	AuditPublish AuditAction = "publish"
)

// ObjectType — 4 entity bắt buộc audit.
const (
	ObjectListBreaking  = "list_breaking"
	ObjectListScheduled = "list_scheduled"
	ObjectSourcePost    = "source_post"
	ObjectVoice         = "voice"
)

// ---------------------------------------------------------------------------
// Phân quyền
// ---------------------------------------------------------------------------

// Role — 3 vai trò. Hệ thống dùng SSO của strongbody: không có đăng ký, ai
// đăng nhập được bằng tài khoản multime thì vào được, admin cấp quyền sau.
//
// Không còn vai trò chỉ-xem: đăng nhập được nghĩa là dùng được, khác nhau chỉ
// ở quyền xoá và quyền cấp quyền.
type Role string

const (
	// RoleAdmin: toàn quyền, kể cả cấp quyền cho người khác.
	RoleAdmin Role = "admin"
	// RoleEditor: toàn quyền nghiệp vụ (kể cả xoá), TRỪ phân quyền.
	RoleEditor Role = "editor"
	// RoleUser: xem tất cả + tạo/chạy/đăng voice. KHÔNG được xoá.
	RoleUser Role = "user"
)

func (r Role) Valid() bool {
	switch r {
	case RoleAdmin, RoleEditor, RoleUser:
		return true
	}
	return false
}

// CanWrite: tạo/sửa Bài Post, Voice, Danh sách; chạy job; đăng voice. Mọi vai
// trò hợp lệ đều làm được — role rỗng/không hợp lệ thì không.
func (r Role) CanWrite() bool { return r.Valid() }

// CanDelete: admin và editor. User bình thường đăng được voice nhưng không xoá
// được dữ liệu của người khác.
func (r Role) CanDelete() bool { return r == RoleAdmin || r == RoleEditor }

// CanManageUsers: cấp quyền, bật/tắt tài khoản — chỉ admin. Đây là điểm duy
// nhất phân biệt admin với editor.
func (r Role) CanManageUsers() bool { return r == RoleAdmin }

// PostTitle chuẩn hoá tiêu đề Bài Post. Giữ nguyên xuống dòng: đây là nội dung
// bài chứ không phải một dòng tiêu đề, ngắt dòng là một phần của nội dung.
func PostTitle(title string) string {
	return truncateRunes(strings.TrimSpace(title), MaxPostTitleRunes)
}

// VoiceTitle dựng tiêu đề gửi lên multime từ tiêu đề Bài Post: gộp về 1 dòng
// (ô tiêu đề bên multime là input một dòng) rồi cắt về MaxVoiceTitleRunes.
//
// Đây là CHỖ DUY NHẤT áp giới hạn của multime, dùng chung cho cả lúc sinh Voice
// lẫn lúc đăng.
func VoiceTitle(title string) string {
	return truncateRunes(strings.Join(strings.Fields(title), " "), MaxVoiceTitleRunes)
}

// truncateRunes cắt ở ranh giới từ để không đứt giữa chữ, thêm … khi đã cắt.
func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	cut := string(runes[:max])
	if i := strings.LastIndexAny(cut, " \n"); i > max/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " \n,.;:-") + "…"
}
