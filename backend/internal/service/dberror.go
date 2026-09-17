package service

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// ---------------------------------------------------------------------------
// Dịch lỗi ràng buộc của Postgres thành câu người dùng đọc được
// ---------------------------------------------------------------------------
//
// VÌ SAO CÓ TỆP NÀY: domain.UserMessage, khi không nhận ra lỗi, trả về NGUYÊN
// VĂN lỗi đã cắt ngắn. Với lỗi nghiệp vụ đó là lựa chọn đúng — thà đọc được
// "3voices: hết credit" còn hơn một dòng "lỗi hệ thống" chung chung. Nhưng với
// lỗi ràng buộc của Postgres thì nguyên văn là thứ này:
//
//	ERROR: update or delete on table "prompt" violates foreign key constraint
//	       "source_post_prompt_id_fkey" on table "source_post" (SQLSTATE 23503)
//
// Người vận hành bấm nút Xoá và nhận lại một câu không nói họ phải làm gì. Đã
// có hai lần báo lỗi kiểu này liên tiếp, nên chỗ hỏng không phải một ràng buộc
// cụ thể mà là việc KHÔNG CÓ AI DỊCH cả nhóm lỗi đó.
//
// Chỉ dịch hai mã thật sự tới tay người dùng. Những mã còn lại (kiểu dữ liệu
// sai, hết bộ nhớ...) là lỗi của lập trình viên hoặc của hạ tầng: chúng cần
// nguyên văn trong log, và một câu dịch đẹp chỉ làm chậm việc tìm ra nguyên
// nhân.

const (
	// foreignKeyViolation: đang xoá một bản ghi mà nơi khác còn trỏ tới, hoặc
	// đang trỏ tới một bản ghi không tồn tại.
	foreignKeyViolation = "23503"
	// checkViolation: dòng vi phạm một luật dữ liệu khai ngay trên bảng.
	checkViolation = "23514"
)

// constraintMessages map tên ràng buộc sang câu nói rõ PHẢI LÀM GÌ.
//
// Khoá theo TÊN RÀNG BUỘC chứ không theo bảng: một bảng có nhiều ràng buộc và
// mỗi cái hỏng vì một lý do khác nhau, gộp theo bảng thì câu trả lời lại chung
// chung đúng như thứ đang muốn tránh.
var constraintMessages = map[string]string{
	// Xoá Prompt mẫu còn kênh/bài đang dùng. Ba ràng buộc, cùng một việc phải
	// làm, nhưng nói rõ ai đang giữ để người dùng biết đi đâu gỡ.
	"list_breaking_prompt_id_fkey": "Prompt này đang được một kênh Breaking dùng — " +
		"đổi prompt của kênh đó trước khi xoá.",
	"list_scheduled_prompt_id_fkey": "Prompt này đang được một kênh Định kỳ dùng — " +
		"đổi prompt của kênh đó trước khi xoá.",
	"source_post_prompt_id_fkey": "Prompt này đã được dùng để tạo Bài Post — " +
		"không xoá được chừng nào những bài đó còn.",

	// Quốc gia của kênh: id không có trong danh mục đã đồng bộ.
	"list_breaking_country_id_fkey": "Quốc gia đã chọn không có trong danh mục — " +
		"tải lại trang để lấy danh mục mới rồi chọn lại.",
	"list_scheduled_country_id_fkey": "Quốc gia đã chọn không có trong danh mục — " +
		"tải lại trang để lấy danh mục mới rồi chọn lại.",
	"source_post_country_id_fkey": "Quốc gia đã chọn không có trong danh mục — " +
		"tải lại trang để lấy danh mục mới rồi chọn lại.",

	"ck_source_post_origin":    "Bài Post phải thuộc đúng một loại kênh, hoặc không thuộc kênh nào.",
	"ck_source_post_prompt":    "Hình thức C bắt buộc chọn Prompt mẫu.",
	"ck_list_breaking_prompt":  "Hình thức C bắt buộc chọn Prompt mẫu.",
	"ck_list_scheduled_prompt": "Hình thức C bắt buộc chọn Prompt mẫu.",
}

// constraintMessage đọc một lỗi Postgres thành câu cho người dùng.
//
// Trả ("", false) khi lỗi không phải lỗi ràng buộc, hoặc là ràng buộc chưa có
// câu dịch — lúc đó để nguyên văn đi tiếp, đúng như domain.UserMessage vẫn làm.
// Bịa một câu chung chung cho ràng buộc lạ thì người đọc mất luôn manh mối duy
// nhất là tên của nó.
func constraintMessage(err error) (string, bool) {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return "", false
	}
	if pgErr.Code != foreignKeyViolation && pgErr.Code != checkViolation {
		return "", false
	}
	msg, ok := constraintMessages[pgErr.ConstraintName]
	return msg, ok
}

// wrapDB bọc một lỗi từ DB trước khi trả lên trên.
//
// Ràng buộc có câu dịch thì gắn câu đó bằng domain.Explain — UserMessage ưu
// tiên UserError gần nhất nên người dùng thấy câu dịch, còn nguyên văn vẫn nằm
// trong chuỗi lỗi cho log. Không có câu dịch thì giữ nguyên hành vi cũ.
//
// `what` là việc đang làm ("xoá prompt", "tạo kênh"), đi vào log chứ không lên
// giao diện.
func wrapDB(err error, what string) error {
	if err == nil {
		return nil
	}
	if msg, ok := constraintMessage(err); ok {
		return domain.Explain(msg, fmt.Errorf("%s: %w", what, err))
	}
	return fmt.Errorf("%s: %w", what, err)
}
