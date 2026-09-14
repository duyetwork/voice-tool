package service

import (
	"context"
	"testing"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

func posts(ids ...string) []domain.RemotePost {
	out := make([]domain.RemotePost, 0, len(ids))
	for _, id := range ids {
		out = append(out, domain.RemotePost{PostID: id})
	}
	return out
}

func ids(ps []domain.RemotePost) []string { return postIDs(ps) }

func TestSplitBackfill(t *testing.T) {
	// Danh sách theo thứ tự MỚI NHẤT TRƯỚC, đúng như adapter trả về.
	all := posts("p5", "p4", "p3", "p2", "p1")

	tests := []struct {
		name     string
		limit    int
		kept     []string
		excluded []string
	}{
		{
			// Mặc định của kênh mới: không lấy bài nào có sẵn từ trước.
			name: "limit 0 bỏ hết", limit: 0,
			kept: nil, excluded: []string{"p5", "p4", "p3", "p2", "p1"},
		},
		{
			// Lấy bài cũ = lấy những bài GẦN thời điểm thêm kênh nhất.
			name: "lấy 2 bài mới nhất", limit: 2,
			kept: []string{"p5", "p4"}, excluded: []string{"p3", "p2", "p1"},
		},
		{
			name: "limit vượt số bài có thật", limit: 99,
			kept: []string{"p5", "p4", "p3", "p2", "p1"}, excluded: nil,
		},
		{
			name: "limit âm coi như 0", limit: -3,
			kept: nil, excluded: []string{"p5", "p4", "p3", "p2", "p1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kept, excluded := splitBackfill(all, tc.limit)
			assertIDs(t, "kept", ids(kept), tc.kept)
			assertIDs(t, "excluded", ids(excluded), tc.excluded)
		})
	}
}

// splitBackfill không được sửa slice gốc: cùng một lát cắt còn được dùng để
// ghi mốc sync ngay sau đó.
func TestSplitBackfillKhongSuaSliceGoc(t *testing.T) {
	all := posts("p3", "p2", "p1")
	splitBackfill(all, 1)
	assertIDs(t, "gốc", ids(all), []string{"p3", "p2", "p1"})
}

func TestExcludeIDs(t *testing.T) {
	// Vòng quét thứ hai của kênh Breaking: cửa sổ quét vẫn chứa nguyên những
	// bài cũ đã cố tình bỏ ở vòng đầu, cộng thêm 1 bài vừa đăng.
	window := posts("p6", "p5", "p4", "p3")
	got := excludeIDs(window, []string{"p5", "p4", "p3"})
	assertIDs(t, "còn lại", ids(got), []string{"p6"})

	// Không có gì để loại thì trả về nguyên vẹn.
	assertIDs(t, "rỗng", ids(excludeIDs(window, nil)), []string{"p6", "p5", "p4", "p3"})
}

// postIDs bỏ bài không có id: không nhớ được nó, và dedup cũng không dùng tới.
func TestPostIDsBoQuaIDRong(t *testing.T) {
	assertIDs(t, "postIDs", postIDs(posts("a", "", "b")), []string{"a", "b"})
}

// ---------------------------------------------------------------------------
// ModeGate: điều kiện của hình thức C đọc lại trong lúc chạy
// ---------------------------------------------------------------------------

type fakeLLMReadiness struct {
	ok    bool
	calls int
}

func (f *fakeLLMReadiness) HasRealLLM(context.Context) bool {
	f.calls++
	return f.ok
}

func TestModeGateHinhThucCTheoTrangThaiLLM(t *testing.T) {
	ctx := context.Background()
	probe := &fakeLLMReadiness{ok: false}
	gate := ModeGate{LLM: probe}

	if gate.Allows(ctx, domain.ModePromptToVoice) {
		t.Fatal("chưa có key LLM mà mode C vẫn được cho qua")
	}
	if got := gate.Why(ctx, domain.ModePromptToVoice); got != noLLMReason {
		t.Fatalf("lý do tắt mode C = %q, muốn %q", got, noLLMReason)
	}

	// Người dùng thêm Bộ API key. KHÔNG restart, không dựng lại gate.
	probe.ok = true
	if !gate.Allows(ctx, domain.ModePromptToVoice) {
		t.Fatal("đã có key LLM mà mode C vẫn bị tắt")
	}
	if got := gate.Why(ctx, domain.ModePromptToVoice); got != "" {
		t.Fatalf("mode C đang bật mà vẫn kèm lý do tắt: %q", got)
	}
	if err := gate.Check(ctx, domain.ModePromptToVoice); err != nil {
		t.Fatalf("Check mode C: %v", err)
	}
}

// Mode A/B không hỏi LLM: chúng không viết lại nội dung.
func TestModeGateChiHoiLLMChoModeC(t *testing.T) {
	probe := &fakeLLMReadiness{ok: false}
	gate := ModeGate{LLM: probe}
	for _, m := range []domain.CollectMode{domain.ModeExtract, domain.ModeTextToVoice} {
		if !gate.Allows(context.Background(), m) {
			t.Fatalf("mode %s bị tắt oan vì thiếu LLM", m)
		}
	}
	if probe.calls != 0 {
		t.Fatalf("đã hỏi LLM %d lần cho mode không cần LLM", probe.calls)
	}
}

// Cấu hình tắt hẳn mode C thì lý do phải nói về cấu hình — người vận hành đi
// thêm key sẽ không hiểu vì sao thêm xong vẫn không dùng được.
func TestModeGateCauHinhTatThiKhongDoChoThieuKey(t *testing.T) {
	gate := ModeGate{
		Enabled: []domain.CollectMode{domain.ModeExtract},
		LLM:     &fakeLLMReadiness{ok: true},
	}
	why := gate.Why(context.Background(), domain.ModePromptToVoice)
	if why == noLLMReason {
		t.Fatalf("mode C bị tắt bởi cấu hình nhưng lý do lại đổ cho thiếu key: %q", why)
	}
	if why == "" {
		t.Fatal("mode C bị tắt mà không có lý do nào")
	}
}

func assertIDs(t *testing.T, what string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, muốn %v", what, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s = %v, muốn %v", what, got, want)
		}
	}
}
