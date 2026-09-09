package domain

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
	PublishDraft     = "draft"
	PublishReady     = "ready"
	PublishPublished = "published"
	PublishFailed    = "failed"
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
type Role string

const (
	// RoleAdmin: toàn quyền, kể cả xoá và cấp quyền cho người khác.
	RoleAdmin Role = "admin"
	// RoleUser: xem tất cả + tạo/chạy/đăng voice. KHÔNG được xoá.
	RoleUser Role = "user"
	// RoleViewer: chỉ xem.
	RoleViewer Role = "viewer"
)

func (r Role) Valid() bool {
	switch r {
	case RoleAdmin, RoleUser, RoleViewer:
		return true
	}
	return false
}

// CanWrite: tạo/sửa Bài Post, Voice, Danh sách; chạy job; đăng voice.
func (r Role) CanWrite() bool { return r == RoleAdmin || r == RoleUser }

// CanDelete: chỉ admin. User bình thường đăng được voice nhưng không xoá được
// dữ liệu của người khác.
func (r Role) CanDelete() bool { return r == RoleAdmin }

// CanManageUsers: cấp quyền, bật/tắt tài khoản.
func (r Role) CanManageUsers() bool { return r == RoleAdmin }
