package handler

import (
	"encoding/json"
	"testing"
)

// TestChannelCountryOnCreate là test của một lỗi ĐÃ XẢY RA THẬT.
//
// Form Thêm kênh gửi `country_id: 0` khi người dùng không chọn nước — 0 là quy
// ước "không có nước" (id của Strongbody luôn > 0). Hai đường TẠO kênh trước
// đây đưa thẳng giá trị đó xuống INSERT, và Postgres từ chối:
//
//	insert or update on table "list_scheduled" violates foreign key
//	constraint "list_scheduled_country_id_fkey" — Key (country_id)=(0)
//
// Hậu quả: không ai tạo được kênh nếu bỏ trống ô Quốc gia — tức là gần như mọi
// lần thêm kênh. Đường SỬA không dính vì nó đã gọi country() từ đầu, nên lỗi
// chỉ lộ ra ở đúng thao tác đầu tiên người dùng làm.
func TestChannelCountryOnCreate(t *testing.T) {
	cases := []struct {
		name string
		body string
		// want nil = phải ghi NULL xuống DB.
		want *int64
	}{
		{
			name: "không chọn nước — giao diện gửi 0",
			body: `{"country_id":0}`,
			want: nil,
		},
		{
			name: "không có trường nào",
			body: `{}`,
			want: nil,
		},
		{
			name: "số âm cũng coi như không có nước",
			body: `{"country_id":-1}`,
			want: nil,
		},
		{
			name: "chọn một nước",
			body: `{"country_id":7}`,
			want: ptr(int64(7)),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var req channelCountryRequest
			if err := json.Unmarshal([]byte(tc.body), &req); err != nil {
				t.Fatalf("giải mã body: %v", err)
			}
			got := req.countryOnCreate()

			switch {
			case tc.want == nil && got != nil:
				t.Fatalf("phải là NULL, nhận %d — giá trị này sẽ làm INSERT văng lỗi khoá ngoại", *got)
			case tc.want != nil && got == nil:
				t.Fatalf("phải là %d, nhận NULL", *tc.want)
			case tc.want != nil && *got != *tc.want:
				t.Fatalf("nhận %d, muốn %d", *got, *tc.want)
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }
