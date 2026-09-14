package domain

import (
	"regexp"
	"sort"
	"strings"
)

// Hashtag của bài gốc là dữ liệu nghiệp vụ, không phải chuyện riêng của một
// adapter: multime nhận hashtag qua trường `hashtags[]` riêng, nên mọi nguồn —
// yt-dlp, thẻ Open Graph, hay text người dùng gõ tay — đều tách hashtag ra
// khỏi nội dung theo đúng một bộ luật ở đây.

// maxHashtags giữ số hashtag ở mức multime.ai chấp nhận được và không biến
// caption thành bãi thẻ.
const maxHashtags = 8

var hashtagRe = regexp.MustCompile(`#[\p{L}\p{N}_]{1,50}`)

// ExtractHashtags lấy hashtag có sẵn trong nội dung bài; kênh không dùng
// hashtag thì fallback sang tags của nền tảng (YouTube keywords, TikTok...).
func ExtractHashtags(text string, tags []string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, maxHashtags)

	add := func(tag string) {
		tag = normalizeHashtag(tag)
		if tag == "" || seen[strings.ToLower(tag)] || len(out) >= maxHashtags {
			return
		}
		seen[strings.ToLower(tag)] = true
		out = append(out, tag)
	}

	for _, m := range hashtagRe.FindAllString(text, -1) {
		add(m)
	}
	if len(out) == 0 {
		sorted := append([]string(nil), tags...)
		sort.SliceStable(sorted, func(i, j int) bool { return len(sorted[i]) < len(sorted[j]) })
		for _, t := range sorted {
			add(t)
		}
	}
	return out
}

// NormalizeHashtags chuẩn hoá một danh sách thẻ ĐÃ TÁCH SẴN (LLM trả về, người
// dùng gõ), theo đúng luật của ExtractHashtags: thêm '#', bỏ ký tự lạ, khử
// trùng không phân biệt hoa thường, cắt ở maxHashtags.
func NormalizeHashtags(tags []string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, maxHashtags)
	for _, t := range tags {
		tag := normalizeHashtag(t)
		if tag == "" || seen[strings.ToLower(tag)] || len(out) >= maxHashtags {
			continue
		}
		seen[strings.ToLower(tag)] = true
		out = append(out, tag)
	}
	return out
}

// StripHashtags bỏ hashtag khỏi text mô tả — chúng đã được tách ra thành
// trường hashtag riêng, để lại trong caption là lặp nội dung.
//
// Chỉ bỏ hashtag đứng riêng; "#1 trending" hay "C#" nằm giữa câu vẫn giữ vì đó
// là chữ chứ không phải thẻ.
func StripHashtags(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))

	for _, line := range lines {
		cleaned := hashtagRe.ReplaceAllString(line, "")
		// Dọn khoảng trắng thừa để lại sau khi bỏ thẻ.
		cleaned = strings.Join(strings.Fields(cleaned), " ")
		out = append(out, cleaned)
	}

	// Gộp các dòng trống liên tiếp sinh ra từ dòng chỉ toàn hashtag.
	var b strings.Builder
	blank := true
	for _, line := range out {
		if line == "" {
			if blank {
				continue
			}
			blank = true
			b.WriteString("\n")
			continue
		}
		if !blank {
			b.WriteString("\n")
		}
		b.WriteString(line)
		blank = false
	}
	return strings.TrimSpace(b.String())
}

// normalizeHashtag bỏ dấu #, khoảng trắng và ký tự lạ: "tin nong" -> "#tinnong".
func normalizeHashtag(raw string) string {
	raw = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "#"))
	if raw == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range raw {
		if r == '_' || isTagRune(r) {
			b.WriteRune(r)
		}
	}
	tag := b.String()
	if tag == "" || len(tag) > 50 {
		return ""
	}
	return "#" + tag
}

// isTagRune: chữ/số ASCII và mọi ký tự Unicode (giữ được tiếng Việt có dấu).
func isTagRune(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r > 127
}
