package service

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"

	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/repository"
)

// Audit ghi Nhật ký thao tác. Append-only: không có API sửa/xoá (specs 1.6).
//
// Business rule #8: mọi create/update/delete/run/publish trên list_breaking,
// list_scheduled, source_post, voice đều đi qua đây — service không tự viết log
// rời rạc, middleware/audit wrapper gọi hàm này.
type Audit struct {
	q   *repository.Queries
	log *slog.Logger
}

func NewAudit(q *repository.Queries, log *slog.Logger) *Audit {
	return &Audit{q: q, log: log}
}

// Record ghi 1 bản ghi audit. Lỗi ghi log KHÔNG làm fail nghiệp vụ chính —
// chỉ log lại để cảnh báo.
func (a *Audit) Record(
	ctx context.Context,
	userID uuid.UUID,
	action domain.AuditAction,
	objectType string,
	objectID uuid.UUID,
	changes any,
) {
	var raw []byte
	if changes != nil {
		b, err := json.Marshal(changes)
		if err != nil {
			a.log.WarnContext(ctx, "audit: marshal changes thất bại", "error", err)
		} else {
			raw = b
		}
	}

	if _, err := a.q.CreateAuditLog(ctx, repository.CreateAuditLogParams{
		UserID:     userID,
		Action:     string(action),
		ObjectType: objectType,
		ObjectID:   objectID,
		Changes:    raw,
	}); err != nil {
		a.log.ErrorContext(ctx, "audit: ghi log thất bại",
			"error", err, "action", action, "object_type", objectType, "object_id", objectID)
	}
}

// Diff sinh changes dạng {field: {before, after}} cho action update.
func Diff(before, after map[string]any) map[string]any {
	out := make(map[string]any)
	for k, av := range after {
		bv, existed := before[k]
		if !existed || !equalJSON(bv, av) {
			out[k] = map[string]any{"before": bv, "after": av}
		}
	}
	return out
}

func equalJSON(a, b any) bool {
	ab, errA := json.Marshal(a)
	bb, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return string(ab) == string(bb)
}

// List trả về nhật ký theo filter (endpoint GET /audit-log).
type AuditFilter struct {
	ObjectType *string
	ObjectID   *uuid.UUID
	UserID     *uuid.UUID
	Limit      int32
	Offset     int32
}

func (a *Audit) List(ctx context.Context, f AuditFilter) ([]repository.AuditLog, error) {
	return a.q.ListAuditLogs(ctx, repository.ListAuditLogsParams{
		ObjectType: f.ObjectType,
		ObjectID:   f.ObjectID,
		UserID:     f.UserID,
		Lim:        f.Limit,
		Off:        f.Offset,
	})
}
