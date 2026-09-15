package domain

import (
	"errors"
	"math"
	"testing"
)

func TestAIPriceCostPerKind(t *testing.T) {
	cases := []struct {
		name          string
		price         AIPrice
		inTok, outTok int64
		chars         int64
		seconds       float64
		want          float64
	}{
		{
			// 1M token vào × $3 + 0.5M token ra × $15 = 3 + 7.5
			name:  "llm tính theo 1 triệu token",
			price: AIPrice{Kind: AIKindLLM, InputPerMTok: 3, OutputPerMTok: 15},
			inTok: 1_000_000, outTok: 500_000,
			want: 10.5,
		},
		{
			name:  "tts tính theo ký tự, token bị bỏ qua",
			price: AIPrice{Kind: AIKindTTS, PerMChars: 20, InputPerMTok: 999},
			inTok: 1_000_000, chars: 250_000,
			want: 5,
		},
		{
			name:    "stt tính theo phút",
			price:   AIPrice{Kind: AIKindSTT, PerMinute: 0.006},
			seconds: 600,
			want:    0.06,
		},
		{
			// Chưa khai giá thì ra 0 — và người gọi phải phân biệt nó với
			// "miễn phí" bằng HasPrice, không phải bằng con số này.
			name:  "chưa khai giá thì bằng 0",
			price: AIPrice{Kind: AIKindLLM},
			inTok: 9_000_000,
			want:  0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.price.Cost(tc.inTok, tc.outTok, tc.chars, tc.seconds)
			if math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("Cost = %v, muốn %v", got, tc.want)
			}
		})
	}
}

func TestValidateAIPrices(t *testing.T) {
	cases := []struct {
		name    string
		table   AIPriceTable
		wantErr bool
	}{
		{"bảng rỗng hợp lệ", AIPriceTable{}, false},
		{
			"đủ trường",
			AIPriceTable{Prices: []AIPrice{
				{Kind: AIKindLLM, Provider: "anthropic", Model: "claude-haiku-4-5-20251001"},
				{Kind: AIKindTTS, Provider: "3voices"},
			}},
			false,
		},
		{
			"loại không hợp lệ",
			AIPriceTable{Prices: []AIPrice{{Kind: "image", Provider: "openai"}}},
			true,
		},
		{
			"thiếu nhà cung cấp",
			AIPriceTable{Prices: []AIPrice{{Kind: AIKindLLM}}},
			true,
		},
		{
			"đơn giá âm",
			AIPriceTable{Prices: []AIPrice{
				{Kind: AIKindLLM, Provider: "openai", InputPerMTok: -1},
			}},
			true,
		},
		{
			// Hai dòng cùng khoá thì dòng nào thắng là tuỳ thứ tự duyệt —
			// chặn ngay lúc lưu thay vì để chi phí đổi theo may rủi.
			"trùng khoá",
			AIPriceTable{Prices: []AIPrice{
				{Kind: AIKindLLM, Provider: "gemini", Model: "gemini-2.5-flash"},
				{Kind: AIKindLLM, Provider: "Gemini", Model: "gemini-2.5-flash"},
			}},
			true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAIPrices(tc.table)
			if tc.wantErr && err == nil {
				t.Fatal("muốn lỗi, nhận nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("muốn hợp lệ, nhận lỗi: %v", err)
			}
			if tc.wantErr && !errors.Is(err, ErrInvalidInput) {
				t.Errorf("lỗi phải bọc ErrInvalidInput để handler trả 400, nhận %v", err)
			}
		})
	}
}

// Khoá phải bỏ qua hoa thường của nhà cung cấp nhưng GIỮ NGUYÊN model: tên
// model là ID đối với nhà cung cấp, hạ chữ có thể đụng vào một ID phân biệt
// hoa thường.
func TestPriceKeyNormalisation(t *testing.T) {
	if PriceKey("LLM", "Anthropic", "claude-haiku") != PriceKey("llm", "anthropic", "claude-haiku") {
		t.Error("loại và nhà cung cấp phải so không phân biệt hoa thường")
	}
	if PriceKey("llm", "anthropic", "Claude") == PriceKey("llm", "anthropic", "claude") {
		t.Error("model phải giữ nguyên hoa thường")
	}
}

func TestLLMUsageAdd(t *testing.T) {
	got := LLMUsage{InputTokens: 10, OutputTokens: 3}.Add(LLMUsage{InputTokens: 5, OutputTokens: 1})
	if got.InputTokens != 15 || got.OutputTokens != 4 {
		t.Errorf("Add = %+v, muốn {15 4}", got)
	}
}
