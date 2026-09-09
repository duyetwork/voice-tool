package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// ytdlpCore là phần dùng chung của mọi adapter chạy trên yt-dlp. YouTube,
// Facebook, TikTok, Instagram và X đều tải bằng đúng 1 công cụ này; adapter
// riêng chỉ khác ở cách nhận diện URL và parse ID.
type ytdlpCore struct {
	runner  CommandRunner
	tempDir string
}

func newCore(runner CommandRunner, tempDir string) ytdlpCore {
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	return ytdlpCore{runner: runner, tempDir: tempDir}
}

// ytEntry là các field của `yt-dlp --dump-json` mà hệ thống dùng tới.
type ytEntry struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Language    string   `json:"language"`
	Thumbnail   string   `json:"thumbnail"`
	Uploader    string   `json:"uploader"`
	Channel     string   `json:"channel"`
	Timestamp   int64    `json:"timestamp"`
	UploadDate  string   `json:"upload_date"`
	Tags        []string `json:"tags"`
	WebpageURL  string   `json:"webpage_url"`
	Duration    float64  `json:"duration"`
}

func (c ytdlpCore) dumpJSON(ctx context.Context, url string) (ytEntry, error) {
	stdout, err := c.runner.Run(ctx, "yt-dlp", "--dump-json", "--no-warnings", "--skip-download", url)
	if err != nil {
		return ytEntry{}, fmt.Errorf("yt-dlp metadata %s: %w", url, err)
	}
	var e ytEntry
	if err := json.Unmarshal(stdout, &e); err != nil {
		return ytEntry{}, fmt.Errorf("parse metadata yt-dlp: %w", err)
	}
	return e, nil
}

