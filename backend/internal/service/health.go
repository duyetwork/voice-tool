package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/strongbody/voice-tool/backend/internal/repository"
)

// Health trả lời một câu hỏi mà giao diện hiện không hỏi được: hệ thống có
// đang hỏng âm thầm không.
//
// Ba thứ hỏng mà KHÔNG có gì hiện ra trên màn hình:
//
//  1. Voice `failed` — nằm trong bảng Voice, nhưng ai không mở bảng đó thì
//     không thấy. Auto-publish chạy trong worker, nên người tạo kênh có thể
//     không mở màn Voice suốt nhiều ngày.
//  2. Token multime của chủ kênh hết hạn — rủi ro 🔴 đã ghi trong docs/status.md
//     §3.1: refresh token chết thì MỌI auto-publish của các kênh người đó tạo
//     đều fail, và không có cảnh báo nào. Chỉ chính họ đăng nhập lại mới sửa
//     được, nên biết sớm là toàn bộ cách xử lý.
//  3. Kênh đang bật mà vòng quét gần nhất lỗi — kênh vẫn hiện "Đang bật", vẫn
//     có tần suất, trông y hệt một kênh khoẻ chưa có bài mới.
//
// Cố tình KHÔNG gửi email/telegram: specs ghi rõ hệ thống không có chức năng
// thông báo. Đây chỉ là con số để giao diện hiện một dấu chấm đỏ.
type Health struct {
	q   *repository.Queries
	log *slog.Logger
}

func NewHealth(q *repository.Queries, log *slog.Logger) *Health {
	return &Health{q: q, log: log}
}

// HealthView là thứ thanh cảnh báo trên giao diện đọc.
type HealthView struct {
	FailedVoices      int64 `json:"failed_voices"`
	UsersNeedRelogin  int64 `json:"users_need_relogin"`
	ChannelsWithError int64 `json:"channels_with_error"`
	// OK = không có gì phải để mắt. Tính ở server để mọi chỗ hiển thị dùng
	// chung một định nghĩa "khoẻ", thay vì mỗi màn tự cộng ba con số một kiểu.
	OK bool `json:"ok"`
}

func (h *Health) View(ctx context.Context) (HealthView, error) {
	row, err := h.q.GetHealthCounters(ctx)
	if err != nil {
		return HealthView{}, fmt.Errorf("đọc chỉ số sức khoẻ: %w", err)
	}
	return HealthView{
		FailedVoices:      row.FailedVoices,
		UsersNeedRelogin:  row.UsersNeedRelogin,
		ChannelsWithError: row.ChannelsWithError,
		OK: row.FailedVoices == 0 &&
			row.UsersNeedRelogin == 0 &&
			row.ChannelsWithError == 0,
	}, nil
}
