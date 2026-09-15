package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	// og là đường lấy metadata cho bài KHÔNG có media (bài viết text, bài chỉ
	// có ảnh) — yt-dlp thoát ngay với "no video" nên không đọc được gì.
	og *OpenGraph

	// ownTitle: nền tảng này có tiêu đề riêng do người đăng đặt (YouTube) hay
	// chỉ có caption (Facebook, TikTok, Instagram, X). Quyết định cách dựng
	// nội dung Bài Post — xem title.go.
	ownTitle bool

	// extraArgs chèn vào TRƯỚC tham số riêng của từng lệnh yt-dlp. Hiện chỉ có
	// `--proxy`, nhưng mọi tham số toàn cục khác (`--cookies`, `--source-address`)
	// đều cắm vào đúng chỗ này — 4 lệnh yt-dlp bên dưới dùng chung một đường.
	extraArgs []string
}

// Option tinh chỉnh adapter sau khi dựng. Dùng dạng option vì chỉ môi trường
// production mới cần tới chúng: dev và test dựng adapter bằng 2 tham số như cũ.
type Option func(*ytdlpCore)

// WithProxy đẩy mọi lệnh yt-dlp của nền tảng này qua proxy.
//
// Cấu hình THEO TỪNG NỀN TẢNG chứ không phải một proxy chung: thường chỉ một
// nền tảng chặn IP máy chủ, và đẩy cả YouTube — nơi đang chạy tốt — qua proxy
// là tự thêm một điểm hỏng cho luồng duy nhất đang sống được.
//
// Proxy là bậc xử lý SAU khi `/settings` cho thấy số lần bị chặn đủ nhiều
// (xem service.FetchStats). Đừng bật sẵn.
func WithProxy(proxyURL string) Option {
	return func(c *ytdlpCore) {
		if p := strings.TrimSpace(proxyURL); p != "" {
			c.extraArgs = append(c.extraArgs, "--proxy", p)
		}
	}
}

// apply gom việc áp option cho cả 5 adapter.
func (c ytdlpCore) apply(opts []Option) ytdlpCore {
	for _, opt := range opts {
		if opt != nil {
			opt(&c)
		}
	}
	return c
}

// args ghép tham số toàn cục vào trước tham số của lệnh. Luôn trả về slice
// mới: dùng chung mảng nền với extraArgs thì lệnh sau ghi đè tham số lệnh trước.
func (c ytdlpCore) args(rest ...string) []string {
	out := make([]string, 0, len(c.extraArgs)+len(rest))
	out = append(out, c.extraArgs...)
	return append(out, rest...)
}

func newCore(runner CommandRunner, tempDir string, ownTitle bool) ytdlpCore {
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	return ytdlpCore{runner: runner, tempDir: tempDir, og: NewOpenGraph(0), ownTitle: ownTitle}
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
	stdout, err := c.runner.Run(ctx, "yt-dlp",
		c.args("--dump-json", "--no-warnings", "--skip-download", url)...)
	if err != nil {
		return ytEntry{}, explainYtDlp(err)
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
	if _, err := c.runner.Run(ctx, "yt-dlp", c.args(
		"-f", "bestaudio/best", "-x", "--audio-format", "mp3",
		"--no-warnings", "-o", outTmpl, url)...); err != nil {
		return nil, explainYtDlp(err)
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
	if _, err := c.runner.Run(ctx, "yt-dlp", c.args(
		"--skip-download", "--write-subs", "--write-auto-subs",
		"--sub-langs", subLangs, "--sub-format", "vtt",
		"--no-warnings", "-o", outTmpl, url)...); err != nil {
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
	stdout, err := c.runner.Run(ctx, "yt-dlp", c.args(
		"--flat-playlist", "--dump-json", "--no-warnings",
		"--playlist-end", fmt.Sprint(limit), target)...)
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
			Meta: c.metaFrom(e),
		})
	}
	return posts, nil
}

