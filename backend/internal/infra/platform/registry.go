// Package platform chứa các adapter nền tảng nguồn (YouTube, Facebook, ...)
// và registry tự nhận diện nền tảng từ URL (specs 1.3).
package platform

import (
	"fmt"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

type Registry struct {
	adapters []domain.PlatformAdapter
	byName   map[domain.Platform]domain.PlatformAdapter
}

var _ domain.PlatformRegistry = (*Registry)(nil)

func NewRegistry(adapters ...domain.PlatformAdapter) *Registry {
	r := &Registry{
		adapters: adapters,
		byName:   make(map[domain.Platform]domain.PlatformAdapter, len(adapters)),
	}
	for _, a := range adapters {
		r.byName[a.Name()] = a
	}
	return r
}

// Resolve tìm adapter khớp URL. Không đoán mò: không khớp thì báo lỗi rõ ràng
// (business rule specs 1.3).
func (r *Registry) Resolve(url string) (domain.PlatformAdapter, error) {
	for _, a := range r.adapters {
		if a.DetectPlatform(url) {
			return a, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", domain.ErrUnsupportedURL, url)
}

func (r *Registry) Get(p domain.Platform) (domain.PlatformAdapter, error) {
	a, ok := r.byName[p]
	if !ok {
		return nil, fmt.Errorf("%w: nền tảng %s chưa được tích hợp", domain.ErrUnsupportedURL, p)
	}
	return a, nil
}

// Supported liệt kê nền tảng đang bật — dùng cho endpoint metadata của FE.
func (r *Registry) Supported() []domain.Platform {
	out := make([]domain.Platform, 0, len(r.adapters))
	for _, a := range r.adapters {
		out = append(out, a.Name())
	}
	return out
}

// ChannelScanSupport trả lời cho TỪNG nền tảng: có quét được cả kênh không, và
// nếu không thì vì sao — form Thêm kênh đọc đây để nói trước thay vì để người
// dùng dán URL rồi ăn lỗi.
func (r *Registry) ChannelScanSupport() []domain.ChannelScan {
	out := make([]domain.ChannelScan, 0, len(r.adapters))
	for _, a := range r.adapters {
		item := domain.ChannelScan{Platform: a.Name(), Enabled: true}
		if err := a.CheckChannelScan(); err != nil {
			item.Enabled = false
			item.Reason = domain.UserMessage(err)
		}
		out = append(out, item)
	}
	return out
}
