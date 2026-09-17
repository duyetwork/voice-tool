package service

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// pgErr dựng đúng hình dạng lỗi mà pgx trả về khi Postgres từ chối một lệnh.
func pgErr(code, constraint string) error {
	return &pgconn.PgError{
		Code:           code,
		ConstraintName: constraint,
		Message:        "violates constraint " + constraint,
	}
}

// Người vận hành bấm Xoá và nhận lại nguyên văn lỗi Postgres là chuyện đã xảy
// ra hai lần liên tiếp. Test này khoá lại phần đã sửa: lỗi ràng buộc phải thành
// câu nói rõ PHẢI LÀM GÌ, và nguyên văn vẫn phải còn trong chuỗi lỗi cho log.
func TestWrapDBTranslatesConstraints(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantPrefix string
	}{
		{
			name:       "xoá prompt còn kênh Định kỳ dùng",
			err:        pgErr(foreignKeyViolation, "list_scheduled_prompt_id_fkey"),
			wantPrefix: "Prompt này đang được một kênh Định kỳ dùng",
		},
		{
			name:       "xoá prompt đã tạo ra Bài Post",
			err:        pgErr(foreignKeyViolation, "source_post_prompt_id_fkey"),
			wantPrefix: "Prompt này đã được dùng để tạo Bài Post",
		},
		{
			// Chính lỗi người dùng báo ở lượt trước.
			name:       "country_id không có trong danh mục",
			err:        pgErr(foreignKeyViolation, "list_scheduled_country_id_fkey"),
			wantPrefix: "Quốc gia đã chọn không có trong danh mục",
		},
		{
			name:       "vi phạm CHECK nguồn gốc bài",
			err:        pgErr(checkViolation, "ck_source_post_origin"),
			wantPrefix: "Bài Post phải thuộc đúng một loại kênh",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wrapped := wrapDB(tc.err, "thao tác")
			got := domain.UserMessage(wrapped)
			if len(got) < len(tc.wantPrefix) || got[:len(tc.wantPrefix)] != tc.wantPrefix {
				t.Errorf("UserMessage = %q, muốn bắt đầu bằng %q", got, tc.wantPrefix)
			}
			// Nguyên văn phải còn: log là chỗ duy nhất truy được tên ràng buộc.
			var pg *pgconn.PgError
			if !errors.As(wrapped, &pg) {
				t.Error("mất lỗi gốc — log sẽ không còn tên ràng buộc để tra")
			}
		})
	}
}

// Lỗi KHÔNG phải ràng buộc, và ràng buộc chưa có câu dịch, đều phải giữ nguyên
// hành vi cũ: nguyên văn còn hơn một câu chung chung, vì tên ràng buộc là manh
// mối duy nhất để tra ra chuyện gì đã xảy ra.
func TestWrapDBKeepsUnknownErrors(t *testing.T) {
	plain := errors.New("mạng đứt")
	if got := wrapDB(plain, "xoá prompt"); !errors.Is(got, plain) {
		t.Errorf("lỗi thường phải đi tiếp nguyên vẹn, nhận %v", got)
	}

	unknown := pgErr(foreignKeyViolation, "mot_rang_buoc_chua_dich")
	msg := domain.UserMessage(wrapDB(unknown, "xoá gì đó"))
	if msg == "" {
		t.Error("ràng buộc lạ vẫn phải nói được cái gì đó")
	}
	if _, ok := constraintMessage(unknown); ok {
		t.Error("ràng buộc lạ không được có câu dịch bịa ra")
	}

	// Mã lỗi khác (vd unique) không thuộc phạm vi tệp này.
	if _, ok := constraintMessage(pgErr(uniqueViolation, "uq_source_post_dedup")); ok {
		t.Error("chỉ dịch 23503 và 23514")
	}

	if wrapDB(nil, "không có lỗi") != nil {
		t.Error("nil phải đi ra nil")
	}
}

// constraintMessages chỉ có tác dụng khi TÊN khớp chính xác với tên trong DB.
// Gõ sai một chữ thì câu dịch im lặng không bao giờ chạy — và không có gì báo.
// Danh sách dưới đây đối chiếu với tên thật, lấy bằng:
//
//	SELECT conname FROM pg_constraint;
func TestConstraintNamesMatchSchema(t *testing.T) {
	// Tên thật trong schema, chép từ pg_constraint ngày 17/09/2026.
	real := map[string]bool{
		"list_breaking_prompt_id_fkey":   true,
		"list_scheduled_prompt_id_fkey":  true,
		"source_post_prompt_id_fkey":     true,
		"list_breaking_country_id_fkey":  true,
		"list_scheduled_country_id_fkey": true,
		"source_post_country_id_fkey":    true,
		"ck_source_post_origin":          true,
		"ck_source_post_prompt":          true,
		"ck_list_breaking_prompt":        true,
		"ck_list_scheduled_prompt":       true,
	}
	for name := range constraintMessages {
		if !real[name] {
			t.Errorf("ràng buộc %q không có trong schema — câu dịch sẽ không bao giờ chạy", name)
		}
	}
	for name := range real {
		if _, ok := constraintMessages[name]; !ok {
			t.Errorf("ràng buộc %q có thật nhưng chưa có câu dịch", name)
		}
	}
}
