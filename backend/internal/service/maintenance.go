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
	// aiUsageRetention: giữ ai_usage bao lâu.
	aiUsageRetention time.Duration
	// scanRunRetention: giữ lịch sử quét bao lâu.
	scanRunRetention time.Duration
	// viaUsageLogRetention: giữ nhật ký dùng via bao lâu.
	viaUsageLogRetention time.Duration
}

// staleScanRunAfter — sau bấy lâu, một vòng quét còn ở `running` được coi là đã
// chết cùng worker của nó.
//
// 2 giờ, trong khi task chỉ có timeout 30 phút và tối đa 3 lần retry: ngưỡng
// rộng để không bao giờ đóng nhầm một vòng đang chạy thật. Đóng nhầm tệ hơn
// đóng muộn — nó ghi "lỗi" vào lịch sử cho một vòng rồi sẽ chạy xong bình
// thường.
const staleScanRunAfter = 2 * time.Hour

func NewMaintenance(
	q *repository.Queries,
	log *slog.Logger,
	skippedLogRetention, aiUsageRetention, scanRunRetention,
	viaUsageLogRetention time.Duration,
) *Maintenance {
	if skippedLogRetention <= 0 {
		skippedLogRetention = 7 * 24 * time.Hour
	}
	if aiUsageRetention <= 0 {
		aiUsageRetention = 90 * 24 * time.Hour
	}
	if scanRunRetention <= 0 {
		scanRunRetention = 30 * 24 * time.Hour
	}
	if viaUsageLogRetention <= 0 {
		viaUsageLogRetention = 30 * 24 * time.Hour
	}
	return &Maintenance{
		q: q, log: log,
		skippedLogRetention:  skippedLogRetention,
		aiUsageRetention:     aiUsageRetention,
		scanRunRetention:     scanRunRetention,
		viaUsageLogRetention: viaUsageLogRetention,
	}
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

// CleanupAIUsage xoá bản ghi lượng dùng AI quá hạn.
//
// Giữ lâu hơn skipped_log (90 ngày thay vì 7) vì đây là dữ liệu để nhìn XU
// HƯỚNG chi phí — một cửa sổ 7 ngày không cho biết tháng này đắt hơn tháng
// trước hay không. Vẫn phải xoá: mỗi voice mode C sinh ít nhất 2 dòng (LLM +
// TTS), và bảng này chỉ dùng để nhìn, không phải sổ sách kế toán.
func (m *Maintenance) CleanupAIUsage(ctx context.Context) (int64, error) {
	before := time.Now().Add(-m.aiUsageRetention)

	rows, err := m.q.DeleteAIUsageBefore(ctx, before)
	if err != nil {
		return 0, fmt.Errorf("dọn ai_usage: %w", err)
	}
	if rows > 0 {
		m.log.InfoContext(ctx, "đã dọn ai_usage quá hạn",
			"deleted", rows, "retention", m.aiUsageRetention.String())
	}
	return rows, nil
}

// CleanupScanRuns đóng những vòng quét treo rồi xoá lịch sử quá hạn.
//
// Hai việc trong một hàm vì chúng phải chạy theo đúng thứ tự đó: xoá trước rồi
// mới đóng thì một vòng treo vừa quá hạn sẽ bị xoá khi vẫn đang mang trạng thái
// `running`, và bản ghi cuối cùng của nó — thứ duy nhất nói ra rằng worker đã
// chết ở đấy — biến mất trước khi ai kịp nhìn.
//
// Vì sao phải đóng vòng treo: worker bị kill giữa vòng quét thì không ai gọi
// FinishScanRun. Trên bảng kênh dòng đó chỉ sai tới vòng kế tiếp (bảng luôn đọc
// vòng mới nhất), nhưng trong tab lịch sử thì nó "đang quét" mãi mãi.
func (m *Maintenance) CleanupScanRuns(ctx context.Context) (int64, error) {
	stale, err := m.q.FailStaleScanRuns(ctx, time.Now().Add(-staleScanRunAfter))
	if err != nil {
		return 0, fmt.Errorf("đóng vòng quét treo: %w", err)
	}
	if stale > 0 {
		m.log.WarnContext(ctx, "đã đóng vòng quét treo (worker dừng giữa chừng)",
			"count", stale, "ngưỡng", staleScanRunAfter.String())
	}

	rows, err := m.q.DeleteScanRunsBefore(ctx, time.Now().Add(-m.scanRunRetention))
	if err != nil {
		return stale, fmt.Errorf("dọn scan_run: %w", err)
	}
	if rows > 0 {
		m.log.InfoContext(ctx, "đã dọn lịch sử quét quá hạn",
			"deleted", rows, "retention", m.scanRunRetention.String())
	}
	return rows, nil
}

// SweepScrapeInfra là job bảo trì của hạ tầng via/proxy.
//
// Ba việc, chạy theo thứ tự này:
//
//  1. Hồi sinh via đã hết cooldown — via nghỉ đủ giờ mà không ai cho thử lại
//     thì cooldown không khác gì cái chết.
//  2. Đặt lại bộ đếm hạn mức ngày. Bộ đếm đã tự liền theo ngày ở
//     MarkScrapeViaUsed, nên đây chỉ để bảng trên giao diện nhìn đúng trước
//     lượt dùng đầu tiên của ngày mới.
//  3. Dọn nhật ký dùng via quá hạn.
//
// Không dừng ở việc đầu tiên hỏng: ba việc độc lập nhau, và bỏ qua việc hồi
// sinh via chỉ vì việc dọn log hỏng là đánh đổi sai hướng.
func (m *Maintenance) SweepScrapeInfra(ctx context.Context) error {
	var firstErr error
	note := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	revived, err := m.q.ReviveScrapeVias(ctx)
	note(err)
	if err != nil {
		m.log.ErrorContext(ctx, "không hồi sinh được via hết cooldown", "error", err)
	} else if revived > 0 {
		m.log.InfoContext(ctx, "via hết cooldown, cho thử lại", "count", revived)
	}

	if _, err := m.q.ResetScrapeViaDailyUsed(ctx); err != nil {
		note(err)
		m.log.ErrorContext(ctx, "không đặt lại được hạn mức ngày của via", "error", err)
	}
	if _, err := m.q.ResetScrapeProxyDaily(ctx); err != nil {
		note(err)
		m.log.ErrorContext(ctx, "không đặt lại được bộ đếm ngày của proxy", "error", err)
	}

	rows, err := m.q.DeleteViaUsageLogsBefore(ctx, time.Now().Add(-m.viaUsageLogRetention))
	if err != nil {
		note(err)
		m.log.ErrorContext(ctx, "không dọn được nhật ký dùng via", "error", err)
	} else if rows > 0 {
		m.log.InfoContext(ctx, "đã dọn nhật ký dùng via quá hạn",
			"deleted", rows, "retention", m.viaUsageLogRetention.String())
	}
	return firstErr
}
