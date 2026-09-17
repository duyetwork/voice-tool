package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/repository"
)

// ---------------------------------------------------------------------------
// Ghi lịch sử quét (bảng scan_run)
// ---------------------------------------------------------------------------

// ScanTrigger cho biết vòng quét này do đâu mà chạy.
//
// Actor nil = lịch tự chạy. Có giá trị = một người đã bấm "Quét thử", và đó là
// nửa câu trả lời cho "ai quét" mà bảng kênh không lưu được ở đâu khác: kênh
// chỉ có created_by, tức là người THÊM kênh, không phải người cho chạy vòng này.
type ScanTrigger struct {
	Actor *uuid.UUID
}

// ManualScan dựng trigger cho một vòng do người dùng bấm.
func ManualScan(actor uuid.UUID) ScanTrigger { return ScanTrigger{Actor: &actor} }

// ScanTriggerFromActorID đọc trigger ra khỏi payload của task.
//
// Chuỗi rỗng, uuid rỗng, hoặc không parse được đều thành vòng tự động: một
// actor_id hỏng không đáng để bỏ cả vòng quét, và ghi nhầm người vào lịch sử
// còn tệ hơn ghi "hệ thống". uuid.Nil cũng vào đây vì nó không trỏ tới tài
// khoản nào — ghi nó xuống là vi phạm khoá ngoại và mất luôn dòng lịch sử.
func ScanTriggerFromActorID(raw string) ScanTrigger {
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil {
		return ScanTrigger{}
	}
	return ScanTrigger{Actor: &id}
}

func (t ScanTrigger) kind() string {
	if t.Actor != nil {
		return domain.ScanTriggerManual
	}
	return domain.ScanTriggerAuto
}

// scanOwner trỏ vòng quét về đúng một trong hai loại kênh.
type scanOwner struct {
	Breaking  *uuid.UUID
	Scheduled *uuid.UUID
}

func breakingOwner(id uuid.UUID) scanOwner  { return scanOwner{Breaking: &id} }
func scheduledOwner(id uuid.UUID) scanOwner { return scanOwner{Scheduled: &id} }

// startRun mở một dòng lịch sử ở trạng thái `running` — đây cũng chính là thứ
// bảng kênh đọc ra để hiện "Đang quét".
//
// Trả nil khi ghi hỏng, và vòng quét vẫn chạy tiếp: mất một dòng lịch sử là mất
// phần hiển thị, còn bỏ vòng quét là mất bài.
func (s *Scan) startRun(ctx context.Context, owner scanOwner, trig ScanTrigger) *uuid.UUID {
	run, err := s.q.StartScanRun(ctx, repository.StartScanRunParams{
		ListBreakingID:  owner.Breaking,
		ListScheduledID: owner.Scheduled,
		TriggerKind:     trig.kind(),
		TriggeredBy:     trig.Actor,
	})
	if err != nil {
		s.log.WarnContext(ctx, "không mở được lịch sử vòng quét", "error", err,
			"list_breaking_id", owner.Breaking, "list_scheduled_id", owner.Scheduled)
		return nil
	}
	return &run.ID
}

// finishRun đóng dòng lịch sử kèm kết quả.
//
// Dùng context riêng: vòng quét hỏng vì context bị huỷ (worker shutdown, task
// timeout) là đúng lúc dòng lịch sử cần được đóng nhất — chạy bằng context đã
// chết thì mọi vòng hỏng đều nằm lại ở "đang quét", tức là bảng nói ngược với
// sự thật đúng vào lúc có sự cố.
func (s *Scan) finishRun(ctx context.Context, runID *uuid.UUID, res ScanResult, cause error) {
	if runID == nil {
		return
	}
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()

	status := domain.ScanRunSuccess
	var msg *string
	if cause != nil {
		status = domain.ScanRunError
		m := domain.UserMessage(cause)
		msg = &m
	}

	if err := s.q.FinishScanRun(writeCtx, repository.FinishScanRunParams{
		ID:            *runID,
		Status:        status,
		Fetched:       int32(res.Fetched),
		PostsCreated:  int32(res.Created),
		VoicesCreated: int32(res.Voices),
		Skipped:       int32(res.Skipped),
		Error:         msg,
	}); err != nil {
		s.log.WarnContext(ctx, "không đóng được lịch sử vòng quét", "error", err, "run_id", *runID)
	}
}

// ---------------------------------------------------------------------------
// Đọc lịch sử quét (tab "Lịch sử quét" của từng kênh)
// ---------------------------------------------------------------------------

