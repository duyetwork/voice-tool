package domain

import (
	"errors"
	"strings"
	"testing"
)

// Giá trị sai phải bị chặn NGAY khi lưu, kèm danh sách giá trị đúng: 3voices
// trả 500 cho một từ khoá lạ, và lúc đó lỗi đã nằm trong một job đã chết.
func TestVoiceStyleValidateChanGiaTriLa(t *testing.T) {
	err := VoiceStyle{Pitch: "cheerful"}.Validate()
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, muốn ErrInvalidInput", err)
	}
	if !strings.Contains(err.Error(), "moderate pitch") {
		t.Errorf("lỗi phải liệt kê giá trị hợp lệ, đang là: %v", err)
	}
}

func TestVoiceStyleValidateChanTocDoNgoaiKhoang(t *testing.T) {
	for _, speed := range []float64{0.1, 5} {
		s := speed
		if err := (VoiceStyle{Speed: &s}).Validate(); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("speed %v: err = %v, muốn ErrInvalidInput", speed, err)
		}
	}
	ok := 1.5
	if err := (VoiceStyle{Speed: &ok}).Validate(); err != nil {
		t.Errorf("speed 1.5 phải hợp lệ, err = %v", err)
	}
}

// Form gửi lên "Female" hay " female " đều là cùng một thứ; nhà cung cấp chỉ
// nhận bản chữ thường.
func TestVoiceStyleNormalize(t *testing.T) {
	got := VoiceStyle{Gender: " Female ", Age: "YOUNG ADULT"}.Normalize()
	if got.Gender != "female" || got.Age != "young adult" {
		t.Errorf("Normalize = %+v", got)
	}
	if err := got.Validate(); err != nil {
		t.Errorf("chuẩn hoá xong phải hợp lệ, err = %v", err)
	}
}

// Cấu hình rỗng lưu thành NULL chứ không phải `{}`: NULL trong cột nghĩa là
// "để mặc định", đọc phát biết ngay.
func TestMarshalVoiceStyleRongThanhNil(t *testing.T) {
	raw, err := MarshalVoiceStyle(VoiceStyle{})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if raw != nil {
		t.Errorf("raw = %q, muốn nil", raw)
	}
}

// IsZero là thứ quyết định dùng giọng đã lưu hay sinh giọng mới — chỉ một
// trường được chỉnh cũng đã là "có cấu hình".
func TestVoiceStyleIsZero(t *testing.T) {
	if !(VoiceStyle{}).IsZero() {
		t.Error("cấu hình trống phải là IsZero")
	}
	speed := 1.0
	for name, style := range map[string]VoiceStyle{
		"gender": {Gender: "male"},
		"accent": {Accent: "british accent"},
		"speed":  {Speed: &speed},
	} {
		if style.IsZero() {
			t.Errorf("%s: chỉnh rồi thì không còn IsZero", name)
		}
	}
}

// Cột hỏng không được làm chết job đọc: trả lỗi để log, giọng rơi về mặc định.
func TestParseVoiceStyleDuLieuHong(t *testing.T) {
	if _, err := ParseVoiceStyle([]byte("{khong-phai-json")); err == nil {
		t.Error("JSON hỏng phải trả lỗi để chỗ gọi ghi log")
	}
	got, err := ParseVoiceStyle(nil)
	if err != nil || !got.IsZero() {
		t.Errorf("cột NULL: got = %+v, err = %v", got, err)
	}
}
