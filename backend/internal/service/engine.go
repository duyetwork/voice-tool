package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/strongbody/voice-tool/backend/internal/config"
	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/repository"
)

// Engine là Core Engine chạy trong worker: Bài Post -> Voice -> multime.ai.
// Không được gọi từ HTTP handler (business rule #10).
type Engine struct {
	q         *repository.Queries
	platforms domain.PlatformRegistry
	tts       domain.TTSProvider
	stt       domain.STTProvider
	llm       domain.LLMProvider
	storage   domain.Storage
	prober    domain.AudioProber
	multime   domain.MultimeClient
	creds     *MultimeCreds
	enq       domain.Enqueuer
	audit     *Audit
	log       *slog.Logger
}

type EngineDeps struct {
	Queries   *repository.Queries
	Platforms domain.PlatformRegistry
	TTS       domain.TTSProvider
	STT       domain.STTProvider
	LLM       domain.LLMProvider
	Storage   domain.Storage
	Prober    domain.AudioProber
	Multime   domain.MultimeClient
	Creds     *MultimeCreds
	Enqueuer  domain.Enqueuer
	Audit     *Audit
	Logger    *slog.Logger
}

func NewEngine(d EngineDeps) *Engine {
	return &Engine{
		q: d.Queries, platforms: d.Platforms, tts: d.TTS, stt: d.STT, llm: d.LLM,
		storage: d.Storage, prober: d.Prober, multime: d.Multime, creds: d.Creds,
		enq: d.Enqueuer, audit: d.Audit, log: d.Logger,
	}
}

// ---------------------------------------------------------------------------
// post:metadata
// ---------------------------------------------------------------------------

// FetchPostMetadata lấy metadata gốc của Bài Post (tiêu đề, mô tả, hashtag,
// ảnh bìa, tác giả, ngày đăng) và lưu lên chính bài đó.
//
// Chạy ngay khi tạo Bài Post, KHÔNG tạo voice — nhờ vậy bảng Bài Post có nội
// dung đọc được trước cả khi bấm tạo Voice, và "chạy Voice" đúng nghĩa là chỉ
// tạo voice. Áp dụng cho cả mode A, B, C vì nó không phụ thuộc mode.
//
// Lỗi ở đây KHÔNG đặt status = failed: bài vẫn chạy voice được, chỉ là thiếu
// phần điền sẵn. Chỉ ghi last_error để người dùng biết vì sao bảng trống.
func (e *Engine) FetchPostMetadata(ctx context.Context, postID uuid.UUID) error {
	post, err := e.q.GetSourcePost(ctx, postID)
	if err != nil {
		return fmt.Errorf("đọc source_post %s: %w", postID, err)
	}

	adapter, err := e.platforms.Get(domain.Platform(post.Platform))
	if err != nil {
		return domain.Permanent(err)
	}

	meta, err := adapter.FetchMetadata(ctx, domain.PostRef{
		URL:    post.SourceUrl,
		PostID: deref(post.PostIDExtracted),
	})
	if err != nil {
		msg := domain.UserMessage(err)
		e.log.WarnContext(ctx, "không lấy được metadata bài post",
			"error", err, "source_post_id", postID)
		if _, serr := e.q.SetSourcePostStatus(ctx, repository.SetSourcePostStatusParams{
			ID:        post.ID,
			Status:    post.Status,
			LastError: &msg,
		}); serr != nil {
			e.log.ErrorContext(ctx, "không ghi được last_error", "error", serr, "source_post_id", postID)
		}
		return err
	}

	e.saveMetadata(ctx, post, meta)
	return nil
}

// ---------------------------------------------------------------------------
// voice:process
// ---------------------------------------------------------------------------

