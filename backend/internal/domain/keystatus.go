package domain

import "errors"

// KeyStatus — tình trạng MỚI NHẤT của một API key TTS, nhìn từ câu trả lời của
// nhà cung cấp.
//
// Vì sao cần: từ ngoài không có cách nào biết một key còn dùng được không.
// Key hết credit, key bị thu hồi và key còn tốt trông giống hệt nhau trên màn
// hình — cùng 4 ký tự cuối, cùng một dòng. Người dùng chỉ phát hiện ra khi một
// voice chết, mà lúc đó đã mất một lần chạy và một job phải làm lại.
//
// Nguồn dữ liệu duy nhất là lần gọi TTS gần nhất: mỗi lần đọc xong (thành công
// hay thất bại) hệ thống ghi lại điều vừa học được về key. Không có API "kiểm
// tra key" nào được gọi thêm — thăm dò định kỳ là tốn tiền của người dùng để
// lấy một thông tin mà lần chạy thật sẽ nói cho ta biết.
type KeyStatus string

const (
	// KeyStatusUnknown: chưa chạy lần nào, hoặc lần gần nhất hỏng vì lý do
	// KHÔNG nói gì về key (mạng chập chờn, 3voices lỗi phía họ, text quá dài).
	KeyStatusUnknown KeyStatus = "unknown"
	// KeyStatusOK: lần gọi gần nhất đọc ra audio — key còn sống, còn hạn mức.
	KeyStatusOK KeyStatus = "ok"
	// KeyStatusInvalid: nhà cung cấp từ chối key (401/403). Key sai hoặc đã bị
	// thu hồi — phải khai lại key mới, chờ cũng không tự khỏi.
	KeyStatusInvalid KeyStatus = "invalid"
	// KeyStatusNoCredit: hết credit/token (402). Key vẫn đúng, chỉ cần nạp.
	KeyStatusNoCredit KeyStatus = "no_credit"
	// KeyStatusRateLimited: vượt giới hạn số request (429). Tự khỏi sau ít
	// phút — hiện ra để người dùng khỏi tưởng key hỏng mà đi khai lại key mới.
	KeyStatusRateLimited KeyStatus = "rate_limited"
)

// Valid: giá trị có nằm trong bộ đã khai (khớp CHECK constraint của cột
// ai_engine.key_status) — chặn một chuỗi lạ ghi xuống DB rồi mới vỡ.
func (s KeyStatus) Valid() bool {
	switch s {
	case KeyStatusUnknown, KeyStatusOK, KeyStatusInvalid, KeyStatusNoCredit, KeyStatusRateLimited:
		return true
	}
	return false
}

// KeyFault là lỗi TTS có nói lên điều gì đó về chính cái key.
//
// Tách khỏi UserError vì hai thứ trả lời hai câu hỏi khác nhau: UserError nói
// "voice này hỏng vì sao", KeyFault nói "cái key đang ở tình trạng nào". Một
// lỗi mạng có UserError nhưng không có KeyFault — key vẫn tốt, chỉ là lần này
// không gọi tới nơi.
type KeyFault struct {
	Status KeyStatus
	// Detail: nguyên văn câu nhà cung cấp trả về, để người dùng đối chiếu khi
	// cần mở ticket với họ.
	Detail string
	Err    error
}

func (e *KeyFault) Error() string {
	if e.Err == nil {
		return string(e.Status)
	}
	return e.Err.Error()
}

func (e *KeyFault) Unwrap() error { return e.Err }

// FaultyKey gắn tình trạng key vào một lỗi đã có.
func FaultyKey(status KeyStatus, detail string, err error) error {
	return &KeyFault{Status: status, Detail: detail, Err: err}
}

// KeyStatusOf đọc tình trạng key từ một chuỗi lỗi TTS.
//
// `ok` báo là tìm thấy — phân biệt "lỗi này nói key hỏng" với "lỗi này không
// nói gì về key". Chỉ khi ok mới được ghi đè trạng thái đang lưu: một lần rớt
// mạng không được phép xoá dấu vết của lần 402 trước đó.
func KeyStatusOf(err error) (KeyStatus, string, bool) {
	var fault *KeyFault
	if errors.As(err, &fault) && fault.Status.Valid() {
		return fault.Status, fault.Detail, true
	}
	return KeyStatusUnknown, "", false
}
