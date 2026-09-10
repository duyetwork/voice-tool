package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/repository"
)

// ptr trả về con trỏ tới v — dùng cho các cột nullable của sqlc.
func ptr[T any](v T) *T { return &v }

// nilIfEmpty trả về nil cho string rỗng, để không ghi chuỗi rỗng vào cột nullable.
func nilIfEmpty(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}

// nilIfZero: 0 nghĩa là "không đo được" -> lưu NULL thay vì 0.
func nilIfZero[T int32 | int64 | int](v T) *T {
	if v == 0 {
		return nil
	}
	return &v
}

// firstNonEmpty lấy ứng viên đầu tiên có nội dung, giữ nguyên cả đoạn.
func firstNonEmpty(candidates ...string) string {
	for _, c := range candidates {
		if s := strings.TrimSpace(c); s != "" {
			return s
		}
	}
	return ""
}

func deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

// wrapNotFound đổi pgx.ErrNoRows thành domain.ErrNotFound để handler map đúng 404.
func wrapNotFound(err error, what string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %s", domain.ErrNotFound, what)
	}
	return err
}

// clampPage chuẩn hoá limit/offset của các endpoint list.
func clampPage(limit, offset int32) (int32, int32) {
	switch {
	case limit <= 0:
		limit = 20
	case limit > 200:
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// Chiều sắp xếp của các endpoint list.
const (
	SortAsc  = "asc"
	SortDesc = "desc"
)

// normalizeSort chuẩn hoá tham số sắp xếp trước khi đưa vào SQL.
//
// `sort` chỉ được nhận nếu nằm trong whitelist của chính bảng đó — SQL dùng
// giá trị này trong CASE WHEN, và whitelist ở đây là thứ đảm bảo không có cột
// lạ nào lọt vào. Chiều mặc định là desc (mới nhất trước) như cũ.
func normalizeSort(sort, dir string, allowed ...string) (string, string) {
	sort = strings.ToLower(strings.TrimSpace(sort))
	if !slices.Contains(allowed, sort) {
		sort = ""
	}
	if strings.ToLower(strings.TrimSpace(dir)) == SortAsc {
		return sort, SortAsc
	}
	return sort, SortDesc
}

// enqueueVoiceProcess tạo sẵn 1 record Voice ở trạng thái `processing` rồi mới
// đẩy job — nhờ vậy bảng Voice hiện ngay dòng "Đang xử lý" thay vì trống trơn
// cho tới khi worker chạy xong.
//
// Metadata gốc của Bài Post được điền sẵn để dòng đó có nội dung đọc được ngay;
// worker sẽ ghi đè bằng metadata fetch mới (xem query FinishVoice).
// Enqueue lỗi thì xoá record, không để lại dòng treo ở "đang xử lý" mãi.
func enqueueVoiceProcess(
	ctx context.Context,
	q *repository.Queries,
	enq domain.Enqueuer,
	post repository.SourcePost,
	actor uuid.UUID,
) (repository.Voice, error) {
	voice, err := q.CreateVoice(ctx, repository.CreateVoiceParams{
		SourcePostID:  &post.ID,
		Language:      post.Language,
		PublishStatus: domain.PublishProcessing,
		Title:         nilIfEmpty(domain.VoiceTitle(deref(post.Title))),
		Hashtag:       nilIfEmpty(strings.Join(post.Hashtags, " ")),
		ImageUrl:      post.ThumbnailUrl,
		CreatedBy:     actor,
	})
	if err != nil {
		return repository.Voice{}, fmt.Errorf("tạo voice processing: %w", err)
	}

	if err := enq.EnqueueVoiceProcess(ctx, post.ID.String(), actor.String(), voice.ID.String()); err != nil {
		if _, delErr := q.DeleteVoice(ctx, voice.ID); delErr != nil {
			// Không xoá được thì dòng đó treo ở "đang xử lý" — log để còn dọn tay.
			err = errors.Join(err, fmt.Errorf("dọn voice processing %s: %w", voice.ID, delErr))
		}
		return repository.Voice{}, err
	}
	return voice, nil
}