// ProcessSourcePost chạy Bài Post theo collect_mode và tạo ra 1 Voice.
//
//	A -> tải audio gốc
//	B -> text (caption/transcript, fallback STT) -> TTS
//	C -> text -> LLM theo Prompt mẫu -> TTS
//
// voiceID là record voice `processing` đã tạo sẵn lúc enqueue; uuid.Nil nghĩa
// là task cũ chưa có record, engine tự tạo như trước.
func (e *Engine) ProcessSourcePost(ctx context.Context, postID, actor, voiceID uuid.UUID) error {
	// Claim để idempotent khi Asynq retry hoặc 2 worker cùng nhận task.
	post, err := e.q.ClaimSourcePostForProcessing(ctx, postID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			e.log.InfoContext(ctx, "voice:process bỏ qua — bài post không ở trạng thái chạy được",
				"source_post_id", postID)
			return nil
		}
		return fmt.Errorf("claim source_post %s: %w", postID, err)
	}

	voice, err := e.buildVoice(ctx, post, actor, voiceID)
	if err != nil {
		// last_error là thứ người dùng đọc trên UI -> chỉ lưu câu ngắn;
		// nguyên văn lỗi (stderr yt-dlp, exit code) đi vào log.
		msg := domain.UserMessage(err)
		e.log.ErrorContext(ctx, "voice:process thất bại",
			"error", err, "source_post_id", post.ID, "voice_id", voiceID)
		if _, serr := e.q.SetSourcePostStatus(ctx, repository.SetSourcePostStatusParams{
			ID:        post.ID,
			Status:    domain.PostStatusFailed,
			LastError: &msg,
		}); serr != nil {
			e.log.ErrorContext(ctx, "không cập nhật được status failed", "error", serr, "source_post_id", post.ID)
		}
		// Record processing phải chuyển sang failed, không thì nó treo ở "đang
		// xử lý" mãi và người dùng không biết đã lỗi.
		if voiceID != uuid.Nil {
			if _, serr := e.q.SetVoicePublishStatus(ctx, repository.SetVoicePublishStatusParams{
				ID:            voiceID,
				PublishStatus: domain.PublishFailed,
				LastError:     &msg,
			}); serr != nil {
				e.log.ErrorContext(ctx, "không cập nhật được voice failed", "error", serr, "voice_id", voiceID)
			}
		}
		return err
	}

	if _, err := e.q.SetSourcePostStatus(ctx, repository.SetSourcePostStatusParams{
		ID:     post.ID,
		Status: domain.PostStatusProcessed,
	}); err != nil {
		return fmt.Errorf("cập nhật status processed: %w", err)
	}

	e.audit.Record(ctx, actor, domain.AuditCreate, domain.ObjectVoice, voice.ID, map[string]any{
		"source_post_id": post.ID,
		"collect_mode":   post.CollectMode,
		"language":       voice.Language,
	})

	// auto_publish theo cấu hình của Danh sách nguồn (business rule #7).
	autoPublish, err := e.autoPublishFor(ctx, post)
	if err != nil {
		e.log.WarnContext(ctx, "không đọc được auto_publish của danh sách", "error", err, "source_post_id", post.ID)
	}
	if autoPublish {
		if err := e.enq.EnqueueVoicePublish(ctx, voice.ID.String(), actor.String()); err != nil {
			e.log.ErrorContext(ctx, "enqueue voice:publish thất bại", "error", err, "voice_id", voice.ID)
		}
	}
	return nil
}

