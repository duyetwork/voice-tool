package domain

import (
	"encoding/json"
	"fmt"
	"strings"
)

// VoiceStyle — cấu hình giọng đọc cho MỘT voice: thứ người dùng mở ra chỉnh ở
// mục "Cấu hình giọng đọc" của form tạo Voice.
//
// Vì sao là kiểu riêng chứ không phải map[string]string: các giá trị ở đây đi
// thẳng vào request của nhà cung cấp TTS, và họ CHỈ nhận đúng bộ từ khoá đã
// công bố — sai một chữ là cả request hỏng (3voices trả 500 kèm danh sách hợp
// lệ). Chặn ở đây thì người dùng thấy lỗi lúc bấm nút, không phải vài giây sau
// trong cột trạng thái của một job đã chết.
//
// Mọi trường đều tuỳ chọn. Trường để trống = "theo mặc định của nhà cung cấp",
// nên VoiceStyle rỗng cư xử y hệt như trước khi có tính năng này.
type VoiceStyle struct {
	// Gender, Age, Pitch, Accent: mô tả giọng cần sinh ra.
	Gender string `json:"gender,omitempty"`
	Age    string `json:"age,omitempty"`
	Pitch  string `json:"pitch,omitempty"`
	Accent string `json:"accent,omitempty"`
	// Speed: tốc độ đọc, 1.0 là bình thường. Dùng con trỏ để phân biệt "chưa
	// chọn" với "chọn đúng bằng 0" — 0 là giá trị vô nghĩa nhưng vẫn gửi đi
	// được nếu coi nó như chưa chọn.
	Speed *float64 `json:"speed,omitempty"`
}

// Bộ từ khoá hợp lệ của 3voices, chép đúng từ tài liệu của họ.
//
// KHÔNG có khái niệm "style" (normal/cheerful/…) dù tài liệu có chỗ gợi ý —
// gửi `style=normal` bị từ chối. Đừng thêm vào nếu chưa thấy nó ở đây.
var (
	VoiceGenders = []string{"female", "male"}
	VoiceAges    = []string{"child", "teenager", "young adult", "middle-aged", "elderly"}
	VoicePitches = []string{
		"very low pitch", "low pitch", "moderate pitch",
		"high pitch", "very high pitch", "whisper",
	}
	VoiceAccents = []string{
		"american accent", "australian accent", "british accent", "canadian accent",
		"chinese accent", "indian accent", "japanese accent", "korean accent",
		"portuguese accent", "russian accent",
	}
)

// Trần tốc độ đọc. 3voices không công bố khoảng cụ thể; lấy 0.5–2.0 vì ngoài
// khoảng đó giọng méo tới mức không dùng được cho bản tin, và chặn sớm vẫn hơn
// để nhà cung cấp trả 400 với câu lỗi của họ.
const (
	MinVoiceSpeed = 0.5
	MaxVoiceSpeed = 2.0
)

// IsZero: người dùng không chỉnh gì cả.
//
// Quan trọng vì nó là thứ quyết định dùng giọng ĐÃ LƯU hay sinh giọng mới:
// có chỉnh thì cấu hình thắng voice_id (xem ThreeVoices.Synthesize).
func (s VoiceStyle) IsZero() bool {
	return s.Gender == "" && s.Age == "" && s.Pitch == "" && s.Accent == "" && s.Speed == nil
}

// Normalize dọn khoảng trắng và hạ chữ thường — form gửi lên "Female" hay
// " female " đều là cùng một thứ, còn nhà cung cấp chỉ nhận bản chữ thường.
func (s VoiceStyle) Normalize() VoiceStyle {
	clean := func(v string) string { return strings.ToLower(strings.TrimSpace(v)) }
	return VoiceStyle{
		Gender: clean(s.Gender),
		Age:    clean(s.Age),
		Pitch:  clean(s.Pitch),
		Accent: clean(s.Accent),
		Speed:  s.Speed,
	}
}

// Validate trả lỗi kèm DANH SÁCH giá trị đúng, không chỉ "giá trị không hợp lệ":
// người dùng (hoặc người gọi API) phải sửa được ngay mà không đi tra tài liệu.
func (s VoiceStyle) Validate() error {
	for _, f := range []struct {
		name  string
		value string
		valid []string
	}{
		{"giới tính giọng", s.Gender, VoiceGenders},
		{"độ tuổi giọng", s.Age, VoiceAges},
		{"cao độ giọng", s.Pitch, VoicePitches},
		{"giọng vùng miền", s.Accent, VoiceAccents},
	} {
		if f.value == "" {
			continue
		}
		if !containsString(f.valid, f.value) {
			return fmt.Errorf("%w: %s %q không hợp lệ — chọn một trong: %s",
				ErrInvalidInput, f.name, f.value, strings.Join(f.valid, ", "))
		}
	}
	if s.Speed != nil && (*s.Speed < MinVoiceSpeed || *s.Speed > MaxVoiceSpeed) {
		return fmt.Errorf("%w: tốc độ đọc %.2f nằm ngoài khoảng %.1f–%.1f",
			ErrInvalidInput, *s.Speed, MinVoiceSpeed, MaxVoiceSpeed)
	}
	return nil
}

// MarshalVoiceStyle đổi cấu hình sang JSONB để lưu vào cột voice.tts_config.
//
// Cấu hình rỗng trả về nil chứ không phải `{}`: NULL trong cột nghĩa là "để
// mặc định", còn `{}` là một object rỗng phải đọc ra rồi mới biết là rỗng.
func MarshalVoiceStyle(s VoiceStyle) ([]byte, error) {
	s = s.Normalize()
	if s.IsZero() {
		return nil, nil
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(s)
}

// ParseVoiceStyle đọc cột tts_config.
//
// Dữ liệu hỏng KHÔNG làm chết job: cột này chỉ mô tả giọng, đọc bằng giọng mặc
// định vẫn ra đúng nội dung — thà có file audio hơi khác ý hơn là không có gì.
// Lỗi trả về để chỗ gọi ghi log, không để chặn.
func ParseVoiceStyle(raw []byte) (VoiceStyle, error) {
	if len(raw) == 0 {
		return VoiceStyle{}, nil
	}
	var s VoiceStyle
	if err := json.Unmarshal(raw, &s); err != nil {
		return VoiceStyle{}, fmt.Errorf("đọc tts_config: %w", err)
	}
	return s.Normalize(), nil
}

func containsString(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
