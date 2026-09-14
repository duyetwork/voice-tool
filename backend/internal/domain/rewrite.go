package domain

import (
	"encoding/json"
	"strings"
)

// RewriteResult là thứ hình thức C nhận về từ LLM.
//
// Trước đây bước này chỉ trả một chuỗi — lời đọc. Nhưng người dùng vẫn phải tự
// gõ tiêu đề và hashtag cho voice, trong khi LLM vừa đọc xong toàn bộ nội dung
// và biết rõ hơn ai hết bài này nên đặt tên là gì. Nên hợp đồng đầu ra giờ có
// ba phần, và chỉ `Content` là bắt buộc.
type RewriteResult struct {
	// Content là lời đọc — thứ DUY NHẤT đi vào TTS.
	Content string
	// Title là tiêu đề đề xuất. Rỗng thì người gọi tự dựng từ Content.
	Title string
	// Hashtags đã bỏ dấu '#'. Rỗng thì giữ hashtag lấy từ bài gốc.
	Hashtags []string
}

// ParseRewrite đọc kết quả thô của LLM theo hợp đồng JSON ở
// infra/ai/llm/prompt.go.
//
// KHÔNG bao giờ trả lỗi, và đó là điều quan trọng nhất ở đây: prompt mẫu do
// người dùng tự viết, model thì mỗi nhà một tính nết — chuyện model phớt lờ
// yêu cầu JSON và trả thẳng đoạn văn là chuyện sẽ xảy ra. Hỏng ở bước đọc kết
// quả mà làm hỏng cả voice thì hình thức C thành thứ không tin được. Không
// parse ra JSON thì coi toàn bộ chuỗi là lời đọc — đúng hành vi cũ.
func ParseRewrite(raw string) RewriteResult {
	text := strings.TrimSpace(raw)
	if text == "" {
		return RewriteResult{}
	}

	var parsed struct {
		Title    string   `json:"title"`
		Content  string   `json:"content"`
		Hashtags []string `json:"hashtags"`
	}
	if err := json.Unmarshal([]byte(stripCodeFence(text)), &parsed); err != nil {
		return RewriteResult{Content: text}
	}
	content := strings.TrimSpace(parsed.Content)
	if content == "" {
		// JSON hợp lệ nhưng không có lời đọc thì cũng vô dụng như không parse
		// được — quay về coi cả chuỗi là lời đọc thay vì tạo voice rỗng.
		return RewriteResult{Content: text}
	}
	return RewriteResult{
		Content:  content,
		Title:    strings.TrimSpace(parsed.Title),
		Hashtags: NormalizeHashtags(parsed.Hashtags),
	}
}

// stripCodeFence gỡ rào ```json ... ``` mà model hay bọc quanh JSON dù đã được
// dặn là không.
func stripCodeFence(s string) string {
	if !strings.HasPrefix(s, "```") {
		return s
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "```"))
}
