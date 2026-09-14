package domain

import (
	"errors"
	"strings"
	"testing"
)

// Cái bẫy đắt nhất của cả tính năng: alias `gpt-5.6` KHÔNG trỏ về Luna mà trỏ
// về Sol, đắt hơn khoảng 25 lần. Viết thiếu hậu tố thì hệ thống vẫn chạy đúng,
// không có lỗi nào hiện ra, chỉ có hoá đơn đội lên — và vì đây là mắt xích chỉ
// chạy khi Gemini đã cạn quota nên rất lâu mới có ai nhận ra.
//
// Nên chuỗi mặc định phải qua được đúng bộ kiểm tra mà admin phải qua.
func TestChuoiDuPhongMacDinhHopLe(t *testing.T) {
	if err := ValidateLLMChain(DefaultLLMChain); err != nil {
		t.Fatalf("DefaultLLMChain không hợp lệ: %v", err)
	}
}

func TestValidateLLMChainChanAlias(t *testing.T) {
	cases := []struct {
		name  string
		chain []LLMChainStep
	}{
		{
			// Đúng cái bẫy: thiếu hậu tố -luna.
			"alias openai không có hậu tố",
			[]LLMChainStep{{LLMOpenAI, "gpt-5.6"}},
		},
		{
			// Cùng lý do: alias Anthropic trôi theo bản mới nhất.
			"alias anthropic không có ngày",
			[]LLMChainStep{{LLMAnthropic, "claude-haiku-4-5"}},
		},
		{
			"model của nhà khác",
			[]LLMChainStep{{LLMGemini, "gpt-5.6-luna"}},
		},
		{
			"nhà cung cấp không tồn tại",
			[]LLMChainStep{{LLMProviderName("cohere"), "command-r"}},
		},
		{
			"chuỗi rỗng",
			nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateLLMChain(tc.chain)
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("ValidateLLMChain() = %v, muốn ErrInvalidInput", err)
			}
		})
	}
}

// Câu lỗi phải NÓI RA danh sách được phép: admin gõ sai một hậu tố thì cần
// thấy ngay chuỗi đúng, không phải đi đọc code.
func TestValidateLLMChainNoiRaModelChoPhep(t *testing.T) {
	err := ValidateLLMChain([]LLMChainStep{{LLMOpenAI, "gpt-5.6"}})
	if err == nil {
		t.Fatal("muốn lỗi")
	}
	if !strings.Contains(err.Error(), "gpt-5.6-luna") {
		t.Errorf("câu lỗi không gợi ý model đúng: %v", err)
	}
}

func TestValidateLLMBatch(t *testing.T) {
	if err := ValidateLLMBatch(DefaultLLMBatch); err != nil {
		t.Fatalf("DefaultLLMBatch không hợp lệ: %v", err)
	}

	bad := []LLMBatchConfig{
		{Size: 0, MaxChars: 12000, WaitMS: 2000},
		{Size: 1000, MaxChars: 12000, WaitMS: 2000},
		{Size: 5, MaxChars: 10, WaitMS: 2000},
		{Size: 5, MaxChars: 12000, WaitMS: -1},
		{Size: 5, MaxChars: 12000, WaitMS: 600000},
	}
	for _, cfg := range bad {
		if err := ValidateLLMBatch(cfg); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("ValidateLLMBatch(%+v) = %v, muốn ErrInvalidInput", cfg, err)
		}
	}
}

// Lỗi chưa được adapter phân loại phải tính là TẠM THỜI: đoán sai theo hướng đó
// chỉ tốn thêm một lần thử, còn đoán sai theo hướng "quota" thì bắt một key còn
// tốt đi nghỉ hàng giờ.
func TestLLMFailureOfMacDinhLaTamThoi(t *testing.T) {
	got := LLMFailureOf(errors.New("mạng đứt"))
	if got.Kind != LLMFailTransient {
		t.Errorf("Kind = %v, muốn transient", got.Kind)
	}

	wrapped := Explain("gọi LLM hỏng", LLMFail(LLMFailQuota, errors.New("429")))
	if got := LLMFailureOf(wrapped); got.Kind != LLMFailQuota {
		t.Errorf("Kind qua lớp bọc = %v, muốn quota", got.Kind)
	}
}
