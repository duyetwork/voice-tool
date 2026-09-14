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
	// defaultLanguage + modes chỉ dùng cho Voice gõ tay (CreateFromText):
	// luồng đó không đi qua Bài Post nên phải tự áp 2 cấu hình này.
	defaultLanguage string
	modes           ModeGate
}

func NewVoice(
	q *repository.Queries,
	storage domain.Storage,
	enq domain.Enqueuer,
	audit *Audit,
	defaultLanguage string,
	modes ModeGate,
) *Voice {
	return &Voice{
		q: q, storage: storage, enq: enq, audit: audit,
		defaultLanguage: defaultLanguage, modes: modes,
	}
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
	// Sort: cột thời gian để sắp xếp — `created_at` (mặc định) hoặc
	// `published_at`. Dir: `asc` | `desc` (mặc định).
	Sort string
	Dir  string
}

func (v *Voice) List(ctx context.Context, f VoiceFilter) ([]repository.ListVoicesRow, int64, error) {
	limit, offset := clampPage(f.Limit, f.Offset)
	sort, dir := normalizeSort(f.Sort, f.Dir, "created_at", "published_at")

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
		Sort:          sort,
		Dir:           dir,
		MinDuration:   domain.MinPublishDurationSeconds,
		Lim:           limit,
		Off:           offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("list voice: %w", err)
	}
	total, err := v.q.CountVoices(ctx, repository.CountVoicesParams{
		MinDuration:   domain.MinPublishDurationSeconds,
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

// ImageFile là ảnh bìa đọc từ storage để hiển thị trên UI.
//
// Phải đi qua API vì bucket là riêng tư và host `minio:9000` chỉ tồn tại trong
// mạng Docker — thẻ <img> của trình duyệt không với tới URL storage được.
type ImageFile struct {
	Data     []byte
	MimeType string
}

// CoverImage trả ảnh bìa người dùng đã tải lên. Ảnh lấy từ URL bài gốc không đi
// qua đây: nó là link công khai của nền tảng khác, trình duyệt tự tải được.
func (v *Voice) CoverImage(ctx context.Context, id uuid.UUID) (ImageFile, error) {
	voice, err := v.Get(ctx, id)
	if err != nil {
		return ImageFile{}, err
	}
	if !voice.ImageUploaded || voice.ImageUrl == nil {
		return ImageFile{}, fmt.Errorf("%w: voice %s không có ảnh bìa tải lên",
			domain.ErrNotFound, id)
	}

	key := v.storage.KeyFromURL(*voice.ImageUrl)
	data, err := v.storage.Get(ctx, key)
	if err != nil {
		return ImageFile{}, fmt.Errorf("đọc ảnh bìa: %w", err)
	}
	return ImageFile{Data: data, MimeType: mimeByExtension(key)}, nil
}

// mimeByExtension đoán kiểu ảnh từ đuôi key đã lưu — key do chính SetImage đặt
// theo mime người dùng tải lên nên đuôi luôn đúng.
func mimeByExtension(key string) string {
	switch strings.ToLower(path.Ext(key)) {
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	default:
		return "image/jpeg"
	}
}

// errVoiceProcessing: voice chưa xử lý xong thì chưa sửa/đăng được — record
// tồn tại chỉ để người dùng thấy tiến trình.
var errVoiceProcessing = fmt.Errorf(
	"%w: voice đang được xử lý, chờ xong rồi thao tác", domain.ErrInvalidInput)

// UpdateMetadataInput — sửa metadata trước khi đăng (chức năng V1).
type UpdateMetadataInput struct {
	Title    *string
	Hashtag  *string
	Language *string
	ImageURL *string
	// AuthorCountryID: quốc gia đã lọc lúc chọn author. Lưu vì việc bốc tài
	// khoản diễn ra ở bước publish, lúc đó không còn form nào để hỏi lại.
	AuthorCountryID *int64
	// AuthorID/AuthorEmail/AuthorGender: tài khoản Strongbody đứng tên bài đăng.
	// Đi thành bộ — tác giả được bốc ngẫu nhiên theo giới tính, nên id, email
	// hiển thị và giới tính đã bốc luôn thuộc về cùng một người.
	AuthorID     *int64
	AuthorEmail  *string
	AuthorGender *string
}

func (v *Voice) UpdateMetadata(ctx context.Context, actor, id uuid.UUID, in UpdateMetadataInput) (repository.Voice, error) {
	before, err := v.Get(ctx, id)
	if err != nil {
		return repository.Voice{}, err
	}
	if before.PublishStatus == domain.PublishPublished {
		return repository.Voice{}, domain.ErrAlreadyPublished
	}
	// Worker sẽ ghi metadata fetch được vào record này khi xong, nên sửa lúc
	// đang xử lý là mất công vô ích.
	if before.PublishStatus == domain.PublishProcessing {
		return repository.Voice{}, errVoiceProcessing
	}
	// Tiêu đề là phần chữ DUY NHẤT multime nhận (không có trường mô tả) -> chặn
	// ngay ở đây thay vì để job publish fail sau.
	if in.Title != nil && len([]rune(strings.TrimSpace(*in.Title))) > domain.MaxVoiceTitleRunes {
		return repository.Voice{}, fmt.Errorf("%w: tiêu đề tối đa %d ký tự",
			domain.ErrInvalidInput, domain.MaxVoiceTitleRunes)
	}

	if in.AuthorID != nil && *in.AuthorID <= 0 {
		return repository.Voice{}, fmt.Errorf("%w: author_id phải là id tài khoản Strongbody",
			domain.ErrInvalidInput)
	}

	// Thay ảnh đã tải lên bằng một URL khác: file cũ trong bucket không còn ai
	// trỏ tới nữa. Gỡ cờ image_uploaded trước (không thì lúc đăng hệ thống đi
	// xoá nhầm ảnh của URL mới) rồi mới dọn file.
	if in.ImageURL != nil && before.ImageUploaded && *in.ImageURL != deref(before.ImageUrl) {
		if _, err := v.q.SetVoiceImage(ctx, repository.SetVoiceImageParams{
			ID: id, ImageUrl: in.ImageURL, ImageUploaded: false,
		}); err != nil {
			return repository.Voice{}, wrapNotFound(err, "voice "+id.String())
		}
		v.deleteUploadedImage(ctx, before)
	}

	// Đổi giới tính hoặc quốc gia mà không đưa kèm tài khoản cụ thể thì tài
	// khoản đã bốc trước đó không còn khớp — bỏ nó đi để bước đăng bốc lại.
	genderChanged := in.AuthorGender != nil && *in.AuthorGender != deref(before.AuthorGender)
	countryChanged := in.AuthorCountryID != nil &&
		deref(in.AuthorCountryID) != deref(before.AuthorCountryID)
	resetAuthor := in.AuthorID == nil && (genderChanged || countryChanged)

	after, err := v.q.UpdateVoiceMetadata(ctx, repository.UpdateVoiceMetadataParams{
		ID:              id,
		Title:           in.Title,
		Hashtag:         in.Hashtag,
		Language:        in.Language,
		ImageUrl:        in.ImageURL,
		AuthorID:        in.AuthorID,
		AuthorEmail:     in.AuthorEmail,
		AuthorGender:    in.AuthorGender,
		SetCountry:      in.AuthorCountryID != nil,
		AuthorCountryID: in.AuthorCountryID,
		ResetAuthor:     resetAuthor,
	})
	if err != nil {
		return repository.Voice{}, wrapNotFound(err, "voice "+id.String())
	}

	v.audit.Record(ctx, actor, domain.AuditUpdate, domain.ObjectVoice, id, Diff(
		map[string]any{
			"title":   before.Title,
			"hashtag": before.Hashtag, "language": before.Language,
			"image_url": before.ImageUrl, "author_email": before.AuthorEmail,
		},
		map[string]any{
			"title":   after.Title,
			"hashtag": after.Hashtag, "language": after.Language,
			"image_url": after.ImageUrl, "author_email": after.AuthorEmail,
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
	if before.PublishStatus == domain.PublishProcessing {
		return repository.Voice{}, errVoiceProcessing
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

// TextVoiceInput là input tạo Voice thẳng từ text người dùng gõ.
type TextVoiceInput struct {
	Text        string
	CollectMode domain.CollectMode // B hoặc C
	PromptID    *uuid.UUID         // bắt buộc với C
	Language    string             // rỗng = mặc định hệ thống
	// LLMAPISetID: Bộ API key viết lại nội dung. Chỉ có nghĩa với hình thức C;
	// rỗng thì rơi về provider trong .env (dev).
	LLMAPISetID *uuid.UUID
}

// CreateFromText tạo Voice thẳng từ đoạn text, KHÔNG qua Bài Post.
//
// Đây là ngoại lệ có chủ đích của luật "mọi Voice đều đi qua Bài Post"
// (specs -1): luật đó tồn tại để Voice lấy từ một URL luôn truy vết được về
// bài gốc và chạy lại được mà không phải fetch lần nữa. Text gõ tay không có
// URL, không có bài gốc, không có gì để fetch lại — Bài Post sinh ra chỉ là
// bản ghi rỗng làm bẩn màn duyệt Bài Post. Xem migration 000011.
//
// Voice tạo ra ở trạng thái `processing`; worker (voice:text) đọc text đã lưu
// trên chính record đó rồi điền file + metadata vào.
func (v *Voice) CreateFromText(
	ctx context.Context,
	actor uuid.UUID,
	in TextVoiceInput,
) (repository.Voice, error) {
	text := domain.NormalizeTTSText(in.Text)
	if text == "" {
		return repository.Voice{}, fmt.Errorf("%w: phải có nội dung text để đọc", domain.ErrInvalidInput)
	}
	if n := len([]rune(text)); n > domain.MaxTTSTextRunes {
		return repository.Voice{}, fmt.Errorf("%w: text dài %d ký tự, tối đa %d",
			domain.ErrInvalidInput, n, domain.MaxTTSTextRunes)
	}
	// Mode A tách audio từ video gốc — gõ text thì không có gì để tách.
	if !in.CollectMode.NeedsText() {
		return repository.Voice{}, fmt.Errorf(
			"%w: nhập bằng text chỉ dùng được hình thức B hoặc C", domain.ErrInvalidInput)
	}
	if err := v.modes.Check(in.CollectMode); err != nil {
		return repository.Voice{}, err
	}
	if in.CollectMode.NeedsPrompt() && in.PromptID == nil {
		return repository.Voice{}, domain.ErrPromptRequired
	}
	if in.PromptID != nil {
		if _, err := v.q.GetPrompt(ctx, *in.PromptID); err != nil {
			return repository.Voice{}, wrapNotFound(err, "prompt "+in.PromptID.String())
		}
	}

	// Tiêu đề tạm lấy từ chính đoạn text (trừ hashtag) để dòng voice đang chạy
	// đã đọc được ngay; worker ghi đè bằng nội dung thật sự được đọc.
	meta := domain.TextPostMetadata(text)
	voice, err := v.q.CreateTextVoice(ctx, repository.CreateTextVoiceParams{
		InputText:   &text,
		CollectMode: ptr(string(in.CollectMode)),
		PromptID:    in.PromptID,
		Language:    resolveLanguage(in.Language, "", v.defaultLanguage),
		Title:       nilIfEmpty(domain.VoiceTitle(meta.Title)),
		CreatedBy:   actor,
		LlmApiSetID: in.LLMAPISetID,
	})
	if err != nil {
		return repository.Voice{}, fmt.Errorf("tạo voice từ text: %w", err)
	}

	// Enqueue lỗi thì xoá record: thà không có dòng nào còn hơn để lại một
	// voice treo ở "đang xử lý" mà không worker nào nhận.
	if err := v.enq.EnqueueVoiceText(ctx, voice.ID.String(), actor.String()); err != nil {
		if _, derr := v.q.DeleteVoice(ctx, voice.ID); derr != nil {
			return repository.Voice{}, fmt.Errorf("enqueue voice:text: %w (dọn record lỗi: %v)", err, derr)
		}
		return repository.Voice{}, fmt.Errorf("enqueue voice:text: %w", err)
	}

	v.audit.Record(ctx, actor, domain.AuditCreate, domain.ObjectVoice, voice.ID, map[string]any{
		"collect_mode": voice.CollectMode,
		"language":     voice.Language,
		"source":       "text",
	})
	return voice, nil
}

// RegenerateInput — sửa lời đọc rồi tạo lại chính Voice đó.
type RegenerateInput struct {
	// Text là lời đọc người dùng đã chốt: với Voice gõ tay là đoạn họ nhập,
	// với Voice từ Bài Post là nội dung/tiêu đề bài đã sửa lại cho dễ nghe.
	Text        string
	CollectMode domain.CollectMode // B hoặc C
	PromptID    *uuid.UUID         // bắt buộc với C
	Language    string             // rỗng = giữ ngôn ngữ đang có
	// LLMAPISetID rỗng = giữ bộ API voice đang dùng.
	LLMAPISetID *uuid.UUID
}

// Regenerate đọc lại Voice bằng nội dung mới, GHI ĐÈ lên chính bản ghi cũ.
//
// Ghi đè chứ không tạo voice mới vì đây là "sửa lại cho đúng" chứ không phải
// "làm thêm một bản": người dùng đã điền tiêu đề/hashtag/ảnh bìa cho voice này,
// tạo bản mới là bắt họ điền lại từ đầu và để lại một dòng rác trong bảng.
// File audio cũ bị xoá khỏi storage sau khi file mới ghi xong (xem
// Engine.buildTextVoice).
//
// Voice đã đăng lên multime thì không sửa được: bài bên đó đã có người nghe,
// đổi file dưới chân họ là sai — xoá rồi đăng lại nếu thật sự cần.
func (v *Voice) Regenerate(
	ctx context.Context,
	actor, id uuid.UUID,
	in RegenerateInput,
) (repository.Voice, error) {
	before, err := v.Get(ctx, id)
	if err != nil {
		return repository.Voice{}, err
	}
	if before.PublishStatus == domain.PublishPublished {
		return repository.Voice{}, domain.ErrAlreadyPublished
	}
	if before.PublishStatus == domain.PublishProcessing {
		return repository.Voice{}, errVoiceProcessing
	}

	text := domain.NormalizeTTSText(in.Text)
	if text == "" {
		return repository.Voice{}, fmt.Errorf(
			"%w: phải có nội dung để đọc lại", domain.ErrInvalidInput)
	}
	if n := len([]rune(text)); n > domain.MaxTTSTextRunes {
		return repository.Voice{}, fmt.Errorf("%w: text dài %d ký tự, tối đa %d",
			domain.ErrInvalidInput, n, domain.MaxTTSTextRunes)
	}
	// Mode A tách audio từ video gốc — đọc lại theo text thì không tách gì cả.
	if !in.CollectMode.NeedsText() {
		return repository.Voice{}, fmt.Errorf(
			"%w: tạo lại voice từ nội dung chỉ dùng được hình thức B hoặc C",
			domain.ErrInvalidInput)
	}
	if err := v.modes.Check(in.CollectMode); err != nil {
		return repository.Voice{}, err
	}
	if in.CollectMode.NeedsPrompt() && in.PromptID == nil {
		return repository.Voice{}, domain.ErrPromptRequired
	}
	if in.PromptID != nil {
		if _, err := v.q.GetPrompt(ctx, *in.PromptID); err != nil {
			return repository.Voice{}, wrapNotFound(err, "prompt "+in.PromptID.String())
		}
	}

	after, err := v.q.SetVoiceContent(ctx, repository.SetVoiceContentParams{
		ID:          id,
		InputText:   &text,
		CollectMode: ptr(string(in.CollectMode)),
		PromptID:    in.PromptID,
		Language:    nilIfEmpty(strings.ToLower(strings.TrimSpace(in.Language))),
		LlmApiSetID: in.LLMAPISetID,
	})
	if err != nil {
		return repository.Voice{}, wrapNotFound(err, "voice "+id.String())
	}

	if err := v.enq.EnqueueVoiceText(ctx, id.String(), actor.String()); err != nil {
		// Trả record về trạng thái cũ, không để treo ở "đang xử lý" mà không
		// worker nào nhận.
		msg := "không đưa được vào hàng đợi đọc lại"
		if _, serr := v.q.SetVoicePublishStatus(ctx, repository.SetVoicePublishStatusParams{
			ID: id, PublishStatus: before.PublishStatus, LastError: &msg,
		}); serr != nil {
			return repository.Voice{}, fmt.Errorf("enqueue voice:text: %w (khôi phục trạng thái: %v)", err, serr)
		}
		return repository.Voice{}, fmt.Errorf("enqueue voice:text: %w", err)
	}

	v.audit.Record(ctx, actor, domain.AuditUpdate, domain.ObjectVoice, id, map[string]any{
		"action":       "regenerate",
		"collect_mode": deref(after.CollectMode),
		"language":     after.Language,
	})
	return after, nil
}

// publishable chặn ngay ở API các điều kiện multime sẽ từ chối, thay vì để job
// chạy tới nơi rồi fail: người dùng thấy lý do lúc bấm nút, không phải đi tìm
// trong cột trạng thái vài giây sau.
// Author: chỉ cần biết ĐANG NHẮM giới tính nào là đủ để đăng — việc bốc ra một
// tài khoản cụ thể lùi xuống bước publish trong worker. Chọn author ở form giờ
// chỉ là chọn author, không kéo theo một lần gọi sang Strongbody.
func publishable(voice repository.Voice) error {
	hasAuthor := voice.AuthorID != nil && *voice.AuthorID > 0
	hasGender := domain.Gender(deref(voice.AuthorGender)).Valid()
	if !hasAuthor && !hasGender {
		return fmt.Errorf("%w: chưa chọn giới tính tài khoản đứng tên bài đăng (author)",
			domain.ErrInvalidInput)
	}
	// Hashtag không còn giá trị mặc định trong cấu hình — bỏ trống là không đăng.
	if strings.TrimSpace(deref(voice.Hashtag)) == "" {
		return fmt.Errorf("%w: voice chưa có hashtag — multime yêu cầu ít nhất 1 hashtag",
			domain.ErrInvalidInput)
	}
	return nil
}

// maxVoiceImageBytes: ảnh bìa tải từ máy. multime tự resize nên không cần ảnh
// lớn hơn; trần này chặn người dùng đẩy nguyên ảnh máy ảnh lên bucket.
const maxVoiceImageBytes = 8 << 20

// voiceImageTypes là các định dạng ảnh bìa nhận vào — đúng những gì multime
// hiển thị được.
var voiceImageTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// ImageInput là ảnh bìa người dùng tải lên từ máy.
type ImageInput struct {
	Data     []byte
	MimeType string
}

// UploadImage lưu ảnh bìa lên storage khi CHƯA có voice nào.
//
// Màn tạo Voice là một bước: người dùng chọn ảnh trước cả khi Bài Post tồn tại,
// nên không thể đi qua SetImage (cần voice id). Ảnh nằm sẵn trên storage, URL
// của nó đi kèm request tạo Bài Post và được gắn vào voice sinh ra.
//
// Người dùng chọn ảnh rồi đóng hộp thoại thì file này thành rác — đổi lại là
// không phải tạo trước một voice rỗng chỉ để có chỗ treo ảnh. Ảnh bìa vài trăm
// KB, dọn định kỳ rẻ hơn nhiều so với dọn voice rỗng.
func (v *Voice) UploadImage(ctx context.Context, actor uuid.UUID, in ImageInput) (string, error) {
	data, ext, err := validateImage(in)
	if err != nil {
		return "", err
	}

	key := fmt.Sprintf("voice-images/pending/%s/%s%s", actor, uuid.NewString(), ext)
	url, err := v.storage.Put(ctx, key, data, in.MimeType)
	if err != nil {
		return "", fmt.Errorf("lưu ảnh bìa: %w", err)
	}
	return url, nil
}

// validateImage kiểm tra ảnh tải lên: rỗng, quá lớn, hoặc không phải định dạng
// multime hiển thị được.
func validateImage(in ImageInput) ([]byte, string, error) {
	if len(in.Data) == 0 {
		return nil, "", fmt.Errorf("%w: file ảnh rỗng", domain.ErrInvalidInput)
	}
	if len(in.Data) > maxVoiceImageBytes {
		return nil, "", fmt.Errorf("%w: ảnh lớn hơn %d MB",
			domain.ErrInvalidInput, maxVoiceImageBytes>>20)
	}
	ext, ok := voiceImageTypes[strings.ToLower(strings.TrimSpace(in.MimeType))]
	if !ok {
		return nil, "", fmt.Errorf("%w: chỉ nhận ảnh JPG, PNG, WEBP hoặc GIF",
			domain.ErrInvalidInput)
	}
	return in.Data, ext, nil
}

// SetImage lưu ảnh bìa tải từ máy vào storage rồi trỏ voice sang ảnh đó.
//
// Ảnh nằm trong bucket của mình nên được đánh dấu image_uploaded: sau khi đăng
// lên multime (nơi đã giữ một bản) thì bản này bị xoá để khỏi tốn dung lượng —
// xem Engine.publish. Ảnh cũ (nếu cũng là ảnh tải lên) bị xoá ngay tại đây,
// không để lại file mồ côi mỗi lần người dùng đổi ảnh.
func (v *Voice) SetImage(ctx context.Context, actor, id uuid.UUID, in ImageInput) (repository.Voice, error) {
	before, err := v.Get(ctx, id)
	if err != nil {
		return repository.Voice{}, err
	}
	if before.PublishStatus == domain.PublishPublished {
		return repository.Voice{}, domain.ErrAlreadyPublished
	}
	if before.PublishStatus == domain.PublishProcessing {
		return repository.Voice{}, errVoiceProcessing
	}
	data, ext, err := validateImage(in)
	if err != nil {
		return repository.Voice{}, err
	}

	// Key có thêm đoạn ngẫu nhiên: đổi ảnh là ra URL mới, trình duyệt không
	// hiện lại ảnh cũ trong cache.
	key := fmt.Sprintf("voice-images/%s/%s%s", id, uuid.NewString(), ext)
	url, err := v.storage.Put(ctx, key, data, in.MimeType)
	if err != nil {
		return repository.Voice{}, fmt.Errorf("lưu ảnh bìa: %w", err)
	}

	after, err := v.q.SetVoiceImage(ctx, repository.SetVoiceImageParams{
		ID: id, ImageUrl: &url, ImageUploaded: true,
	})
	if err != nil {
		// Ghi DB hỏng thì ảnh vừa lên không ai trỏ tới -> dọn luôn.
		if derr := v.storage.Delete(ctx, key); derr != nil {
			return repository.Voice{}, fmt.Errorf("%w (ảnh thừa %s chưa xoá được: %v)",
				wrapNotFound(err, "voice "+id.String()), key, derr)
		}
		return repository.Voice{}, wrapNotFound(err, "voice "+id.String())
	}

	v.deleteUploadedImage(ctx, before)
	v.audit.Record(ctx, actor, domain.AuditUpdate, domain.ObjectVoice, id, map[string]any{
		"action": "set_image", "image_url": url,
	})
	return after, nil
}

// deleteUploadedImage dọn ảnh bìa nằm trong storage của mình. Ảnh lấy từ URL
// bài gốc không phải của mình nên không đụng tới.
func (v *Voice) deleteUploadedImage(ctx context.Context, voice repository.Voice) {
	if !voice.ImageUploaded || voice.ImageUrl == nil {
		return
	}
	key := v.storage.KeyFromURL(*voice.ImageUrl)
	if key == "" {
		return
	}
	// Ảnh thừa chỉ tốn vài chục KB — không đáng để làm hỏng thao tác chính.
	_ = v.storage.Delete(ctx, key)
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
	v.deleteUploadedImage(ctx, voice)

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
	if voice.PublishStatus == domain.PublishProcessing {
		return errVoiceProcessing
	}
	if voice.VoiceFileUrl == nil {
		return domain.ErrNoVoiceFile
	}
	if err := publishable(voice); err != nil {
		return err
	}

	if err := v.enq.EnqueueVoicePublish(ctx, id.String(), actor.String()); err != nil {
		return err
	}
	v.audit.Record(ctx, actor, domain.AuditPublish, domain.ObjectVoice, id, nil)
	return nil
}
