package service

import (
	"strings"
	"testing"
)

// TestMergeHashtagsKeepsBothSources: hashtag người dùng gõ và hashtag của bài
// gốc đều mang thông tin — bỏ bên nào cũng mất. Trùng nhau (khác hoa thường,
// có/không dấu #) thì chỉ giữ một.
func TestMergeHashtagsKeepsBothSources(t *testing.T) {
	got := mergeHashtags("#TinNong, vietnam", []string{"#tinnong", "#hanoi"})

	if !strings.Contains(got, "#vietnam") || !strings.Contains(got, "#hanoi") {
		t.Errorf("mất hashtag: %q", got)
	}
	if strings.Count(strings.ToLower(got), "#tinnong") != 1 {
		t.Errorf("hashtag trùng bị lặp: %q", got)
	}
	// Người dùng gõ trước thì đứng trước: đó là phần họ chủ động chọn.
	if !strings.HasPrefix(got, "#tinnong") {
		t.Errorf("thứ tự sai, muốn hashtag người dùng đứng đầu: %q", got)
	}
}

func TestMergeHashtagsHandlesEmptySides(t *testing.T) {
	if got := mergeHashtags("", []string{"#a", "#b"}); got != "#a #b" {
		t.Errorf("chỉ có hashtag bài gốc: %q", got)
	}
	if got := mergeHashtags("#a #b", nil); got != "#a #b" {
		t.Errorf("chỉ có hashtag người dùng: %q", got)
	}
	if got := mergeHashtags("   ", nil); got != "" {
		t.Errorf("không có gì thì phải rỗng: %q", got)
	}
}

// keepUserValue là luật "ưu tiên thứ người dùng điền": worker chỉ điền vào ô
// còn trống chứ không ghi đè.
func TestKeepUserValuePrefersUserInput(t *testing.T) {
	user, fetched := ptr("Tiêu đề tôi gõ"), ptr("Tiêu đề lấy từ bài")

	if got := keepUserValue(user, fetched); deref(got) != "Tiêu đề tôi gõ" {
		t.Errorf("giá trị người dùng bị ghi đè: %q", deref(got))
	}
	if got := keepUserValue(nil, fetched); deref(got) != "Tiêu đề lấy từ bài" {
		t.Errorf("để trống thì phải lấy từ bài: %q", deref(got))
	}
	if got := keepUserValue(ptr("   "), fetched); deref(got) != "Tiêu đề lấy từ bài" {
		t.Errorf("chuỗi toàn khoảng trắng vẫn là để trống: %q", deref(got))
	}
}