func (c ytdlpCore) downloadAudio(ctx context.Context, url, name string) ([]byte, error) {
	dir, err := os.MkdirTemp(c.tempDir, "vt-audio-*")
	if err != nil {
		return nil, fmt.Errorf("tạo temp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	if name == "" {
		name = "audio"
	}
	outTmpl := filepath.Join(dir, name+".%(ext)s")
	if _, err := c.runner.Run(ctx, "yt-dlp",
		"-f", "bestaudio/best", "-x", "--audio-format", "mp3",
		"--no-warnings", "-o", outTmpl, url); err != nil {
		return nil, fmt.Errorf("yt-dlp tách audio %s: %w", url, err)
	}

	data, err := os.ReadFile(filepath.Join(dir, name+".mp3"))
	if err != nil {
		return nil, fmt.Errorf("đọc audio đã tách: %w", err)
	}
	return data, nil
}

func (c ytdlpCore) subtitleText(ctx context.Context, url, name, lang string) (string, error) {
	dir, err := os.MkdirTemp(c.tempDir, "vt-sub-*")
	if err != nil {
		return "", fmt.Errorf("tạo temp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	subLangs := "en,vi"
	if lang != "" && !domain.IsAutoLanguage(lang) {
		subLangs = lang + "," + subLangs
	}
	if name == "" {
		name = "sub"
	}
	outTmpl := filepath.Join(dir, name+".%(ext)s")
	if _, err := c.runner.Run(ctx, "yt-dlp",
		"--skip-download", "--write-subs", "--write-auto-subs",
		"--sub-langs", subLangs, "--sub-format", "vtt",
		"--no-warnings", "-o", outTmpl, url); err != nil {
		return "", fmt.Errorf("yt-dlp lấy phụ đề: %w", err)
	}

	matches, err := filepath.Glob(filepath.Join(dir, "*.vtt"))
	if err != nil || len(matches) == 0 {
		return "", fmt.Errorf("không có file phụ đề")
	}
	raw, err := os.ReadFile(matches[0])
	if err != nil {
		return "", fmt.Errorf("đọc phụ đề: %w", err)
	}
	return ParseVTT(string(raw)), nil
}

// latestPosts liệt kê bài mới nhất của 1 kênh/playlist.
func (c ytdlpCore) latestPosts(ctx context.Context, target string, limit int) ([]domain.RemotePost, error) {
	if limit <= 0 {
		limit = 10
	}
	stdout, err := c.runner.Run(ctx, "yt-dlp",
		"--flat-playlist", "--dump-json", "--no-warnings",
		"--playlist-end", fmt.Sprint(limit), target)
	if err != nil {
		return nil, fmt.Errorf("yt-dlp liệt kê kênh %s: %w", target, err)
	}

	posts := make([]domain.RemotePost, 0, limit)
	for _, line := range strings.Split(string(stdout), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e ytEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		posts = append(posts, domain.RemotePost{
			PostID: e.ID,
			URL:    e.WebpageURL,
			// Text dùng để so khớp regex ở breaking scan.
			Text: strings.TrimSpace(e.Title + "\n" + e.Description),
			Meta: metaFrom(e),
		})
	}
	return posts, nil
}

// fetch chạy chung cho mọi nền tảng: 1 lần lấy metadata, rồi tải audio (mode A)
// hoặc lấy text (mode B/C).
func (c ytdlpCore) fetch(
	ctx context.Context,
	url, name, contentType string,
	mode domain.CollectMode,
) (domain.FetchedContent, error) {
	entry, err := c.dumpJSON(ctx, url)
	if err != nil {
		return domain.FetchedContent{}, err
	}
	meta := metaFrom(entry)

	out := domain.FetchedContent{
		ContentType: contentType,
		Language:    meta.Language,
		Meta:        meta,
	}
	if name == "" {
		name = entry.ID
	}

	if mode == domain.ModeExtract {
		audio, err := c.downloadAudio(ctx, url, name)
		if err != nil {
			return domain.FetchedContent{}, err
		}
		out.AudioBytes = audio
		return out, nil
	}

	// Mode B/C: ưu tiên phụ đề (transcript), fallback title + description.
	text, err := c.subtitleText(ctx, url, name, meta.Language)
	if err != nil || strings.TrimSpace(text) == "" {
		text = strings.TrimSpace(meta.Title + "\n\n" + meta.Description)
	}
	if strings.TrimSpace(text) == "" {
		return domain.FetchedContent{}, domain.Permanent(
			fmt.Errorf("%w: bài %s không có phụ đề lẫn mô tả", domain.ErrNoTextExtracted, name))
	}
	out.Text = text
	return out, nil
}

// ---------------------------------------------------------------------------
// Metadata gốc của bài đăng
// ---------------------------------------------------------------------------

// metaFrom rút metadata phục vụ đăng lại lên multime.ai: tiêu đề, mô tả,
// hashtag, ảnh bìa, tác giả, ngày đăng gốc.
func metaFrom(e ytEntry) domain.PostMetadata {
	author := strings.TrimSpace(e.Uploader)
	if author == "" {
		author = strings.TrimSpace(e.Channel)
	}
	return domain.PostMetadata{
		Title: strings.TrimSpace(e.Title),
		// Mô tả bỏ hashtag: chúng đã tách sang trường Hashtags riêng.
		Description:  StripHashtags(e.Description),
		Hashtags:     ExtractHashtags(e.Title+"\n"+e.Description, e.Tags),
		ThumbnailURL: strings.TrimSpace(e.Thumbnail),
		AuthorName:   author,
		PostedAt:     postedAt(e),
		Language:     strings.ToLower(strings.TrimSpace(e.Language)),
	}
}

func postedAt(e ytEntry) *time.Time {
	if e.Timestamp > 0 {
		t := time.Unix(e.Timestamp, 0).UTC()
		return &t
	}
	if len(e.UploadDate) == 8 {
		if t, err := time.Parse("20060102", e.UploadDate); err == nil {
			return &t
		}
	}
	return nil
}

// maxHashtags giữ số hashtag ở mức multime.ai chấp nhận được và không biến
// caption thành bãi thẻ.
const maxHashtags = 8

var hashtagRe = regexp.MustCompile(`#[\p{L}\p{N}_]{1,50}`)

// ExtractHashtags lấy hashtag có sẵn trong tiêu đề/mô tả; kênh không dùng
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