// metadata chỉ lấy metadata gốc của bài (tiêu đề, mô tả, ảnh bìa, tác giả) —
// không tải audio, không lấy phụ đề. Dùng lúc TẠO Bài Post để bảng hiện được
// nội dung ngay, tách khỏi việc tạo Voice.
//
// yt-dlp không đọc được (bài viết text, bài chỉ có ảnh) thì fallback sang thẻ
// Open Graph của trang; cả hai thất bại thì trả lỗi của yt-dlp vì nó cụ thể hơn.
func (c ytdlpCore) metadata(ctx context.Context, url string) (domain.PostMetadata, error) {
	entry, err := c.dumpJSON(ctx, url)
	if err == nil {
		return c.metaFrom(entry), nil
	}
	if c.og == nil {
		return domain.PostMetadata{}, err
	}
	meta, ogErr := c.og.Fetch(ctx, url)
	if ogErr != nil {
		return domain.PostMetadata{}, err
	}
	return meta, nil
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
		// Mode A cần audio: không có media thì không có đường nào cứu.
		if mode == domain.ModeExtract {
			return domain.FetchedContent{}, err
		}
		// Mode B/C chỉ cần text -> thử đọc nội dung bài qua thẻ Open Graph.
		meta, ogErr := c.og.Fetch(ctx, url)
		if ogErr != nil {
			return domain.FetchedContent{}, err
		}
		// meta.Title đã là toàn bộ nội dung bài (xem PostContent).
		text := strings.TrimSpace(meta.Title)
		if text == "" {
			return domain.FetchedContent{}, domain.Permanent(domain.Explain(
				"Bài này không có nội dung text để đọc", domain.ErrNoTextExtracted))
		}
		return domain.FetchedContent{
			ContentType: contentType,
			Language:    meta.Language,
			Meta:        meta,
			Text:        text,
		}, nil
	}
	meta := c.metaFrom(entry)

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

	// Mode B/C: đọc ĐÚNG nội dung bài mà người dùng nhìn thấy — meta.Title là
	// nội dung đã làm sạch (bỏ số liệu tương tác, tên nền tảng, tên tài khoản bị
	// nối vào đầu, hashtag), cũng chính là tiêu đề Bài Post hiện trên bảng và
	// điền sẵn trong ô "Nội dung đọc".
	//
	// Trước đây chỗ này lấy phụ đề trước, và khi không có phụ đề thì ghép thô
	// `entry.Title + entry.Description`. Cả hai đường đều cho ra thứ khác hẳn cái
	// người dùng thấy: bài TikTok/Facebook có title trùng description nên bị đọc
	// lặp 2 lần, bài chỉ có hashtag thì TTS đọc ra nguyên chuỗi "#fyp #studytok",
	// còn bài có phụ đề thì đọc lời thoại trong video chứ không phải caption.
	//
	// Phụ đề chỉ còn là đường dự phòng: bài không có chữ nào (video thuần hình
	// ảnh) thì lời thoại là thứ duy nhất đọc được.
	text := strings.TrimSpace(meta.Title)
	if text == "" {
		if sub, serr := c.subtitleText(ctx, url, name, meta.Language); serr == nil {
			text = strings.TrimSpace(sub)
		}
	}
	if text == "" {
		return domain.FetchedContent{}, domain.Permanent(domain.Explain(
			"Bài này không có nội dung text để đọc (không có caption/tiêu đề lẫn phụ đề)",
			fmt.Errorf("%w: bài %s", domain.ErrNoTextExtracted, name)))
	}
	out.Text = text
	return out, nil
}

// ---------------------------------------------------------------------------
// Metadata gốc của bài đăng
// ---------------------------------------------------------------------------

// metaFrom rút metadata phục vụ đăng lại lên multime.ai: tiêu đề (= nội dung
// bài), hashtag, ảnh bìa, tác giả, ngày đăng gốc.
func (c ytdlpCore) metaFrom(e ytEntry) domain.PostMetadata {
	author := strings.TrimSpace(e.Uploader)
	if author == "" {
		author = strings.TrimSpace(e.Channel)
	}
	return domain.PostMetadata{
		// Tiêu đề = toàn bộ nội dung bài, trừ hashtag (xem title.go). Truyền
		// kèm tên tài khoản để bóc phần tên bị yt-dlp nối vào đầu tiêu đề.
		Title:        c.postContent(e.Title, e.Description, author),
		Hashtags:     domain.ExtractHashtags(e.Title+"\n"+e.Description, e.Tags),
		ThumbnailURL: strings.TrimSpace(e.Thumbnail),
		AuthorName:   author,
		PostedAt:     postedAt(e),
		Language:     strings.ToLower(strings.TrimSpace(e.Language)),
	}
}

// postContent chọn cách dựng nội dung theo kiểu bài của nền tảng.
func (c ytdlpCore) postContent(title, description, author string) string {
	if c.ownTitle {
		return PostContentTitled(title, description, author)
	}
	return PostContent(title, description, author)
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
