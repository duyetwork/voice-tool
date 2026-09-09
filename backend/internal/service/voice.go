package service

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/repository"
)

// Voice là service tầng 3. CRUD + enqueue publish; việc gọi TTS và multime.ai
// nằm ở Engine (chạy trong worker).
type Voice struct {
	q       *repository.Queries
	storage domain.Storage
	enq     domain.Enqueuer
	audit   *Audit
}

func NewVoice(q *repository.Queries, storage domain.Storage, enq domain.Enqueuer, audit *Audit) *Voice {
	return &Voice{q: q, storage: storage, enq: enq, audit: audit}
}

func (v *Voice) Get(ctx context.Context, id uuid.UUID) (repository.Voice, error) {
	voice, err := v.q.GetVoice(ctx, id)
	if err != nil {
		return repository.Voice{}, wrapNotFound(err, "voice "+id.String())
	}
	return voice, nil
}

// VoiceFilter gom bộ lọc của bảng Voice: trạng thái đăng, nền tảng nguồn,
// ngôn ngữ, người tạo, khoảng ngày tạo và khoảng ngày đăng (prompt.md mục 8).
type VoiceFilter struct {
	PublishStatus *string
	SourcePostID  *uuid.UUID
	Platform      *string
	Language      *string
	CreatedBy     *uuid.UUID
	CreatedFrom   *time.Time
	CreatedTo     *time.Time
	PublishedFrom *time.Time
	PublishedTo   *time.Time
	Limit         int32
	Offset        int32
}

func (v *Voice) List(ctx context.Context, f VoiceFilter) ([]repository.ListVoicesRow, int64, error) {
	limit, offset := clampPage(f.Limit, f.Offset)

	items, err := v.q.ListVoices(ctx, repository.ListVoicesParams{
		PublishStatus: f.PublishStatus,
		SourcePostID:  f.SourcePostID,
		Platform:      f.Platform,
		Language:      f.Language,
		CreatedBy:     f.CreatedBy,
		CreatedFrom:   f.CreatedFrom,
		CreatedTo:     f.CreatedTo,
		PublishedFrom: f.PublishedFrom,
		PublishedTo:   f.PublishedTo,
		Lim:           limit,
		Off:           offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list voice: %w", err)
	}
	total, err := v.q.CountVoices(ctx, repository.CountVoicesParams{
		PublishStatus: f.PublishStatus,
		SourcePostID:  f.SourcePostID,
		Platform:      f.Platform,
		Language:      f.Language,
		CreatedBy:     f.CreatedBy,
		CreatedFrom:   f.CreatedFrom,
		CreatedTo:     f.CreatedTo,
		PublishedFrom: f.PublishedFrom,
		PublishedTo:   f.PublishedTo,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count voice: %w", err)
	}
	return items, total, nil
}

// AudioFile là file voice đọc từ storage để nghe thử / tải về trước khi đăng
// (prompt.md mục 2). File nằm trong bucket riêng tư nên phải đi qua API — trình
// duyệt không truy cập thẳng MinIO/S3 được.
type AudioFile struct {
	Data     []byte
	MimeType string
	FileName string
}

func (v *Voice) Audio(ctx context.Context, id uuid.UUID) (AudioFile, error) {
	voice, err := v.Get(ctx, id)
	if err != nil {
		return AudioFile{}, err
	}
	if voice.VoiceFileUrl == nil {
		// Đã publish -> file bị xoá theo business rule #2, nghe lại trên multime.
		return AudioFile{}, domain.ErrNoVoiceFile
	}

	key := v.storage.KeyFromURL(*voice.VoiceFileUrl)
	data, err := v.storage.Get(ctx, key)
	if err != nil {
		return AudioFile{}, fmt.Errorf("đọc file voice: %w", err)
	}

	mime := deref(voice.MimeType)
	if mime == "" {
		mime = "audio/mpeg"
	}
	return AudioFile{Data: data, MimeType: mime, FileName: audioFileName(voice, key)}, nil
}

// audioFileName đặt tên file tải về theo tiêu đề voice cho dễ tìm lại.
func audioFileName(voice repository.Voice, key string) string {
	base := strings.TrimSpace(deref(voice.Title))
	if base == "" {
		base = "voice-" + voice.ID.String()[:8]
	}
	if r := []rune(base); len(r) > 80 {
		base = string(r[:80])
	}
	base = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`\/:*?"<>|`, r) || r < 32 {
			return '-'
		}
		return r
	}, base)

	ext := path.Ext(key)
	if ext == "" {
		ext = ".mp3"
	}
	return base + ext
}