func (e *Engine) buildVoice(
	ctx context.Context,
	post repository.SourcePost,
	actor, voiceID uuid.UUID,
) (repository.Voice, error) {
	mode := domain.CollectMode(post.CollectMode)
	if !mode.Valid() {
		return repository.Voice{}, domain.Permanent(
			fmt.Errorf("%w: collect_mode %q", domain.ErrInvalidInput, post.CollectMode))
	}

	adapter, err := e.platforms.Get(domain.Platform(post.Platform))
	if err != nil {
		return repository.Voice{}, domain.Permanent(err)
	}

	postID := deref(post.PostIDExtracted)
	if postID == "" {
		return repository.Voice{}, domain.Permanent(
			fmt.Errorf("%w: bài post chưa có post_id_extracted", domain.ErrInvalidInput))
	}

	fetched, err := adapter.FetchContent(ctx,
		domain.PostRef{URL: post.SourceUrl, PostID: postID}, mode)
	if err != nil {
		return repository.Voice{}, fmt.Errorf("fetch nội dung từ %s: %w", post.Platform, err)
	}

	// KHÔNG ghi metadata lên Bài Post ở đây: việc đó thuộc task post:metadata
	// chạy lúc tạo bài (một nơi ghi duy nhất). "Chạy Voice" chỉ tạo Voice.
	// Metadata vừa fetch vẫn dùng để điền cho chính Voice bên dưới.

	// Ngôn ngữ: 'auto' nghĩa là lấy theo nền tảng khai báo, không đoán bừa.
	// Nền tảng không nói gì thì giữ 'auto' để multime.ai tự nhận diện từ audio.
	language := post.Language
	if domain.IsAutoLanguage(language) {
		language = domain.LanguageAuto
		if detected := baseLanguage(fetched.Meta.Language); detected != "" {
			language = detected
		}
	}

	var (
		audio    []byte
		engineID *uuid.UUID
		// Tiêu đề Voice lấy từ tiêu đề Bài Post (= toàn bộ nội dung bài, trừ
		// hashtag), gộp về 1 dòng và cắt theo giới hạn của multime. Metadata vừa
		// fetch được ưu tiên hơn bản đã lưu vì nó mới hơn.
		title *string
	)

	if mode == domain.ModeExtract {
		// Mode A: không qua TTS -> text dự phòng là caption/transcript nền
		// tảng trả về, hoặc text đã lưu từ lần chạy trước.
		audio, err = e.audioFor(ctx, fetched)
		if err != nil {
			return repository.Voice{}, err
		}
		title = nilIfEmpty(domain.VoiceTitle(firstNonEmpty(
			fetched.Meta.Title, deref(post.Title), fetched.Text, deref(post.ExtractedText))))
	} else {
		sourceText, spoken, err := e.textFor(ctx, post, mode, fetched)
		if err != nil {
			return repository.Voice{}, err
		}
		// Mode B/C: text dự phòng là nội dung TTS đọc ra (mode B là text gốc,
		// mode C là bản LLM đã viết lại).
		title = nilIfEmpty(domain.VoiceTitle(firstNonEmpty(
			fetched.Meta.Title, deref(post.Title), spoken)))

		// Lưu text NGUỒN (không phải bản LLM viết lại) để chạy lại Voice khác
		// với prompt khác mà không cần fetch URL lần nữa.
		if _, err := e.q.UpdateSourcePost(ctx, repository.UpdateSourcePostParams{
			ID:            post.ID,
			ExtractedText: &sourceText,
		}); err != nil {
			e.log.WarnContext(ctx, "không lưu được extracted_text", "error", err, "source_post_id", post.ID)
		}

		engine, err := e.pickEngine(ctx, language)
		if err != nil {
			return repository.Voice{}, err
		}
		if engine != nil {
			engineID = &engine.ID
		}

		// TTS nhận chuỗi rỗng khi chưa chốt ngôn ngữ -> provider dùng mặc định
		// của chính nó thay vì bị ép đọc sai giọng.
		ttsLang := language
		if domain.IsAutoLanguage(ttsLang) {
			ttsLang = ""
		}
		audio, err = e.tts.Synthesize(ctx, spoken, ttsLang)
		if err != nil {
			return repository.Voice{}, fmt.Errorf("TTS (%s): %w", e.tts.Name(), err)
		}
	}

	if len(audio) == 0 {
		return repository.Voice{}, fmt.Errorf("không tạo được dữ liệu audio")
	}

	// Đo metadata kỹ thuật — multime.ai cần duration/size/mime/sample_rate để
	// tạo audio asset. Lỗi ffprobe không chặn luồng, chỉ mất metadata.
	info := domain.AudioInfo{SizeBytes: int64(len(audio)), MimeType: "audio/mpeg"}
	if e.prober != nil {
		probed, err := e.prober.Probe(ctx, audio)
		if err != nil {
			e.log.WarnContext(ctx, "không đo được metadata audio",
				"error", err, "source_post_id", post.ID)
		} else {
			info = probed
		}
	}
	if info.MimeType == "" || info.MimeType == "application/octet-stream" {
		info.MimeType = "audio/mpeg"
	}

	key := fmt.Sprintf("voices/%s/%s-%d.mp3", post.ID, postID, time.Now().Unix())
	fileURL, err := e.storage.Put(ctx, key, audio, info.MimeType)
	if err != nil {
		return repository.Voice{}, fmt.Errorf("lưu file voice lên storage: %w", err)
	}

	hashtag := nilIfEmpty(strings.Join(fetched.Meta.Hashtags, " "))
	image := nilIfEmpty(fetched.Meta.ThumbnailURL)

	// Có record `processing` tạo sẵn lúc enqueue -> điền vào đúng record đó để
	// người dùng thấy voice chuyển trạng thái tại chỗ, không nhân thêm dòng.
	if voiceID != uuid.Nil {
		voice, err := e.q.FinishVoice(ctx, repository.FinishVoiceParams{
			ID:              voiceID,
			AiEngineID:      engineID,
			VoiceFileUrl:    &fileURL,
			DurationSeconds: nilIfZero(int32(info.DurationSeconds)),
			Title:           title,
			Hashtag:         hashtag,
			ImageUrl:        image,
			Language:        language,
			MimeType:        nilIfEmpty(info.MimeType),
			SizeBytes:       ptr(info.SizeBytes),
			SampleRate:      nilIfZero(int32(info.SampleRate)),
		})
		if err != nil {
			e.cleanupOrphan(ctx, key)
			if errors.Is(err, pgx.ErrNoRows) {
				// Người dùng đã xoá voice trong lúc worker đang chạy.
				return repository.Voice{}, domain.Permanent(fmt.Errorf(
					"%w: voice %s đã bị xoá trong lúc xử lý", domain.ErrNotFound, voiceID))
			}
			return repository.Voice{}, fmt.Errorf("cập nhật voice: %w", err)
		}
		return voice, nil
	}

	voice, err := e.q.CreateVoice(ctx, repository.CreateVoiceParams{
		SourcePostID:    post.ID,
		AiEngineID:      engineID,
		VoiceFileUrl:    &fileURL,
		DurationSeconds: nilIfZero(int32(info.DurationSeconds)),
		Hashtag:         hashtag,
		ImageUrl:        image,
		Language:        language,
		PublishStatus:   domain.PublishDraft,
		Title:           title,
		MimeType:        nilIfEmpty(info.MimeType),
		SizeBytes:       ptr(info.SizeBytes),
		SampleRate:      nilIfZero(int32(info.SampleRate)),
		CreatedBy:       actor,
	})
	if err != nil {
		e.cleanupOrphan(ctx, key)
		return repository.Voice{}, fmt.Errorf("tạo voice: %w", err)
	}
	return voice, nil
}

