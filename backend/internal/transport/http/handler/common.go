// Package handler chứa các Gin HTTP handler. Theo business rule #10, handler
// chỉ CRUD + enqueue job — không gọi TTS/STT/LLM/Multime trực tiếp.
package handler

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// pathUUID parse tham số :id trên URL.
func pathUUID(c *gin.Context, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		return uuid.Nil, fmt.Errorf("%w: %s không phải UUID hợp lệ", domain.ErrInvalidInput, name)
	}
	return id, nil
}

// queryUUID parse query param dạng UUID (nil nếu không truyền).
func queryUUID(c *gin.Context, name string) (*uuid.UUID, error) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %s không phải UUID hợp lệ", domain.ErrInvalidInput, name)
	}
	return &id, nil
}

// queryString trả về nil nếu query param không được truyền.
func queryString(c *gin.Context, name string) *string {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return nil
	}
	return &raw
}

func queryBool(c *gin.Context, name string) *bool {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return nil
	}
	return &v
}

// queryTime đọc bộ lọc ngày dạng "2026-01-31" hoặc RFC3339. Ngày trần (to)
// được đẩy tới cuối ngày để "từ 1/1 đến 1/1" vẫn lấy đủ bài trong ngày đó.
func queryTime(c *gin.Context, name string, endOfDay bool) (*time.Time, error) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return &t, nil
	}
	t, err := time.ParseInLocation("2006-01-02", raw, time.Local)
	if err != nil {
		return nil, fmt.Errorf("%w: %s phải là ngày dạng YYYY-MM-DD", domain.ErrInvalidInput, name)
	}
	if endOfDay {
		t = t.Add(24*time.Hour - time.Nanosecond)
	}
	return &t, nil
}

// sorting đọc tham số sắp xếp của bảng: `sort` là cột, `dir` là chiều.
// Service whitelist lại tên cột, handler không cần biết bảng nào có cột gì.
func sorting(c *gin.Context) (sort, dir string) {
	return c.Query("sort"), c.Query("dir")
}

// pagination đọc limit/offset; service sẽ clamp lại giá trị.
func pagination(c *gin.Context) (limit, offset int32) {
	limit = int32(atoiDefault(c.Query("limit"), 20))
	offset = int32(atoiDefault(c.Query("offset"), 0))
	return limit, offset
}

func atoiDefault(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}

// parseDuration nhận cả dạng Go ("30m", "6h") và số giây ("1800").
func parseDuration(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("%w: scan_frequency là bắt buộc", domain.ErrInvalidInput)
	}
	if secs, err := strconv.Atoi(raw); err == nil {
		return time.Duration(secs) * time.Second, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%w: scan_frequency %q không hợp lệ (ví dụ: 30m, 6h, 86400)",
			domain.ErrInvalidInput, raw)
	}
	return d, nil
}

// derefOr trả giá trị con trỏ, rỗng nếu nil — dùng khi map cột nullable của
// sqlc sang response JSON.
func derefOr[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}
