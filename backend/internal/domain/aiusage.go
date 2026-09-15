package domain

import (
	"fmt"
	"strings"
)

// AIKind — loại dịch vụ AI bị tính tiền. Ba loại tính tiền theo ba đơn vị khác
// nhau, nên chúng không gộp vào một con số được.
const (
	AIKindLLM = "llm" // token vào + token ra
	AIKindTTS = "tts" // ký tự đọc
	AIKindSTT = "stt" // phút audio
)

func ValidAIKind(kind string) bool {
	switch kind {
	case AIKindLLM, AIKindTTS, AIKindSTT:
		return true
	}
	return false
}

// AIUsageEvent là một lần gọi nhà cung cấp AI.
//
// Ghi cả lần THẤT BẠI: một model liên tục lỗi vẫn đốt token đầu vào, và tỉ lệ
// lỗi theo model là nửa còn lại của câu hỏi "model nào đáng tiền".
type AIUsageEvent struct {
	Kind         string
	Provider     string
	Model        string
	InputTokens  int64
	OutputTokens int64
	Characters   int64
	AudioSeconds float64
	OK           bool
}

// ---------------------------------------------------------------------------
// Đơn giá
// ---------------------------------------------------------------------------

// AIPrice là đơn giá của MỘT (loại, nhà cung cấp, model).
//
// Vì sao đơn giá là DỮ LIỆU do admin khai, không phải hằng số trong code: giá
// của cả ba nhà đổi vài lần một năm, khác nhau theo hợp đồng, và một bảng giá
// ghim trong Go nghĩa là mỗi lần nhà cung cấp chỉnh giá lại phải deploy. Tệ hơn
// nữa là bảng giá cũ KHÔNG BÁO LỖI — nó vẫn cho ra một con số, chỉ là con số
// sai, và người đọc không có cách nào biết.
//
// Chưa khai giá thì hệ thống vẫn đếm token đầy đủ và hiển thị "chưa có đơn
// giá". Không bịa số.
type AIPrice struct {
	Kind     string `json:"kind"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	// LLM: USD cho 1 TRIỆU token.
	InputPerMTok  float64 `json:"input_per_mtok"`
	OutputPerMTok float64 `json:"output_per_mtok"`
	// TTS: USD cho 1 TRIỆU ký tự.
	PerMChars float64 `json:"per_mchars"`
	// STT: USD cho 1 PHÚT audio.
	PerMinute float64 `json:"per_minute"`
}

// AIPriceTable là toàn bộ bảng giá, lưu trong app_setting.
type AIPriceTable struct {
	Prices []AIPrice `json:"prices"`
}

// PriceKey định danh một dòng giá. Model rỗng là hợp lệ (TTS/STT của nhà không
// có khái niệm model).
func PriceKey(kind, provider, model string) string {
	return strings.ToLower(strings.TrimSpace(kind)) + "|" +
		strings.ToLower(strings.TrimSpace(provider)) + "|" +
		strings.TrimSpace(model)
}

func (p AIPrice) Key() string { return PriceKey(p.Kind, p.Provider, p.Model) }

// Zero: dòng giá chưa khai gì. Dùng để phân biệt "giá bằng 0" (miễn phí, có
// thật với free tier) khỏi "chưa ai khai giá".
func (p AIPrice) Zero() bool {
	return p.InputPerMTok == 0 && p.OutputPerMTok == 0 && p.PerMChars == 0 && p.PerMinute == 0
}

// Cost quy một lượng dùng ra tiền theo đơn giá này.
func (p AIPrice) Cost(inputTokens, outputTokens, characters int64, audioSeconds float64) float64 {
	switch p.Kind {
	case AIKindLLM:
		return float64(inputTokens)/1e6*p.InputPerMTok +
			float64(outputTokens)/1e6*p.OutputPerMTok
	case AIKindTTS:
		return float64(characters) / 1e6 * p.PerMChars
	case AIKindSTT:
		return audioSeconds / 60 * p.PerMinute
	}
	return 0
}

// ValidateAIPrices kiểm tra bảng giá trước khi lưu.
func ValidateAIPrices(t AIPriceTable) error {
	seen := make(map[string]struct{}, len(t.Prices))
	for i, p := range t.Prices {
		if !ValidAIKind(p.Kind) {
			return fmt.Errorf("%w: dòng %d có loại %q không hợp lệ (llm/tts/stt)",
				ErrInvalidInput, i+1, p.Kind)
		}
		if strings.TrimSpace(p.Provider) == "" {
			return fmt.Errorf("%w: dòng %d thiếu nhà cung cấp", ErrInvalidInput, i+1)
		}
		for _, v := range []float64{p.InputPerMTok, p.OutputPerMTok, p.PerMChars, p.PerMinute} {
			if v < 0 {
				return fmt.Errorf("%w: dòng %d có đơn giá âm", ErrInvalidInput, i+1)
			}
		}
		key := p.Key()
		if _, dup := seen[key]; dup {
			return fmt.Errorf("%w: dòng %d trùng (%s, %s, %s) với một dòng phía trên",
				ErrInvalidInput, i+1, p.Kind, p.Provider, p.Model)
		}
		seen[key] = struct{}{}
	}
	return nil
}