// cleanupOrphan dọn file vừa upload khi không ghi được record — tránh rác trên
// storage mà không có gì trong DB trỏ tới.
func (e *Engine) cleanupOrphan(ctx context.Context, key string) {
	if err := e.storage.Delete(ctx, key); err != nil {
		e.log.WarnContext(ctx, "không dọn được file voice mồ côi", "error", err, "key", key)
	}
}

// saveMetadata ghi metadata gốc của bài (tiêu đề = nội dung bài, hashtag, ảnh
// bìa, tác giả, ngày đăng) lên Bài Post. Không ghi được thì chỉ mất phần
// auto-fill, không chặn việc tạo Voice.
func (e *Engine) saveMetadata(
	ctx context.Context,
	post repository.SourcePost,
	meta domain.PostMetadata,
) repository.SourcePost {
	if meta.Title == "" && meta.ThumbnailURL == "" && len(meta.Hashtags) == 0 {
		return post
	}
	updated, err := e.q.UpdateSourcePostMetadata(ctx, repository.UpdateSourcePostMetadataParams{
		ID:           post.ID,
		Title:        nilIfEmpty(meta.Title),
		Hashtags:     meta.Hashtags,
		ThumbnailUrl: nilIfEmpty(meta.ThumbnailURL),
		AuthorName:   nilIfEmpty(meta.AuthorName),
		PostedAt:     meta.PostedAt,
	})
	if err != nil {
		e.log.WarnContext(ctx, "không lưu được metadata bài post",
			"error", err, "source_post_id", post.ID)
		return post
	}
	return updated
}

