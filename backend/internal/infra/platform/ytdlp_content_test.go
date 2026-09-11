package platform

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// fakeRunner giả lập yt-dlp: trả JSON đã dựng sẵn cho --dump-json, và ghi lại
// xem có lệnh nào đi lấy phụ đề hay không.
type fakeRunner struct {
	dumpJSON    string
	dumpErr     error
	subtitleVTT string
	askedSubs   bool
}

func (f *fakeRunner) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "--write-subs") {
		f.askedSubs = true
		if f.subtitleVTT == "" {
			return nil, errors.New("không có phụ đề")
		}
		// Test không ghi ra file thật -> báo lỗi để subtitleText trả rỗng.
		return nil, errors.New("fake: không ghi file")
	}
	if f.dumpErr != nil {
		return nil, f.dumpErr
	}
	return []byte(f.dumpJSON), nil
}

// TestFetchTextReadsPostContentNotRawTitleDescription khoá lại đúng bug đã
// báo: bài TikTok có `title` trùng hệt `description` và chỉ gồm hashtag.
//
// Trước khi sửa, mode B đọc "title\n\ndescription" nên TTS đọc chuỗi hashtag
// LẶP 2 LẦN, trong khi màn hình hiện "(chưa có tiêu đề)". Giờ text đọc phải
// đúng bằng nội dung bài đã làm sạch — ở bài này là rỗng, nên phải báo lỗi
// "không có nội dung text để đọc" thay vì tạo ra voice đọc hashtag.
func TestFetchRejectsHashtagOnlyPost(t *testing.T) {
	runner := &fakeRunner{dumpJSON: `{
		"id":"7661929625055972615",
		"title":"#studytok #fyp #studywithme",
		"description":"#studytok #fyp #studywithme",
		"uploader":"y.veev"
	}`}
	core := newCore(runner, t.TempDir(), false)

	_, err := core.fetch(context.Background(), "https://www.tiktok.com/@y.veev/video/7661929625055972615",
		"7661929625055972615", domain.ContentVideo, domain.ModeTextToVoice)
	if !errors.Is(err, domain.ErrNoTextExtracted) {
		t.Fatalf("lỗi = %v, muốn ErrNoTextExtracted", err)
	}
	if !domain.IsPermanent(err) {
		t.Errorf("bài không có chữ để đọc thì retry vô nghĩa, phải là PermanentError: %v", err)
	}
}

// Bài kiểu caption: title là bản cắt cụt của description kèm số liệu tương tác
// và tên page. Text đọc phải là caption sạch, không lặp và không có "42K views".
func TestFetchTextMatchesPostTitleForCaptionPlatforms(t *testing.T) {
	runner := &fakeRunner{dumpJSON: `{
		"id":"555",
		"title":"42K views · 824 reactions | Sáng nay Hà Nội mưa to, nhiều tuyến phố ng | VTV24 | Facebook",
		"description":"Sáng nay Hà Nội mưa to, nhiều tuyến phố ngập sâu. #tinnong",
		"uploader":"VTV24"
	}`}
	core := newCore(runner, t.TempDir(), false)

	got, err := core.fetch(context.Background(), "https://www.facebook.com/vtv24/videos/555",
		"555", domain.ContentVideo, domain.ModeTextToVoice)
	if err != nil {
		t.Fatalf("fetch lỗi: %v", err)
	}

	// Đọc đúng thứ hiện trên màn hình: Text phải trùng tiêu đề Bài Post.
	if got.Text != got.Meta.Title {
		t.Errorf("Text = %q, muốn trùng tiêu đề Bài Post %q", got.Text, got.Meta.Title)
	}
	if strings.Contains(got.Text, "views") || strings.Contains(got.Text, "Facebook") {
		t.Errorf("Text còn lẫn số liệu/tên nền tảng: %q", got.Text)
	}
	if strings.Contains(got.Text, "#tinnong") {
		t.Errorf("Text còn hashtag (đã đi vào trường hashtags riêng): %q", got.Text)
	}
	if strings.Count(got.Text, "Sáng nay Hà Nội mưa to") != 1 {
		t.Errorf("nội dung bị lặp: %q", got.Text)
	}
	// Có caption rồi thì không tốn thêm một lượt gọi yt-dlp lấy phụ đề.
	if runner.askedSubs {
		t.Error("đã có nội dung bài mà vẫn đi lấy phụ đề")
	}
}

// YouTube (bài có tiêu đề riêng): đọc tiêu đề video — phần mô tả bên dưới là
// link/timestamp, không phải nội dung bài.
func TestFetchTextUsesVideoTitleForYouTube(t *testing.T) {
	runner := &fakeRunner{dumpJSON: `{
		"id":"abc12345678",
		"title":"Bản tin sáng 11/9: Mưa lớn diện rộng",
		"description":"Đăng ký kênh: https://youtube.com/vtv24\n#tinnong",
		"uploader":"VTV24"
	}`}
	core := newCore(runner, t.TempDir(), true)

	got, err := core.fetch(context.Background(), "https://www.youtube.com/watch?v=abc12345678",
		"abc12345678", domain.ContentVideo, domain.ModeTextToVoice)
	if err != nil {
		t.Fatalf("fetch lỗi: %v", err)
	}
	if got.Text != "Bản tin sáng 11/9: Mưa lớn diện rộng" {
		t.Errorf("Text = %q", got.Text)
	}
	if got.Text != got.Meta.Title {
		t.Errorf("Text %q phải trùng tiêu đề Bài Post %q", got.Text, got.Meta.Title)
	}
}

// Bài không có chữ nào (video thuần hình ảnh) mới rơi về phụ đề — đó là thứ
// duy nhất còn đọc được.
func TestFetchFallsBackToSubtitlesOnlyWhenPostHasNoText(t *testing.T) {
	runner := &fakeRunner{dumpJSON: `{"id":"999","title":"","description":"","uploader":"ai.do"}`}
	core := newCore(runner, t.TempDir(), false)

	_, err := core.fetch(context.Background(), "https://www.tiktok.com/@ai.do/video/999",
		"999", domain.ContentVideo, domain.ModeTextToVoice)
	if !runner.askedSubs {
		t.Error("bài không có chữ thì phải thử lấy phụ đề")
	}
	if !errors.Is(err, domain.ErrNoTextExtracted) {
		t.Errorf("không có cả phụ đề thì phải báo ErrNoTextExtracted, được: %v", err)
	}
}
