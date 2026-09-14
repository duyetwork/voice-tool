package llm

import (
	"strings"
	"testing"
)

// Batch khoá theo CHỈ SỐ, không theo thứ tự mảng. Đây là thứ chặn lỗi tệ nhất
// của batch: model đảo thứ tự thì voice của bài A đọc nội dung của bài B — im
// lặng, không có lỗi nào, và gần như không thể lần ra khi đã đăng lên multime.
func TestParseBatchSapXepTheoIndex(t *testing.T) {
	raw := `{"results":[
		{"index":2,"text":"ba"},
		{"index":0,"text":"một"},
		{"index":1,"text":"hai"}
	]}`

	got, err := parseBatch(raw, 3)
	if err != nil {
		t.Fatalf("parseBatch lỗi: %v", err)
	}
	want := []string{"một", "hai", "ba"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("kết quả[%d] = %q, muốn %q", i, got[i], want[i])
		}
	}
}

// Thiếu / thừa / lệch chỉ số đều phải TRẢ LỖI, không được cố vá: người gọi sẽ
// tự động hạ về gọi lẻ từng mẩu, và như thế đúng hơn nhiều so với đoán xem kết
// quả nào thuộc về mẩu nào.
func TestParseBatchTuChoiKetQuaKhongKhop(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want int
	}{
		{"thiếu 1 mẩu", `{"results":[{"index":0,"text":"a"},{"index":1,"text":"b"}]}`, 3},
		{"index vượt khoảng", `{"results":[{"index":5,"text":"a"}]}`, 2},
		{"index âm", `{"results":[{"index":-1,"text":"a"}]}`, 2},
		{"trùng index", `{"results":[{"index":0,"text":"a"},{"index":0,"text":"b"}]}`, 2},
		{"nội dung rỗng", `{"results":[{"index":0,"text":"  "},{"index":1,"text":"b"}]}`, 2},
		{"không phải JSON", "xin chào, đây là kết quả:", 2},
		{"rỗng hoàn toàn", "   ", 2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseBatch(tc.raw, tc.want); err == nil {
				t.Error("parseBatch() = nil, muốn lỗi")
			}
		})
	}
}

// Prompt mẫu của người dùng KHÔNG được lọt vào phần model coi là văn bản nguồn
// một cách lẫn lộn: mỗi mẩu phải có số thứ tự riêng để model gắn kết quả về
// đúng chỗ.
func TestBatchUserMessageDanhSoTungMau(t *testing.T) {
	msg := batchUserMessage("viết ngắn lại", []string{"bài một", "bài hai"})

	for _, want := range []string{"Văn bản 0", "Văn bản 1", "bài một", "bài hai", "viết ngắn lại"} {
		if !strings.Contains(msg, want) {
			t.Errorf("thiếu %q trong:\n%s", want, msg)
		}
	}
	if !strings.Contains(msg, "index từ 0 đến 1") {
		t.Errorf("không nói rõ khoảng index:\n%s", msg)
	}
}

// OpenAI bật `strict` thì schema phải ĐÓNG (additionalProperties:false) ở mọi
// cấp, còn Gemini lại không nhận khoá đó — nên hai nhà phải nhận hai bản schema
// khác nhau từ cùng một hàm.
func TestBatchSchemaStrict(t *testing.T) {
	open := batchSchema(false)
	if _, has := open["additionalProperties"]; has {
		t.Error("schema cho Gemini không được có additionalProperties")
	}

	strict := batchSchema(true)
	if v, _ := strict["additionalProperties"].(bool); v {
		t.Error("additionalProperties phải là false")
	}
	if _, has := strict["additionalProperties"]; !has {
		t.Error("schema strict của OpenAI phải đóng ở cấp gốc")
	}

	props, _ := strict["properties"].(map[string]any)
	results, _ := props["results"].(map[string]any)
	item, _ := results["items"].(map[string]any)
	if _, has := item["additionalProperties"]; !has {
		t.Error("schema strict phải đóng ở cả cấp phần tử")
	}
}