// audioFor lấy audio cho Mode A: adapter có thể trả bytes trực tiếp hoặc
// trả URL/object key trên storage.
func (e *Engine) audioFor(ctx context.Context, fetched domain.FetchedContent) ([]byte, error) {
	if len(fetched.AudioBytes) > 0 {
		return fetched.AudioBytes, nil
	}
	if fetched.AudioFileURL == "" {
		return nil, domain.Permanent(fmt.Errorf("mode A: adapter không trả về audio"))
	}
	key := e.storage.KeyFromURL(fetched.AudioFileURL)
	data, err := e.storage.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("đọc audio %s: %w", key, err)
	}
	return data, nil
}

// textFor lấy text cho Mode B/C. Ưu tiên caption/transcript từ nền tảng, không
// có thì nghe audio qua STT; Mode C viết lại qua LLM theo Prompt mẫu.
//
// Trả về (text nguồn, text sẽ đọc). Mode B hai giá trị bằng nhau; mode C giá
// trị thứ hai là bản LLM đã viết lại.
func (e *Engine) textFor(
	ctx context.Context,
	post repository.SourcePost,
	mode domain.CollectMode,
	fetched domain.FetchedContent,
) (sourceText string, spokenText string, err error) {
	sourceText = strings.TrimSpace(fetched.Text)

	if sourceText == "" && len(fetched.AudioBytes) > 0 {
		transcribed, err := e.stt.Transcribe(ctx, fetched.AudioBytes, post.Language)
		if err != nil {
			return "", "", fmt.Errorf("STT (%s): %w", e.stt.Name(), err)
		}
		sourceText = strings.TrimSpace(transcribed)
	}
	if sourceText == "" {
		// Chạy lại Bài Post đã fetch trước đó — dùng text đã lưu.
		sourceText = strings.TrimSpace(deref(post.ExtractedText))
	}
	if sourceText == "" {
		return "", "", domain.Permanent(domain.ErrNoTextExtracted)
	}

	if mode != domain.ModePromptToVoice {
		return sourceText, sourceText, nil
	}

	if post.PromptID == nil {
		return "", "", domain.Permanent(domain.ErrPromptRequired)
	}
	prompt, err := e.q.GetPrompt(ctx, *post.PromptID)
	if err != nil {
		return "", "", domain.Permanent(wrapNotFound(err, "prompt "+post.PromptID.String()))
	}

	generated, err := e.llm.Generate(ctx, prompt.Content, sourceText)
	if err != nil {
		return "", "", fmt.Errorf("LLM (%s): %w", e.llm.Name(), err)
	}
	generated = strings.TrimSpace(generated)
	if generated == "" {
		return "", "", fmt.Errorf("LLM trả về nội dung rỗng")
	}
	return sourceText, generated, nil
}

