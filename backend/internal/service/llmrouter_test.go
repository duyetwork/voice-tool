package service

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/repository"
)

// Con số nhà cung cấp đưa ra luôn thắng: họ biết chính xác khi nào hạn mức hồi,
// ta thì đoán.
func TestCooldownForUuTienRetryAfter(t *testing.T) {
	fail := &domain.LLMError{Kind: domain.LLMFailQuota, RetryAfter: 90 * time.Second}
	if got := cooldownFor(fail, 5); got != 90*time.Second {
		t.Errorf("cooldownFor() = %v, muốn 90s", got)
	}
}

// Không có Retry-After thì nghỉ tăng dần theo số lần hỏng liên tiếp: 429 lẻ tẻ
// chỉ nghỉ 1 phút, còn key đã cạn hạn mức NGÀY thì tự leo lên mức hàng giờ thay
// vì hỏi lại mỗi phút suốt cả ngày.
func TestCooldownForTangDanVaCoTran(t *testing.T) {
	fail := &domain.LLMError{Kind: domain.LLMFailQuota}

	first := cooldownFor(fail, 0)
	if first != quotaCooldownMin {
		t.Errorf("lần hỏng đầu = %v, muốn %v", first, quotaCooldownMin)
	}

	if second := cooldownFor(fail, 1); second <= first {
		t.Errorf("lần hỏng thứ hai = %v, phải dài hơn %v", second, first)
	}

	// Key hỏng cả trăm lần cũng không được nghỉ quá trần — quá trần thì một
	// sự cố thoáng qua khoá key lại hàng ngày.
	if got := cooldownFor(fail, 100); got != quotaCooldownMax {
		t.Errorf("cooldownFor(100 lần hỏng) = %v, muốn đúng trần %v", got, quotaCooldownMax)
	}
}

// Cột cooldown_until không được xoá sau khi hết hạn (router chỉ so với now()),
// nên đọc thẳng cột đó sẽ hiện "đang nghỉ" cho một key đã khoẻ lại từ lâu.
func TestKeyHealth(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)

	cases := []struct {
		name string
		key  repository.LlmApiKey
		want string
	}{
		{"key mới khai", repository.LlmApiKey{}, "ok"},
		{"đang nghỉ", repository.LlmApiKey{CooldownUntil: &future}, "cooldown"},
		{"nghỉ xong rồi", repository.LlmApiKey{CooldownUntil: &past}, "ok"},
		{"đã tắt vì key sai", repository.LlmApiKey{DisabledAt: &past}, "disabled"},
		{
			// Tắt thắng nghỉ: key sai thì chờ bao lâu cũng không tự khỏi.
			"vừa tắt vừa đang nghỉ",
			repository.LlmApiKey{DisabledAt: &past, CooldownUntil: &future},
			"disabled",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := keyHealth(tc.key); got != tc.want {
				t.Errorf("keyHealth() = %q, muốn %q", got, tc.want)
			}
		})
	}
}

// Nhãn key phải nói được "key nào, model nào" để câu lỗi "bộ API đã cạn" còn
// dùng được: người dùng cần biết đi nạp tiền cho nhà nào, dán lại key nào.
func TestKeyLabel(t *testing.T) {
	label := "công ty"
	got := keyLabel(repository.LlmApiKey{Provider: "gemini", Label: &label}, "gemini-2.5-flash")
	for _, want := range []string{"gemini", "công ty", "gemini-2.5-flash"} {
		if !strings.Contains(got, want) {
			t.Errorf("nhãn %q thiếu %q", got, want)
		}
	}

	// Không đặt label thì vẫn phải ra nhà + model, không được trống hoác.
	bare := keyLabel(repository.LlmApiKey{Provider: "openai"}, "gpt-5.6-luna")
	if bare != "openai / gpt-5.6-luna" {
		t.Errorf("nhãn không label = %q", bare)
	}
}

// Rải lệch phải TẤT ĐỊNH theo id: ngẫu nhiên thì mỗi vòng dispatch lại xáo lại
// thứ tự và hai kênh vẫn có lúc rơi trúng nhau.
func TestScanJitterTatDinhVaTrongCuaSo(t *testing.T) {
	id := uuid.New().String()
	window := 30 * time.Second

	first := ScanJitter(id, window)
	if second := ScanJitter(id, window); first != second {
		t.Errorf("cùng id cho 2 giá trị khác nhau: %v vs %v", first, second)
	}
	if first < 0 || first >= window {
		t.Errorf("ScanJitter() = %v, phải nằm trong [0, %v)", first, window)
	}
	if got := ScanJitter(id, 0); got != 0 {
		t.Errorf("cửa sổ 0 phải cho 0, được %v", got)
	}
}
