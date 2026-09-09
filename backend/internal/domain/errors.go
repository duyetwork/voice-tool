package domain

import "errors"

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
	ErrRegexInvalid     = errors.New("regex pattern không hợp lệ")
	ErrNoTextExtracted  = errors.New("không lấy được text nguồn cho mode B/C")
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