// pickEngine chọn AI Engine và validate ngôn ngữ (specs 3.3). Chưa cấu hình
// engine nào thì bỏ qua, dùng provider mặc định trong .env.
func (e *Engine) pickEngine(ctx context.Context, language string) (*repository.AiEngine, error) {
	engine, err := e.q.GetDefaultAIEngine(ctx)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("lấy ai_engine mặc định: %w", err)
	}
	// Chưa chốt ngôn ngữ thì không có gì để validate — engine tự xử.
	if domain.IsAutoLanguage(language) {
		return &engine, nil
	}
	if !languageSupported(language, engine.SupportedLanguages) {
		return nil, domain.Permanent(fmt.Errorf("%w: engine %s không hỗ trợ %s",
			domain.ErrLangUnsupported, engine.Name, language))
	}
	return &engine, nil
}

func (e *Engine) autoPublishFor(ctx context.Context, post repository.SourcePost) (bool, error) {
	switch {
	case post.ListBreakingID != nil:
		list, err := e.q.GetListBreaking(ctx, *post.ListBreakingID)
		if err != nil {
			return false, err
		}
		return list.AutoPublish, nil
	case post.ListScheduledID != nil:
		list, err := e.q.GetListScheduled(ctx, *post.ListScheduledID)
		if err != nil {
			return false, err
		}
		return list.AutoPublish, nil
	default:
		// F1: mặc định tắt — luôn qua bước preview trước khi đăng (specs -1).
		return false, nil
	}
}

// ---------------------------------------------------------------------------
// voice:publish
// ---------------------------------------------------------------------------

// PublishVoice đăng Voice lên multime.ai rồi xoá file nội bộ (business rule #2).
func (e *Engine) PublishVoice(ctx context.Context, voiceID, actor uuid.UUID) error {
	voice, err := e.q.GetVoice(ctx, voiceID)
	if err != nil {
		return domain.Permanent(wrapNotFound(err, "voice "+voiceID.String()))
	}
	if voice.PublishStatus == domain.PublishPublished {
		e.log.InfoContext(ctx, "voice:publish bỏ qua — đã publish", "voice_id", voiceID)
		return nil
	}
	if voice.VoiceFileUrl == nil {
		return domain.Permanent(domain.ErrNoVoiceFile)
	}

	if err := e.publish(ctx, voice); err != nil {
		msg := err.Error()
		// Publish thất bại thì GIỮ NGUYÊN file để retry (business rule #2).
		if _, serr := e.q.SetVoicePublishStatus(ctx, repository.SetVoicePublishStatusParams{
			ID:            voiceID,
			PublishStatus: domain.PublishFailed,
			LastError:     &msg,
		}); serr != nil {
			e.log.ErrorContext(ctx, "không cập nhật được publish_status failed", "error", serr, "voice_id", voiceID)
		}
		return err
	}

	e.audit.Record(ctx, actor, domain.AuditPublish, domain.ObjectVoice, voiceID, nil)
	return nil
}

func (e *Engine) publish(ctx context.Context, voice repository.Voice) error {
	key := e.storage.KeyFromURL(*voice.VoiceFileUrl)
	audio, err := e.storage.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("đọc file voice %s: %w", key, err)
	}

	// Ảnh bìa: API nhận file, không nhận URL — phải tải về trước.
	var imageBytes []byte
	var imageName string
	if url := deref(voice.ImageUrl); url != "" {
		imageBytes, imageName, err = e.fetchImage(ctx, url)
		if err != nil {
			// Thiếu ảnh bìa không đáng để chặn việc đăng.
			e.log.WarnContext(ctx, "không tải được ảnh bìa, đăng không kèm ảnh",
				"error", err, "voice_id", voice.ID, "image_url", url)
		}
	}

	// Ngôn ngữ 'auto' -> gửi rỗng để multime tự nhận diện từ audio.
	lang := voice.Language
	if domain.IsAutoLanguage(lang) {
		lang = ""
	}

	post := domain.VoicePostInput{
		FileName: filepath.Base(key),
		MimeType: deref(voice.MimeType),
		Title:    publishTitle(voice),
		// Language rỗng thì multime tự nhận diện từ audio — không đoán bừa.
		Language:        lang,
		SourceLang:      lang,
		Hashtags:        config.SplitHashtags(deref(voice.Hashtag)),
		ImageBytes:      imageBytes,
		ImageName:       imageName,
		DurationSeconds: int(deref(voice.DurationSeconds)),
	}

	postURL, err := e.publishAs(ctx, voice.CreatedBy, audio, post)
	if err != nil {
		return fmt.Errorf("đăng voice lên multime: %w", err)
	}

	// Ghi DB trước rồi mới xoá file: nếu xoá lỗi thì chỉ còn file rác, không
	// mất dấu bài đã đăng.
	if _, err := e.q.MarkVoicePublished(ctx, repository.MarkVoicePublishedParams{
		ID:             voice.ID,
		MultimePostUrl: &postURL,
	}); err != nil {
		return fmt.Errorf("cập nhật voice published: %w", err)
	}

	if err := e.storage.Delete(ctx, key); err != nil {
		e.log.WarnContext(ctx, "đã publish nhưng không xoá được file voice",
			"error", err, "voice_id", voice.ID, "key", key)
	}
	return nil
}

