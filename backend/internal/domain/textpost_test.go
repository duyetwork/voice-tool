package domain

import (
	"slices"
	"strings"
	"testing"
)

// Bài nhập tay bằng text đi đúng luật của bài lấy từ nền tảng: tiêu đề là
// toàn bộ nội dung TRỪ hashtag, hashtag tách sang trường riêng.
func TestTextPostMetadata(t *testing.T) {
	meta := TextPostMetadata("Bản tin sáng nay\nCháy lớn tại KCN. #tinnong #khancap")

	want := "Bản tin sáng nay\nCháy lớn tại KCN."
	if meta.Title != want {
		t.Errorf("Title = %q, muốn %q", meta.Title, want)
	}
	if !slices.Equal(meta.Hashtags, []string{"#tinnong", "#khancap"}) {
		t.Errorf("Hashtags = %v", meta.Hashtags)
	}
}

// Text dài hơn giới hạn tiêu đề Bài Post vẫn ra tiêu đề hợp lệ (bị cắt), không
// làm hỏng bản ghi.
func TestTextPostMetadataCatTheoGioiHan(t *testing.T) {
	meta := TextPostMetadata(strings.Repeat("chi tiet ", 1000))

	if n := len([]rune(meta.Title)); n > MaxPostTitleRunes+1 {
		t.Errorf("Title dài %d rune, muốn <= %d", n, MaxPostTitleRunes+1)
	}
	if n := len([]rune(VoiceTitle(meta.Title))); n > MaxVoiceTitleRunes+1 {
		t.Errorf("VoiceTitle dài %d rune, muốn <= %d", n, MaxVoiceTitleRunes+1)
	}
}

func TestNormalizeTTSText(t *testing.T) {
	if got := NormalizeTTSText("  dòng 1\r\ndòng 2\r\n  "); got != "dòng 1\ndòng 2" {
		t.Errorf("NormalizeTTSText() = %q", got)
	}
	if got := NormalizeTTSText("   \n  "); got != "" {
		t.Errorf("text toàn khoảng trắng phải về rỗng, được %q", got)
	}
}