// ScanRunEntry là 1 vòng quét đã chạy, ở dạng giao diện đọc được.
type ScanRunEntry struct {
	ID         uuid.UUID  `json:"id"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	Status     string     `json:"status"`
	// TriggerKind: `auto` = lịch chạy, `manual` = có người bấm.
	TriggerKind string `json:"trigger_kind"`
	// TriggeredByEmail rỗng ở vòng tự động, và cũng rỗng khi tài khoản đã bị
	// xoá — trigger_kind mới là thứ phân biệt hai trường hợp đó.
	TriggeredByEmail string `json:"triggered_by_email"`
	Fetched          int32  `json:"fetched"`
	PostsCreated     int32  `json:"posts_created"`
	VoicesCreated    int32  `json:"voices_created"`
	Skipped          int32  `json:"skipped"`
	Error            string `json:"error"`
}

// ScanHistory là trang lịch sử của 1 kênh kèm tổng kết cửa sổ gần đây.
type ScanHistory struct {
	Items []ScanRunEntry `json:"items"`
	Total int64          `json:"total"`
	// Tổng kết 7 ngày qua — cửa sổ cố định để so được giữa các kênh, giống
	// last_7_days của "Bài bị bỏ qua".
	Runs7d          int64 `json:"runs_7d"`
	PostsCreated7d  int64 `json:"posts_created_7d"`
	VoicesCreated7d int64 `json:"voices_created_7d"`
	Failed7d        int64 `json:"failed_7d"`
	Limit           int32 `json:"limit"`
	Offset          int32 `json:"offset"`
}

// scanHistoryWindow là cửa sổ tổng kết ở đầu tab.
const scanHistoryWindow = 7 * 24 * time.Hour

// ScanRunsBreaking trả lịch sử quét của 1 kênh Breaking.
func (l *List) ScanRunsBreaking(
	ctx context.Context, id uuid.UUID, limit, offset int32,
) (ScanHistory, error) {
	if _, err := l.GetBreaking(ctx, id); err != nil {
		return ScanHistory{}, err
	}
	return l.scanHistory(ctx, breakingOwner(id), limit, offset)
}

// ScanRunsScheduled trả lịch sử quét của 1 kênh Định kỳ.
func (l *List) ScanRunsScheduled(
	ctx context.Context, id uuid.UUID, limit, offset int32,
) (ScanHistory, error) {
	if _, err := l.GetScheduled(ctx, id); err != nil {
		return ScanHistory{}, err
	}
	return l.scanHistory(ctx, scheduledOwner(id), limit, offset)
}

func (l *List) scanHistory(
	ctx context.Context, owner scanOwner, limit, offset int32,
) (ScanHistory, error) {
	lim, off := clampPage(limit, offset)

	rows, err := l.q.ListScanRuns(ctx, repository.ListScanRunsParams{
		ListBreakingID:  owner.Breaking,
		ListScheduledID: owner.Scheduled,
		Lim:             lim,
		Off:             off,
	})
	if err != nil {
		return ScanHistory{}, fmt.Errorf("đọc lịch sử quét: %w", err)
	}
	total, err := l.q.CountScanRuns(ctx, repository.CountScanRunsParams{
		ListBreakingID:  owner.Breaking,
		ListScheduledID: owner.Scheduled,
	})
	if err != nil {
		return ScanHistory{}, fmt.Errorf("đếm lịch sử quét: %w", err)
	}
	sum, err := l.q.SumScanRunsSince(ctx, repository.SumScanRunsSinceParams{
		ListBreakingID:  owner.Breaking,
		ListScheduledID: owner.Scheduled,
		Since:           time.Now().Add(-scanHistoryWindow),
	})
	if err != nil {
		return ScanHistory{}, fmt.Errorf("tổng kết lịch sử quét: %w", err)
	}

	items := make([]ScanRunEntry, 0, len(rows))
	for _, r := range rows {
		items = append(items, ScanRunEntry{
			ID:               r.ID,
			StartedAt:        r.StartedAt,
			FinishedAt:       r.FinishedAt,
			Status:           r.Status,
			TriggerKind:      r.TriggerKind,
			TriggeredByEmail: deref(r.TriggeredByEmail),
			Fetched:          r.Fetched,
			PostsCreated:     r.PostsCreated,
			VoicesCreated:    r.VoicesCreated,
			Skipped:          r.Skipped,
			Error:            deref(r.Error),
		})
	}
	return ScanHistory{
		Items:           items,
		Total:           total,
		Runs7d:          sum.Runs,
		PostsCreated7d:  sum.PostsCreated,
		VoicesCreated7d: sum.VoicesCreated,
		Failed7d:        sum.Failed,
		Limit:           lim,
		Offset:          off,
	}, nil
}