// UpdateMetadataInput — sửa metadata trước khi đăng (chức năng V1).
type UpdateMetadataInput struct {
	Title       *string
	Description *string
	Hashtag     *string
	Language    *string
	ImageURL    *string
}

func (v *Voice) UpdateMetadata(ctx context.Context, actor, id uuid.UUID, in UpdateMetadataInput) (repository.Voice, error) {
	before, err := v.Get(ctx, id)
	if err != nil {
		return repository.Voice{}, err
	}
	if before.PublishStatus == domain.PublishPublished {
		return repository.Voice{}, domain.ErrAlreadyPublished
	}

	after, err := v.q.UpdateVoiceMetadata(ctx, repository.UpdateVoiceMetadataParams{
		ID:          id,
		Title:       in.Title,
		Description: in.Description,
		Hashtag:     in.Hashtag,
		Language:    in.Language,
		ImageUrl:    in.ImageURL,
	})
	if err != nil {
		return repository.Voice{}, wrapNotFound(err, "voice "+id.String())
	}

	v.audit.Record(ctx, actor, domain.AuditUpdate, domain.ObjectVoice, id, Diff(
		map[string]any{
			"title": before.Title, "description": before.Description,
			"hashtag": before.Hashtag, "language": before.Language,
			"image_url": before.ImageUrl,
		},
		map[string]any{
			"title": after.Title, "description": after.Description,
			"hashtag": after.Hashtag, "language": after.Language,
			"image_url": after.ImageUrl,
		},
	))
	return after, nil
}

// MarkReady chuyển draft -> ready (đã duyệt, chờ đăng).
func (v *Voice) MarkReady(ctx context.Context, actor, id uuid.UUID) (repository.Voice, error) {
	before, err := v.Get(ctx, id)
	if err != nil {
		return repository.Voice{}, err
	}
	if before.PublishStatus == domain.PublishPublished {
		return repository.Voice{}, domain.ErrAlreadyPublished
	}

	after, err := v.q.SetVoicePublishStatus(ctx, repository.SetVoicePublishStatusParams{
		ID:            id,
		PublishStatus: domain.PublishReady,
	})
	if err != nil {
		return repository.Voice{}, wrapNotFound(err, "voice "+id.String())
	}
	v.audit.Record(ctx, actor, domain.AuditUpdate, domain.ObjectVoice, id,
		map[string]any{"publish_status": map[string]any{"before": before.PublishStatus, "after": after.PublishStatus}})
	return after, nil
}

// Delete xoá Voice và dọn luôn file trên storage nếu còn.
func (v *Voice) Delete(ctx context.Context, actor, id uuid.UUID) error {
	voice, err := v.Get(ctx, id)
	if err != nil {
		return err
	}

	if voice.VoiceFileUrl != nil {
		if key := v.storage.KeyFromURL(*voice.VoiceFileUrl); key != "" {
			if err := v.storage.Delete(ctx, key); err != nil {
				return fmt.Errorf("xoá file voice: %w", err)
			}
		}
	}

	rows, err := v.q.DeleteVoice(ctx, id)
	if err != nil {
		return fmt.Errorf("xoá voice: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("%w: voice %s", domain.ErrNotFound, id)
	}
	v.audit.Record(ctx, actor, domain.AuditDelete, domain.ObjectVoice, id, nil)
	return nil
}

// Publish đẩy Voice vào queue voice:publish.
func (v *Voice) Publish(ctx context.Context, actor, id uuid.UUID) error {
	voice, err := v.Get(ctx, id)
	if err != nil {
		return err
	}
	if voice.PublishStatus == domain.PublishPublished {
		return domain.ErrAlreadyPublished
	}
	if voice.VoiceFileUrl == nil {
		return domain.ErrNoVoiceFile
	}

	if err := v.enq.EnqueueVoicePublish(ctx, id.String(), actor.String()); err != nil {
		return err
	}
	v.audit.Record(ctx, actor, domain.AuditPublish, domain.ObjectVoice, id, nil)
	return nil
}
