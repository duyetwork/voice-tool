package service

import (
	"strings"
	"testing"

	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/repository"
)

// TestPublishableRequiresAuthorAndHashtag: hai điều kiện này multime từ chối
// thẳng, nên API phải chặn ngay lúc bấm Đăng thay vì để job chạy rồi fail.
func TestPublishableRequiresAuthorAndHashtag(t *testing.T) {
	authorID := int64(9001)
	hashtag := "tinnong"

	cases := []struct {
		name  string
		voice repository.Voice
		want  string
	}{
		{
			name:  "chưa chọn author",
			voice: repository.Voice{Hashtag: &hashtag},
			want:  "author",
		},
		{
			name:  "author_id = 0 cũng là chưa chọn",
			voice: repository.Voice{AuthorID: ptr(int64(0)), Hashtag: &hashtag},
			want:  "author",
		},
		{
			name:  "không hashtag",
			voice: repository.Voice{AuthorID: &authorID},
			want:  "hashtag",
		},
		{
			// Hashtag mặc định đã bị bỏ khỏi cấu hình -> khoảng trắng không
			// còn được coi là "có hashtag".
			name:  "hashtag chỉ có khoảng trắng",
			voice: repository.Voice{AuthorID: &authorID, Hashtag: ptr("   ")},
			want:  "hashtag",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := publishable(tc.voice)
			if err == nil {
				t.Fatal("phải trả lỗi")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("lỗi %q phải nói về %q", err, tc.want)
			}
			// UI phân biệt lỗi nhập liệu với lỗi hệ thống qua sentinel này.
			if !strings.Contains(err.Error(), domain.ErrInvalidInput.Error()) {
				t.Errorf("lỗi phải bọc ErrInvalidInput, được: %v", err)
			}
		})
	}
}

func TestPublishableAcceptsCompleteVoice(t *testing.T) {
	voice := repository.Voice{AuthorID: ptr(int64(9001)), Hashtag: ptr("tinnong")}
	if err := publishable(voice); err != nil {
		t.Errorf("voice đủ điều kiện bị chặn: %v", err)
	}
}
