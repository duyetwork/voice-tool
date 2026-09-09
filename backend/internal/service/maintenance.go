package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/strongbody/voice-tool/backend/internal/repository"
)

// Maintenance chứa các job dọn dẹp định kỳ.
type Maintenance struct {
	q   *repository.Queries
	log *slog.Logger
	// skippedLogRetention: giữ skipped_log bao lâu.
	skippedLogRetention time.Duration
}

func NewMaintenance(q *repository.Queries, log *slog.Logger, skippedLogRetention time.Duration) *Maintenance {
	if skippedLogRetention <= 0 {
		skippedLogRetention = 7 * 24 * time.Hour
	}
	return &Maintenance{q: q, log: log, skippedLogRetention: skippedLogRetention}
}

// CleanupSkippedLogs xoá bản ghi "bài bị bỏ qua" quá hạn.
//
// Vì sao cần skipped_log: Breaking quét liên tục và chỉ lấy bài khớp regex —
// khi kênh có bài mà hệ thống không bắt, câu hỏi đầu tiên luôn là "regex sai
// hay nền tảng không trả về bài đó?". skipped_log trả lời được câu hỏi đó, và
// tỉ lệ bỏ qua cho biết pattern đang quá chặt hay quá lỏng.
//
// Vì sao phải dọn: mỗi vòng quét ghi tối đa scan_limit bản ghi cho MỖI kênh.
// Với 20 kênh, scan_limit 20, chu kỳ 60s -> tối đa ~576k bản ghi/ngày. Đây là
// dữ liệu debug, không phải audit trail (audit_log mới là append-only vĩnh
// viễn), nên giữ 7 ngày là đủ để truy vết một sự cố vừa xảy ra.
func (m *Maintenance) CleanupSkippedLogs(ctx context.Context) (int64, error) {
	before := time.Now().Add(-m.skippedLogRetention)

	rows, err := m.q.DeleteSkippedLogsBefore(ctx, before)
	if err != nil {
		return 0, fmt.Errorf("dọn skipped_log: %w", err)
	}
	if rows > 0 {
		m.log.InfoContext(ctx, "đã dọn skipped_log quá hạn",
			"deleted", rows, "retention", m.skippedLogRetention.String())
	}
	return rows, nil
}
