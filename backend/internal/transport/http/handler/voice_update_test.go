package handler

import (
	"encoding/json"
	"testing"
)

// Ba trạng thái của author_country_id phải tách bạch được, vì mỗi cái ứng với
// một thao tác có thật trên giao diện:
//
//	vắng mặt  bảng Voice đổi author/giới tính ngay tại dòng -> GIỮ quốc gia
//	null      modal Sửa chọn "— Tất cả quốc gia —"          -> XOÁ quốc gia
//	có số     modal Sửa chọn một nước                       -> ĐẶT quốc gia
//
// Gộp hai trạng thái đầu (cách cũ: *int64, nil là cả hai) thì nhánh xoá im lặng
// không chạy — người dùng bấm Lưu xong mở lại vẫn thấy nước cũ.
func TestUpdateVoiceRequestCountryTriState(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		wantSet  bool
		wantNil  bool
		wantVal  int64
		hasValue bool
	}{
		{
			name:    "vắng mặt: không đụng tới quốc gia",
			body:    `{"title":"abc"}`,
			wantSet: false,
			wantNil: true,
		},
		{
			name:    "null: xoá quốc gia",
			body:    `{"author_country_id":null}`,
			wantSet: true,
			wantNil: true,
		},
		{
			name:     "có số: đặt quốc gia",
			body:     `{"author_country_id":7}`,
			wantSet:  true,
			wantVal:  7,
			hasValue: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var req updateVoiceRequest
			if err := json.Unmarshal([]byte(c.body), &req); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if req.AuthorCountryID.Set != c.wantSet {
				t.Errorf("Set = %v, muốn %v", req.AuthorCountryID.Set, c.wantSet)
			}
			if c.hasValue {
				if req.AuthorCountryID.Value == nil {
					t.Fatalf("Value = nil, muốn %d", c.wantVal)
				}
				if *req.AuthorCountryID.Value != c.wantVal {
					t.Errorf("Value = %d, muốn %d", *req.AuthorCountryID.Value, c.wantVal)
				}
				return
			}
			if c.wantNil && req.AuthorCountryID.Value != nil {
				t.Errorf("Value = %d, muốn nil", *req.AuthorCountryID.Value)
			}
		})
	}
}

// Các trường còn lại vẫn là *T thuần: chúng không có nhánh "xoá" nào nên vắng
// mặt và null trùng nghĩa, và COALESCE trong SQL đã xử lý đúng.
func TestUpdateVoiceRequestOtherFieldsUnchanged(t *testing.T) {
	var req updateVoiceRequest
	if err := json.Unmarshal([]byte(`{"hashtag":"tinnong vietnam"}`), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.Hashtag == nil || *req.Hashtag != "tinnong vietnam" {
		t.Errorf("Hashtag = %v", req.Hashtag)
	}
	if req.Title != nil {
		t.Errorf("Title phải nil khi không gửi, được %q", *req.Title)
	}
	if req.AuthorCountryID.Set {
		t.Error("không gửi author_country_id thì Set phải là false")
	}
}
