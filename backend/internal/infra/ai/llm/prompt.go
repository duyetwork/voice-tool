package llm

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// systemPrompt cố định phần "khung" của Mode C; nội dung Prompt mẫu do user
// cấu hình được đưa vào như chỉ dẫn biên tập.
//
// Dùng chung cho cả 3 nhà: đổi cách nói theo từng nhà nghĩa là cùng một Prompt
// mẫu cho ra giọng văn khác nhau tuỳ key nào còn quota — mà người dùng thì
// không biết hôm nay chạy bằng nhà nào.
const systemPrompt = `Bạn là biên tập viên viết kịch bản đọc (voice-over) cho nội dung mạng xã hội.
Nhiệm vụ: viết lại văn bản nguồn theo đúng chỉ dẫn biên tập được cung cấp.

Yêu cầu đầu ra:
- Chỉ trả về nội dung kịch bản để đọc, không thêm lời dẫn, tiêu đề hay giải thích.
- Không dùng markdown, emoji, hay ký tự đặc biệt gây khó cho hệ thống đọc.
- Viết số, ngày tháng, viết tắt dưới dạng chữ để đọc tự nhiên.
- Giữ nguyên ngôn ngữ của văn bản nguồn, trừ khi chỉ dẫn yêu cầu khác.`

// batchSystemPrompt thêm luật riêng của batch lên trên khung chung.
const batchSystemPrompt = systemPrompt + `

Lần này bạn nhận NHIỀU văn bản nguồn cùng lúc, mỗi văn bản có một số thứ tự.
Viết lại TỪNG văn bản một cách độc lập — không gộp, không tóm tắt chéo, không
tham chiếu giữa các văn bản. Trả về đúng một kết quả cho mỗi số thứ tự đã cho.`

// userMessage ghép chỉ dẫn biên tập với văn bản nguồn.
func userMessage(promptContent, sourceText string) string {
	return fmt.Sprintf("## Chỉ dẫn biên tập\n%s\n\n## Văn bản nguồn\n%s",
		strings.TrimSpace(promptContent), strings.TrimSpace(sourceText))
}

// batchUserMessage đánh số từng mẩu để model gắn kết quả về đúng chỗ.
func batchUserMessage(promptContent string, items []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Chỉ dẫn biên tập\n%s\n\n## Các văn bản nguồn\n",
		strings.TrimSpace(promptContent))
	for i, item := range items {
		fmt.Fprintf(&b, "\n### Văn bản %d\n%s\n", i, strings.TrimSpace(item))
	}
	fmt.Fprintf(&b, "\nTrả về đúng %d kết quả, index từ 0 đến %d.",
		len(items), len(items)-1)
	return b.String()
}

// batchResponse là hình dạng JSON mọi nhà phải trả về.
//
// Khoá theo CHỈ SỐ chứ không phải theo thứ tự mảng: model thỉnh thoảng đảo thứ
// tự hoặc bỏ sót một phần tử, và mảng thuần thì không có cách nào phát hiện —
// kết quả là voice của bài A đọc nội dung của bài B, im lặng và rất khó lần ra.
type batchResponse struct {
	Results []struct {
		Index int    `json:"index"`
		Text  string `json:"text"`
	} `json:"results"`
}

// batchSchema là JSON schema gửi kèm request (structured output native của
// từng nhà). `strict` bật thêm additionalProperties:false cho OpenAI; Gemini
// không nhận khoá đó nên để tắt.
func batchSchema(strict bool) map[string]any {
	item := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"index": map[string]any{
				"type":        "integer",
				"description": "Số thứ tự của văn bản nguồn tương ứng, bắt đầu từ 0.",
			},
			"text": map[string]any{
				"type":        "string",
				"description": "Kịch bản đọc đã viết lại cho văn bản đó.",
			},
		},
		"required": []string{"index", "text"},
	}
	root := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"results": map[string]any{"type": "array", "items": item},
		},
		"required": []string{"results"},
	}
	if strict {
		item["additionalProperties"] = false
		root["additionalProperties"] = false
	}
	return root
}

// parseBatch đổi JSON model trả về thành mảng theo đúng thứ tự đầu vào.
//
// Trả lỗi khi thiếu/thừa/lệch chỉ số thay vì cố vá: người gọi sẽ tự động hạ về
// gọi lẻ từng mẩu (xem LLMRouter.GenerateBatch), và như thế đúng hơn nhiều so
// với việc đoán xem kết quả nào thuộc về mẩu nào.
func parseBatch(raw string, want int) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("model trả về nội dung rỗng cho batch %d mẩu", want)
	}

	var parsed batchResponse
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("parse JSON batch: %w", err)
	}

	out := make([]string, want)
	seen := make([]bool, want)
	for _, r := range parsed.Results {
		if r.Index < 0 || r.Index >= want {
			return nil, fmt.Errorf("batch trả về index %d ngoài khoảng 0–%d", r.Index, want-1)
		}
		if seen[r.Index] {
			return nil, fmt.Errorf("batch trả về trùng index %d", r.Index)
		}
		text := strings.TrimSpace(r.Text)
		if text == "" {
			return nil, fmt.Errorf("batch trả về nội dung rỗng ở index %d", r.Index)
		}
		seen[r.Index] = true
		out[r.Index] = text
	}
	for i, ok := range seen {
		if !ok {
			return nil, fmt.Errorf("batch thiếu kết quả cho index %d/%d", i, want)
		}
	}
	return out, nil
}

// emptyResult là lỗi chung khi model chạy xong nhưng không ra chữ nào. Tính là
// TẠM THỜI: lần gọi sau trên cùng key thường ra kết quả, không có gì cho thấy
// key hỏng.
func emptyResult(name, detail string) error {
	return domain.LLMFail(domain.LLMFailTransient,
		fmt.Errorf("%s trả về nội dung rỗng (%s)", name, detail))
}
