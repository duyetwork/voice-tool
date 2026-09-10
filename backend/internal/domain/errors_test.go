package domain

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// Lỗi hiện lên UI phải nói được CÁI GÌ hỏng. Câu chung chung kiểu "lỗi hệ
// thống, xem log" biến mọi sự cố khác nhau thành cùng một chữ, trong khi người
// dùng không đọc được log của server.
func TestUserMessageKhongBaoChungChung(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			"UserError thắng — câu đã viết sẵn cho người dùng",
			fmt.Errorf("TTS: %w", Explain("API key 3voices không hợp lệ",
				errors.New("3voices trả về 401"))),
			"API key 3voices không hợp lệ",
		},
		{
			"sentinel có câu riêng",
			fmt.Errorf("mode C: %w", ErrPromptRequired),
			"Hình thức C bắt buộc chọn Prompt mẫu",
		},
		{
			"lỗi lạ thì trả nguyên văn, không nuốt thành câu chung chung",
			errors.New("3voices trả về 400: Provide voice attributes"),
			"3voices trả về 400: Provide voice attributes",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := UserMessage(tc.err); got != tc.want {
				t.Errorf("UserMessage() = %q, muốn %q", got, tc.want)
			}
		})
	}
}

// Lỗi nhiều dòng (stderr yt-dlp) chỉ lấy dòng đầu và cắt ngắn — đủ để biết
// hỏng gì, không đủ để tràn bảng.
func TestUserMessageCatNgan(t *testing.T) {
	multi := errors.New("exit status 1\nERROR: chi tiết dài dòng\nvà dòng nữa")
	if got := UserMessage(multi); got != "exit status 1" {
		t.Errorf("UserMessage() = %q, muốn dòng đầu", got)
	}

	long := errors.New(strings.Repeat("x", maxUserMessageRunes+50))
	got := UserMessage(long)
	if n := len([]rune(got)); n != maxUserMessageRunes+1 { // +1 cho dấu …
		t.Errorf("độ dài = %d, muốn %d", n, maxUserMessageRunes+1)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("lỗi bị cắt phải có dấu …, được %q", got[len(got)-10:])
	}
}

func TestUserMessageRong(t *testing.T) {
	if got := UserMessage(nil); got != "" {
		t.Errorf("UserMessage(nil) = %q, muốn rỗng", got)
	}
}
