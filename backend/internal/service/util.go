package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/strongbody/voice-tool/backend/internal/domain"
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

// firstLine lấy dòng đầu tiên khác rỗng trong các ứng viên, cắt còn 200 ký tự
// để dùng làm tiêu đề Voice.
func firstLine(candidates ...string) string {
	for _, c := range candidates {
		for line := range strings.SplitSeq(c, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if r := []rune(line); len(r) > 200 {
				return string(r[:200])
			}
			return line
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
