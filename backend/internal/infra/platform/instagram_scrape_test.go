package platform

import (
	"testing"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// TestInstagramUsername khoá lại ranh giới "URL này là một KÊNH hay một BÀI".
//
// Nhận nhầm link bài thành kênh là lỗi tốn kém nhất ở màn Thêm kênh: kênh vẫn
// tạo được, vẫn hiện "Đang bật", và im lặng không ra bài nào — không có gì trên
// giao diện nói vì sao.
func TestInstagramUsername(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"hồ sơ", "https://www.instagram.com/vnexpress", "vnexpress"},
		{"hồ sơ có dấu / cuối", "https://instagram.com/vnexpress/", "vnexpress"},
		{"bỏ query", "https://www.instagram.com/vnexpress/?hl=vi", "vnexpress"},
		{"link bài -> từ chối", "https://www.instagram.com/p/ABC123/", ""},
		{"link reel -> từ chối", "https://www.instagram.com/reel/ABC123/", ""},
		{"link share -> từ chối", "https://www.instagram.com/share/p/ABC123/", ""},
		{"trang chủ -> từ chối", "https://www.instagram.com/", ""},
		{"không phải URL -> từ chối", "vnexpress", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := instagramUsername(tc.url)
			if tc.want == "" {
				if err == nil {
					t.Fatalf("mong đợi bị từ chối, nhưng nhận được %q", got)
				}
				// Sai URL thì thử lại bao nhiêu lần cũng vậy — phải là lỗi vĩnh
				// viễn, không thì job retry ba lượt cho một thứ người dùng phải sửa.
				if !domain.IsPermanent(err) {
					t.Errorf("URL sai phải là lỗi vĩnh viễn, nhận: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("lỗi ngoài dự kiến: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// igBody là phản hồi web_profile_info rút gọn, giữ nguyên hình dạng thật.
const igBody = `{"data":{"user":{"edge_owner_to_timeline_media":{"edges":[
  {"node":{"id":"3001","shortcode":"AAA","taken_at_timestamp":1700000200,
    "is_video":true,"product_type":"clips",
    "edge_media_to_caption":{"edges":[{"node":{"text":"Tin nóng #breaking"}}]}}},
  {"node":{"id":"3002","shortcode":"BBB","taken_at_timestamp":1700000100,
    "is_video":false,"product_type":"",
    "edge_media_to_caption":{"edges":[]}}},
  {"node":{"id":"3003","shortcode":"","taken_at_timestamp":1700000000,
    "is_video":false,"edge_media_to_caption":{"edges":[]}}}
]}}}}`

func TestParseInstagramProfile(t *testing.T) {
	posts, err := parseInstagramProfile([]byte(igBody), "vnexpress", 10)
	if err != nil {
		t.Fatalf("lỗi ngoài dự kiến: %v", err)
	}
	// Mục thứ ba không có shortcode nên bị bỏ: không dựng được URL bài.
	if len(posts) != 2 {
		t.Fatalf("mong đợi 2 bài, nhận %d", len(posts))
	}

	// Giữ NGUYÊN thứ tự Instagram trả về, không sắp lại theo thời gian.
	if posts[0].PostID != "3001" || posts[1].PostID != "3002" {
		t.Errorf("sai thứ tự: %q, %q", posts[0].PostID, posts[1].PostID)
	}
	// product_type=clips phải ra reel VÀ ra URL /reel/ — /p/ của một reel làm
	// bước tải về sau đó nhận diện sai loại nội dung.
	if posts[0].ContentType != domain.ContentReel {
		t.Errorf("clips phải là reel, nhận %q", posts[0].ContentType)
	}
	if posts[0].URL != "https://www.instagram.com/reel/AAA/" {
		t.Errorf("URL reel sai: %s", posts[0].URL)
	}
	if posts[1].URL != "https://www.instagram.com/p/BBB/" {
		t.Errorf("URL bài ảnh sai: %s", posts[1].URL)
	}
	if posts[0].Text != "Tin nóng #breaking" {
		t.Errorf("caption sai: %q", posts[0].Text)
	}
	if len(posts[0].Meta.Hashtags) != 1 || posts[0].Meta.Hashtags[0] != "#breaking" {
		t.Errorf("hashtag sai: %v", posts[0].Meta.Hashtags)
	}
	if posts[0].Meta.PostedAt == nil || posts[0].Meta.PostedAt.Unix() != 1700000200 {
		t.Errorf("thời điểm đăng sai: %v", posts[0].Meta.PostedAt)
	}
}

// TestParseInstagramProfileOrder: bài GHIM nằm đầu danh sách Instagram trả về
// mà không có cờ nào phân biệt. Với trần 12 bài/lần gọi, ba bài ghim cũ chiếm
// 1/4 số suất của một vòng quét tin nóng nếu ta tin thứ tự mảng.
func TestParseInstagramProfileOrder(t *testing.T) {
	body := `{"data":{"user":{"edge_owner_to_timeline_media":{"edges":[
	  {"node":{"id":"ghim","shortcode":"PIN","taken_at_timestamp":1600000000,
	    "edge_media_to_caption":{"edges":[]}}},
	  {"node":{"id":"moi","shortcode":"NEW","taken_at_timestamp":1700000900,
	    "edge_media_to_caption":{"edges":[]}}},
	  {"node":{"id":"cu","shortcode":"OLD","taken_at_timestamp":1700000100,
	    "edge_media_to_caption":{"edges":[]}}}
	]}}}}`

	posts, err := parseInstagramProfile([]byte(body), "a", 10)
	if err != nil {
		t.Fatalf("lỗi ngoài dự kiến: %v", err)
	}
	want := []string{"moi", "cu", "ghim"}
	for i, id := range want {
		if posts[i].PostID != id {
			t.Fatalf("thứ tự sai: %v, muốn %v (mới nhất trước)",
				[]string{posts[0].PostID, posts[1].PostID, posts[2].PostID}, want)
		}
	}

	top, err := parseInstagramProfile([]byte(body), "a", 1)
	if err != nil {
		t.Fatalf("lỗi ngoài dự kiến: %v", err)
	}
	if top[0].PostID != "moi" {
		t.Errorf("limit=1 phải lấy bài mới nhất, nhận %q", top[0].PostID)
	}
}

func TestParseInstagramProfileLimit(t *testing.T) {
	posts, err := parseInstagramProfile([]byte(igBody), "vnexpress", 1)
	if err != nil {
		t.Fatalf("lỗi ngoài dự kiến: %v", err)
	}
	if len(posts) != 1 {
		t.Fatalf("limit=1 phải trả 1 bài, nhận %d", len(posts))
	}
}

// TestParseInstagramProfileEmpty: tài khoản không tồn tại là lỗi VĨNH VIỄN và
// mang nhãn `unavailable` — nó không nói gì về sức khoẻ của via hay proxy, nên
// không được phép trừ điểm của chúng.
func TestParseInstagramProfileEmpty(t *testing.T) {
	_, err := parseInstagramProfile([]byte(`{"data":{"user":null}}`), "khongcothat", 10)
	if err == nil {
		t.Fatal("user null phải báo lỗi")
	}
	if !domain.IsPermanent(err) {
		t.Errorf("user null phải là lỗi vĩnh viễn, nhận: %v", err)
	}

	// Hồ sơ đọc được nhưng không có bài: KHÔNG gắn nhãn chặn — gắn `bot_block`
	// ở đây là giết proxy vì một tài khoản trống.
	_, err = parseInstagramProfile(
		[]byte(`{"data":{"user":{"edge_owner_to_timeline_media":{"edges":[]}}}}`), "trong", 10)
	if err == nil {
		t.Fatal("0 bài phải báo lỗi")
	}
	if kind, ok := domain.FetchBlockKindOf(err); ok {
		t.Errorf("0 bài không được gắn nhãn chặn, nhận: %v", kind)
	}
}
