package domain

import (
	"errors"
	"strings"
)

var (
	ErrNotFound         = errors.New("not found")
	ErrInvalidInput     = errors.New("invalid input")
	ErrUnsupportedURL   = errors.New("không nhận diện được nền tảng từ URL")
	ErrUnsupportedType  = errors.New("không nhận diện được loại nội dung từ URL")
	ErrPromptRequired   = errors.New("mode C bắt buộc phải có prompt_id")
	ErrLangUnsupported  = errors.New("ngôn ngữ không được AI engine hỗ trợ")
	ErrNoVoiceFile      = errors.New("voice không còn file (đã publish)")
	ErrAlreadyPublished = errors.New("voice đã được publish")
	ErrUnauthorized     = errors.New("unauthorized")
	// ErrForbidden: đã đăng nhập nhưng không được đụng vào dữ liệu của người
	// khác (ví dụ API key TTS của user khác).
	ErrForbidden    = errors.New("forbidden")
	ErrRegexInvalid = errors.New("regex pattern không hợp lệ")
	// ErrDuplicate: đã có Bài Post cho đúng ID bài đăng này (dedup theo
	// (platform, post_id_extracted), không theo URL).
	ErrDuplicate       = errors.New("bài đăng đã có trong hệ thống")
	ErrNoTextExtracted = errors.New("không lấy được text nguồn cho mode B/C")
	// ErrTokenExpired: access token multime hết hạn — thử refresh trước.
	ErrTokenExpired = errors.New("access token multime đã hết hạn")
	// ErrReloginRequired: refresh cũng thất bại — chỉ user đó đăng nhập lại mới
	// khôi phục được.
	ErrReloginRequired = errors.New("cần đăng nhập lại multime")
	// ErrTOTPRequired: tài khoản bật 2FA nên login không trả về token.
	ErrTOTPRequired = errors.New("tài khoản đang bật 2FA, chưa hỗ trợ đăng nhập")
)

// PermanentError — lỗi vĩnh viễn (bài bị xoá, URL chết...).
// Business rule #6: lỗi tạm thời thì KHÔNG cập nhật last_synced_post_id (để retry
// đúng bài đó); lỗi vĩnh viễn thì VẪN cập nhật để không kẹt vĩnh viễn.
type PermanentError struct{ Err error }

func (e *PermanentError) Error() string { return e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }

func Permanent(err error) error { return &PermanentError{Err: err} }

func IsPermanent(err error) bool {
	var pe *PermanentError
	return errors.As(err, &pe)
}

// ---------------------------------------------------------------------------
// Lỗi hiển thị cho người dùng
// ---------------------------------------------------------------------------

// UserError bọc lỗi kỹ thuật kèm 1 câu tiếng Việt ngắn để hiện lên UI.
//
// Chuỗi lỗi gốc (stderr của yt-dlp, exit code, URL...) vẫn giữ trong `Err` để
// ghi log và debug; chỉ `Msg` được lưu vào `last_error` và hiện cho người dùng
// — họ cần biết "phải làm gì", không cần đọc stack của yt-dlp.
type UserError struct {
	Msg string
	Err error
}

func (e *UserError) Error() string {
	if e.Err == nil {
		return e.Msg
	}
	return e.Msg + " (" + e.Err.Error() + ")"
}

func (e *UserError) Unwrap() error { return e.Err }

// Explain gắn câu giải thích ngắn vào 1 lỗi kỹ thuật.
func Explain(msg string, err error) error { return &UserError{Msg: msg, Err: err} }

// userMessages map các lỗi sentinel sang câu giải thích ngắn.
var userMessages = []struct {
	target error
	msg    string
}{
	{ErrUnsupportedURL, "Không nhận diện được nền tảng từ link này"},
	{ErrUnsupportedType, "Link không đúng dạng bài đăng của nền tảng"},
	{ErrPromptRequired, "Hình thức C bắt buộc chọn Prompt mẫu"},
	{ErrLangUnsupported, "AI engine đang bật không hỗ trợ ngôn ngữ này"},
	{ErrNoTextExtracted, "Bài này không có phụ đề lẫn nội dung text để đọc"},
	{ErrNoVoiceFile, "Voice không còn file (đã đăng lên multime)"},
	{ErrTokenExpired, "Phiên multime đã hết hạn — đăng nhập lại"},
	{ErrReloginRequired, "Phiên multime đã hết hạn — đăng nhập lại"},
	{ErrTOTPRequired, "Tài khoản đang bật 2FA, chưa hỗ trợ"},
	{ErrNotFound, "Không tìm thấy bản ghi"},
	{ErrDuplicate, "Bài đăng này đã có trong hệ thống"},
}

// maxUserMessageRunes giới hạn độ dài câu lỗi hiện lên UI. Đủ để đọc được nhà
// cung cấp nào nói gì, không đủ để nguyên trang stderr của yt-dlp tràn ra bảng.
const maxUserMessageRunes = 300

// UserMessage rút ra câu ngắn để lưu vào `last_error` và hiện lên UI.
//
// Ưu tiên UserError gần nhất trong chuỗi lỗi (câu đã viết sẵn cho người dùng);
// không có thì map theo sentinel; cuối cùng mới dùng chính nội dung lỗi.
//
// KHÔNG trả câu chung chung kiểu "lỗi hệ thống, xem log": người dùng không đọc
// được log của server, và một dòng như thế biến mọi sự cố khác nhau — key sai,
// hết credit, bài không có text — thành cùng một chữ. Thà đưa nguyên văn lỗi
// kỹ thuật đã cắt ngắn còn hơn, vì ít nhất nó nói được cái gì hỏng.
func UserMessage(err error) string {
	if err == nil {
		return ""
	}
	var ue *UserError
	if errors.As(err, &ue) {
		return truncateMessage(ue.Msg)
	}
	for _, m := range userMessages {
		if errors.Is(err, m.target) {
			return m.msg
		}
	}
	return truncateMessage(firstLine(err.Error()))
}

// firstLine lấy dòng đầu của lỗi: stderr nhiều dòng thì dòng đầu là nguyên
// nhân, phần sau là stack/tham số chỉ có ích trong log.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i > 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

func truncateMessage(s string) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= maxUserMessageRunes {
		return string(r)
	}
	return string(r[:maxUserMessageRunes]) + "…"
}
