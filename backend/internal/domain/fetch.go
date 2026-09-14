package domain

import "errors"

// FetchBlockKind phân loại vì sao nền tảng không cho lấy bài.
//
// Tồn tại để ĐẾM ĐƯỢC, không phải để hiển thị: câu hiện cho người dùng đã có
// sẵn trong UserError. Cái thiếu là dữ liệu trả lời câu hỏi "có đáng mua proxy
// không" — và câu đó chỉ trả lời được bằng số lần bị chặn theo từng nền tảng,
// tách riêng loại chặn IP (proxy giải quyết được) khỏi loại đòi đăng nhập
// (proxy không giải quyết được, phải có cookies).
type FetchBlockKind string

const (
	// FetchBlockBot — "confirm you're not a bot": nền tảng nghi IP máy chủ.
	// Đây là loại DUY NHẤT mà proxy giải quyết trực tiếp.
	FetchBlockBot FetchBlockKind = "bot_block"
	// FetchBlockLogin — đòi đăng nhập. Cần cookies, không phải proxy.
	FetchBlockLogin FetchBlockKind = "login_required"
	// FetchBlockRateLimit — HTTP 429. Giảm tần suất trước đã.
	FetchBlockRateLimit FetchBlockKind = "rate_limit"
	FetchBlockGeo       FetchBlockKind = "geo_blocked"
	// FetchBlockUnavailable — bài bị xoá / riêng tư. KHÔNG phải dấu hiệu bị
	// chặn; đếm riêng để nó không làm phồng con số dùng để quyết định proxy.
	FetchBlockUnavailable FetchBlockKind = "unavailable"
	FetchBlockTimeout     FetchBlockKind = "timeout"
	FetchBlockOther       FetchBlockKind = "other"
)

// FetchError gắn nhãn phân loại vào 1 lỗi lấy bài.
type FetchError struct {
	Kind FetchBlockKind
	Err  error
}

func (e *FetchError) Error() string { return e.Err.Error() }
func (e *FetchError) Unwrap() error { return e.Err }

// FetchBlocked bọc lỗi kèm nhãn.
func FetchBlocked(kind FetchBlockKind, err error) error {
	return &FetchError{Kind: kind, Err: err}
}

// FetchBlockKindOf đọc nhãn ra khỏi chuỗi lỗi. false = lỗi này không phải do
// nền tảng chặn (lỗi cấu hình, thiếu ffmpeg, bug của ta) — đếm nó vào bảng
// thống kê chặn chỉ làm nhiễu con số dùng để quyết định mua proxy.
func FetchBlockKindOf(err error) (FetchBlockKind, bool) {
	var fe *FetchError
	if errors.As(err, &fe) {
		return fe.Kind, true
	}
	return "", false
}
