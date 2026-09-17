package platform

import (
	"testing"
	"time"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// TestXScreenName — cùng ranh giới với TestInstagramUsername: link tweet lẻ dán
// vào ô "kênh" phải bị từ chối ngay, không phải im lặng tạo ra một kênh chết.
func TestXScreenName(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"hồ sơ x.com", "https://x.com/vnexpress", "vnexpress"},
		{"hồ sơ twitter.com", "https://twitter.com/vnexpress/", "vnexpress"},
		{"có @", "https://x.com/@vnexpress", "vnexpress"},
		{"tab media vẫn là kênh", "https://x.com/vnexpress/media", "vnexpress"},
		{"link tweet -> từ chối", "https://x.com/vnexpress/status/1234567890", ""},
		{"đường dẫn hệ thống -> từ chối", "https://x.com/i/web/status/123", ""},
		{"trang chủ -> từ chối", "https://x.com/", ""},
		{"tên quá dài -> từ chối", "https://x.com/abcdefghijklmnopqrst", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := xScreenName(tc.url)
			if tc.want == "" {
				if err == nil {
					t.Fatalf("mong đợi bị từ chối, nhưng nhận được %q", got)
				}
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

// xBody — trang syndication rút gọn, giữ nguyên hình dạng thật: khối
// __NEXT_DATA__ nằm trong HTML, và dòng thời gian có lẫn entry không phải tweet.
const xBody = `<!DOCTYPE html><html><body>
<script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"timeline":{"entries":[
 {"type":"tweet","entry_id":"tweet-9001","content":{"tweet":{
   "id_str":"9001","full_text":"Nóng: tin mới #breaking",
   "created_at":"Wed Oct 10 20:19:24 +0000 2018",
   "permalink":"/vnexpress/status/9001","user":{"screen_name":"vnexpress"}}}},
 {"type":"timelineMessage","entry_id":"msg-1","content":{}},
 {"type":"tweet","entry_id":"tweet-9002","content":{"tweet":{
   "id_str":"9002","text":"Tin cũ hơn","created_at":"",
   "permalink":"","user":{"screen_name":"vnexpress"}}}}
]}}}}</script>
</body></html>`

func TestParseXTimeline(t *testing.T) {
	posts, err := parseXTimeline([]byte(xBody), "vnexpress", 10)
	if err != nil {
		t.Fatalf("lỗi ngoài dự kiến: %v", err)
	}
	// Entry không phải tweet (quảng cáo, gợi ý theo dõi) phải bị bỏ.
	if len(posts) != 2 {
		t.Fatalf("mong đợi 2 tweet, nhận %d", len(posts))
	}

	if posts[0].PostID != "9001" || posts[1].PostID != "9002" {
		t.Errorf("sai thứ tự: %q, %q", posts[0].PostID, posts[1].PostID)
	}
	if posts[0].ContentType != domain.ContentTweet {
		t.Errorf("phải là tweet, nhận %q", posts[0].ContentType)
	}
	if posts[0].URL != "https://x.com/vnexpress/status/9001" {
		t.Errorf("URL sai: %s", posts[0].URL)
	}
	// permalink rỗng thì URL phải tự ghép từ handle + id — một bài không mở
	// được là một bài vô dụng ở mọi bước sau.
	if posts[1].URL != "https://x.com/vnexpress/status/9002" {
		t.Errorf("URL ghép tay sai: %s", posts[1].URL)
	}
	// `text` là chỗ dự phòng khi không có `full_text`.
	if posts[1].Text != "Tin cũ hơn" {
		t.Errorf("text dự phòng sai: %q", posts[1].Text)
	}
	if posts[0].Meta.PostedAt == nil {
		t.Error("created_at đúng định dạng phải phân tích được")
	} else if posts[0].Meta.PostedAt.Year() != 2018 {
		t.Errorf("năm sai: %v", posts[0].Meta.PostedAt)
	}
	// created_at rỗng KHÔNG loại bài: thiếu ngày không có nghĩa là không phải bài.
	if posts[1].Meta.PostedAt != nil {
		t.Errorf("created_at rỗng phải để trống, nhận %v", posts[1].Meta.PostedAt)
	}
	if len(posts[0].Meta.Hashtags) != 1 || posts[0].Meta.Hashtags[0] != "#breaking" {
		t.Errorf("hashtag sai: %v", posts[0].Meta.Hashtags)
	}
}

// TestParseXTimelineOrder là test của một lỗi ĐÃ XẢY RA THẬT.
//
// Mảng `entries` của endpoint syndication không bảo đảm theo thứ tự thời gian.
// Đo trên @TF1Info ngày 17/09/2026: một biến thể phản hồi trả 100 tweet mà 5
// phần tử đầu lần lượt từ 2022, 2020, 2025, 2025, 2022. Bản đầu tiên của
// adapter lấy N phần tử đầu của mảng, nên "50 bài cũ" thành ra 50 bài ngẫu
// nhiên rải suốt 10 năm.
//
// Dữ liệu dưới đây giữ nguyên hình dạng đó, kèm cả `sort_index` giảm dần —
// đúng như thật, và đúng chỗ bẫy: sort_index trông như khoá xếp hạng nhưng giá
// trị của nó là snowflake của THỜI ĐIỂM TRẢ LỜI, giảm 1 đơn vị mỗi phần tử.
// Sắp theo nó là một phép toán không làm gì cả, và test này sẽ fail.
func TestParseXTimelineOrder(t *testing.T) {
	body := `<script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"timeline":{"entries":[
	 {"type":"tweet","sort_index":"2100487973588959232","content":{"tweet":{"id_str":"300","full_text":"giữa",
	   "created_at":"Sun Jan 09 19:51:29 +0000 2022","user":{"screen_name":"a"}}}},
	 {"type":"tweet","sort_index":"2100487973588959231","content":{"tweet":{"id_str":"100","full_text":"cũ nhất",
	   "created_at":"Fri Dec 18 13:29:51 +0000 2020","user":{"screen_name":"a"}}}},
	 {"type":"tweet","sort_index":"2100487973588959230","content":{"tweet":{"id_str":"900","full_text":"mới nhất",
	   "created_at":"Wed May 21 16:54:56 +0000 2025","user":{"screen_name":"a"}}}}
	]}}}}</script>`

	posts, err := parseXTimeline([]byte(body), "a", 10)
	if err != nil {
		t.Fatalf("lỗi ngoài dự kiến: %v", err)
	}
	want := []string{"900", "300", "100"}
	got := []string{posts[0].PostID, posts[1].PostID, posts[2].PostID}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("thứ tự sai: %v, muốn %v (mới nhất trước)", got, want)
		}
	}

	// Điều thật sự quan trọng: cắt limit phải rơi vào bài MỚI NHẤT, không phải
	// phần tử đầu mảng.
	top, err := parseXTimeline([]byte(body), "a", 1)
	if err != nil {
		t.Fatalf("lỗi ngoài dự kiến: %v", err)
	}
	if len(top) != 1 || top[0].PostID != "900" {
		t.Errorf("limit=1 phải lấy bài mới nhất, nhận %+v", top)
	}
}

// TestParseXTimelineOrderBySnowflake: created_at rỗng thì thời điểm đăng giải
// từ chính id — ID của X là snowflake, 41 bit cao là mốc thời gian.
//
// Không có bước dự phòng này thì một tweet thiếu created_at nhận khoá 0 và bị
// đẩy xuống cuối, kể cả khi nó là tweet mới nhất.
func TestParseXTimelineOrderBySnowflake(t *testing.T) {
	// 1339925847520718850 -> 2020-12-18; 1925233863866872185 -> 2025-05-21.
	body := `<script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"timeline":{"entries":[
	 {"type":"tweet","content":{"tweet":{"id_str":"1339925847520718850","full_text":"cũ",
	   "created_at":"","user":{"screen_name":"a"}}}},
	 {"type":"tweet","content":{"tweet":{"id_str":"1925233863866872185","full_text":"mới",
	   "created_at":"","user":{"screen_name":"a"}}}}
	]}}}}</script>`

	posts, err := parseXTimeline([]byte(body), "a", 10)
	if err != nil {
		t.Fatalf("lỗi ngoài dự kiến: %v", err)
	}
	if posts[0].PostID != "1925233863866872185" {
		t.Errorf("thiếu created_at phải xếp theo snowflake của id, nhận %q trước", posts[0].PostID)
	}
}

func TestXSnowflakeMillis(t *testing.T) {
	// Tweet 1480265994903990278 đăng Sun Jan 09 19:51:29 +0000 2022 — đối chiếu
	// bằng chính created_at của nó trong phản hồi thật.
	got := xSnowflakeMillis("1480265994903990278")
	want := time.Date(2022, 1, 9, 19, 51, 29, 0, time.UTC).UnixMilli()
	if diff := got - want; diff < -1000 || diff > 1000 {
		t.Errorf("giải snowflake ra %d, muốn ~%d (lệch %d ms)", got, want, diff)
	}
	if xSnowflakeMillis("khong-phai-so") != 0 {
		t.Error("id không đọc được phải trả 0 để xếp cuối")
	}
}

// TestParseXTimelineNoData: trang không có khối JSON = tài khoản riêng tư/bị
// khoá, hoặc X đổi cấu trúc. Cả hai đều KHÔNG phải lỗi của proxy, nên không
// được gắn nhãn chặn — gắn vào là giết proxy vì một tài khoản đóng.
func TestParseXTimelineNoData(t *testing.T) {
	_, err := parseXTimeline([]byte(`<html><body>Nothing here</body></html>`), "rieng-tu", 10)
	if err == nil {
		t.Fatal("thiếu __NEXT_DATA__ phải báo lỗi")
	}
	if kind, ok := domain.FetchBlockKindOf(err); ok {
		t.Errorf("không được gắn nhãn chặn, nhận: %v", kind)
	}

	// Có khối JSON nhưng 0 tweet — cũng vậy.
	empty := `<script id="__NEXT_DATA__" type="application/json">` +
		`{"props":{"pageProps":{"timeline":{"entries":[]}}}}</script>`
	if _, err := parseXTimeline([]byte(empty), "trong", 10); err == nil {
		t.Fatal("0 tweet phải báo lỗi")
	}
}