// publishAs đăng voice bằng credential của user đã tạo ra nó — voice xuất hiện
// trên multime dưới đúng tài khoản đó, không phải một tài khoản hệ thống dùng chung.
//
// Access token hết hạn thì refresh 1 lần rồi thử lại; refresh cũng thất bại thì
// trả lỗi vĩnh viễn yêu cầu user đăng nhập lại (không có cách tự khắc phục).
func (e *Engine) publishAs(
	ctx context.Context,
	ownerID uuid.UUID,
	audio []byte,
	post domain.VoicePostInput,
) (string, error) {
	creds, err := e.creds.For(ctx, ownerID)
	if err != nil {
		return "", err
	}

	url, err := e.multime.PublishVoice(ctx, creds, audio, post)
	if err == nil {
		return url, nil
	}
	if !errors.Is(err, domain.ErrTokenExpired) {
		return "", err
	}

	e.log.InfoContext(ctx, "token multime hết hạn, thử refresh", "user_id", ownerID)
	refreshed, refreshErr := e.creds.Refresh(ctx, ownerID)
	if refreshErr != nil {
		return "", refreshErr
	}
	return e.multime.PublishVoice(ctx, refreshed, audio, post)
}

// ---------------------------------------------------------------------------
// helpers cho bước publish
// ---------------------------------------------------------------------------

// maxImageBytes chặn ảnh bìa quá lớn khi tải từ URL người dùng nhập.
const maxImageBytes = 8 << 20

// publishTitle: multime bắt buộc có title, và đó là toàn bộ phần chữ của bài
// đăng — luôn phải có giá trị, 1 dòng, trong giới hạn ký tự.
func publishTitle(voice repository.Voice) string {
	if t := domain.VoiceTitle(deref(voice.Title)); t != "" {
		return t
	}
	// Bài không có tiêu đề nào dùng được -> đặt tên theo id để vẫn đăng được.
	return "Voice " + voice.ID.String()[:8]
}

// fetchImage tải ảnh bìa từ URL để đính kèm vào request publish.
func (e *Engine) fetchImage(ctx context.Context, url string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("tải ảnh bìa trả về %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > maxImageBytes {
		return nil, "", fmt.Errorf("ảnh bìa lớn hơn %d MB", maxImageBytes>>20)
	}

	name := path.Base(url)
	if ext := path.Ext(name); ext == "" {
		name = "cover" + extensionFor(resp.Header.Get("Content-Type"))
	}
	return data, name, nil
}

func extensionFor(contentType string) string {
	switch {
	case strings.Contains(contentType, "png"):
		return ".png"
	case strings.Contains(contentType, "webp"):
		return ".webp"
	case strings.Contains(contentType, "gif"):
		return ".gif"
	default:
		return ".jpg"
	}
}
