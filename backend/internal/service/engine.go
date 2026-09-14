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
	"github.com/strongbody/voice-tool/backend/internal/pkg/secret"
	"github.com/strongbody/voice-tool/backend/internal/repository"
)

// Engine là Core Engine chạy trong worker: Bài Post -> Voice -> multime.ai.
// Không được gọi từ HTTP handler (business rule #10).
type Engine struct {
	q         *repository.Queries
	platforms domain.PlatformRegistry
	// tts là provider dự phòng cấu hình trong .env (mock khi dev). Đường chính
	// là API key của từng user ở bảng ai_engine — xem ttsFor.
	tts        domain.TTSProvider
	ttsFactory domain.TTSFactory
	box        *secret.Box
	stt        domain.STTProvider
	// llm là ROUTER, không phải 1 provider: nó chọn key + model theo Bộ API gắn
	// trên voice và tự chuyển dự phòng khi hết hạn mức (xem llmrouter.go).
	llm     LLMGenerator
	storage domain.Storage
	prober  domain.AudioProber
	multime domain.MultimeClient
	creds   *MultimeCreds
	enq     domain.Enqueuer
	audit   *Audit
	log     *slog.Logger
	// gate giữ nhịp gọi yt-dlp theo từng nền tảng; stats đếm số lần bị chặn.
	gate  *PlatformGate
	stats *FetchStats
	// authors bốc tài khoản Strongbody đứng tên bài đăng — chạy ở BƯỚC ĐĂNG,
	// không phải lúc người dùng chọn giới tính trên form.
	authors *MultimeUsers
}

type EngineDeps struct {
	Queries   *repository.Queries
	Platforms domain.PlatformRegistry
	TTS       domain.TTSProvider
	// TTSFactory dựng provider từ API key của user (bảng ai_engine).
	TTSFactory domain.TTSFactory
	Secret     *secret.Box
	STT        domain.STTProvider
	LLM        LLMGenerator
	Storage    domain.Storage
	Prober     domain.AudioProber
	Multime    domain.MultimeClient
	Creds      *MultimeCreds
	Enqueuer   domain.Enqueuer
	Audit      *Audit
	Logger     *slog.Logger
	Gate       *PlatformGate
	FetchStats *FetchStats
	Authors    *MultimeUsers
}

func NewEngine(d EngineDeps) *Engine {
	return &Engine{
		q: d.Queries, platforms: d.Platforms, tts: d.TTS, ttsFactory: d.TTSFactory,
		box: d.Secret, stt: d.STT, llm: d.LLM,
		storage: d.Storage, prober: d.Prober, multime: d.Multime, creds: d.Creds,
		enq: d.Enqueuer, audit: d.Audit, log: d.Logger,
		gate: d.Gate, stats: d.FetchStats, authors: d.Authors,
	}
}

// fetchContent / fetchMetadata gọi adapter qua PlatformGate — cùng lý do với
// Scan.latestPosts: tải bài cho voice cũng là một lần gọi yt-dlp tới nền tảng,
// và nó còn dày hơn quét kênh. Giới hạn ở một chỗ mà bỏ chỗ kia thì nền tảng
// vẫn thấy đúng lượng request như cũ.
func (e *Engine) fetchContent(
	ctx context.Context,
	adapter domain.PlatformAdapter,
	platform string,
	ref domain.PostRef,
	mode domain.CollectMode,
) (domain.FetchedContent, error) {
	release, err := e.gate.Acquire(ctx, platform)
	if err != nil {
		return domain.FetchedContent{}, err
	}
	defer release()

	out, err := adapter.FetchContent(ctx, ref, mode)
	if err != nil {
		e.stats.Record(ctx, platform, err)
	}
	return out, err
}

func (e *Engine) fetchMetadata(
	ctx context.Context,
	adapter domain.PlatformAdapter,
	platform string,
	ref domain.PostRef,
) (domain.PostMetadata, error) {
	release, err := e.gate.Acquire(ctx, platform)
	if err != nil {
		return domain.PostMetadata{}, err
	}
	defer release()

	out, err := adapter.FetchMetadata(ctx, ref)
	if err != nil {
		e.stats.Record(ctx, platform, err)
	}
	return out, err
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

	// Bài nhập tay bằng text không có URL để fetch: metadata (tiêu đề, hashtag)
	// đã dựng ngay lúc tạo bài từ chính text đó.
	if isTextPost(post) {
		return nil
	}

	adapter, err := e.platforms.Get(domain.Platform(post.Platform))
	if err != nil {
		return domain.Permanent(err)
	}

	meta, err := e.fetchMetadata(ctx, adapter, post.Platform, domain.PostRef{
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

	// auto_publish theo cấu hình của Danh sách nguồn (business rule #7), hoặc
	// người dùng đã bấm "Đăng" ngay ở màn tạo Voice (publish_when_ready).
	autoPublish, err := e.autoPublishFor(ctx, post)
	if err != nil {
		e.log.WarnContext(ctx, "không đọc được auto_publish của danh sách", "error", err, "source_post_id", post.ID)
	}
	autoPublish = autoPublish || voice.PublishWhenReady
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

	// Bản ghi voice `processing` mang theo metadata người dùng đã điền ở màn tạo
	// (tiêu đề, hashtag, ngôn ngữ, ảnh, author). Đọc ngay từ đây vì ngôn ngữ họ
	// chọn còn quyết định GIỌNG TTS ở dưới, không chỉ là thứ hiển thị.
	var current repository.Voice
	if voiceID != uuid.Nil {
		if v, err := e.q.GetVoice(ctx, voiceID); err == nil {
			current = v
		} else {
			e.log.WarnContext(ctx, "không đọc được voice đang xử lý để giữ metadata người dùng",
				"error", err, "voice_id", voiceID)
		}
	}

	// Bài nhập tay bằng text đi đường riêng: không có URL, không có adapter,
	// nội dung chính là text người dùng đã gõ.
	postID := deref(post.PostIDExtracted)
	var (
		fetched domain.FetchedContent
		err     error
	)
	if isTextPost(post) {
		if mode == domain.ModeExtract {
			return repository.Voice{}, domain.Permanent(fmt.Errorf(
				"%w: bài nhập bằng text không có audio gốc để tách, dùng hình thức B hoặc C",
				domain.ErrInvalidInput))
		}
		fetched, err = textContent(post)
		if err != nil {
			return repository.Voice{}, err
		}
	} else {
		adapter, aerr := e.platforms.Get(domain.Platform(post.Platform))
		if aerr != nil {
			return repository.Voice{}, domain.Permanent(aerr)
		}
		if postID == "" {
			return repository.Voice{}, domain.Permanent(
				fmt.Errorf("%w: bài post chưa có post_id_extracted", domain.ErrInvalidInput))
		}
		fetched, err = e.fetchContent(ctx, adapter, post.Platform,
			domain.PostRef{URL: post.SourceUrl, PostID: postID}, mode)
		if err != nil {
			return repository.Voice{}, fmt.Errorf("fetch nội dung từ %s: %w", post.Platform, err)
		}
	}

	// KHÔNG ghi metadata lên Bài Post ở đây: việc đó thuộc task post:metadata
	// chạy lúc tạo bài (một nơi ghi duy nhất). "Chạy Voice" chỉ tạo Voice.
	// Metadata vừa fetch vẫn dùng để điền cho chính Voice bên dưới.

	// Ngôn ngữ: 'auto' nghĩa là lấy theo nền tảng khai báo, không đoán bừa.
	// Nền tảng không nói gì thì giữ 'auto' để multime.ai tự nhận diện từ audio.
	//
	// autoDetected ghi lại việc giá trị này do HỆ THỐNG đoán chứ không phải
	// người dùng chọn — TTS xử lý 2 trường hợp đó khác nhau (xem speechLanguage).
	// Người dùng chọn ngôn ngữ ở màn tạo Voice thì đó là chốt: nhận diện tự động
	// chỉ dành cho trường hợp họ để trống.
	language := firstNonEmpty(nonAutoLanguage(current.Language), post.Language)
	autoDetected := domain.IsAutoLanguage(language)
	if autoDetected {
		language = domain.LanguageAuto
		if detected := baseLanguage(fetched.Meta.Language); detected != "" {
			language = detected
		}
	}

	var (
		audio    []byte
		engineID *uuid.UUID
		// llmModel: model THẬT đã viết lại nội dung (chỉ mode C mới có).
		llmModel *string
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
		sourceText, spoken, model, err := e.textFor(ctx, post, mode, fetched, current.LlmApiSetID, actor)
		if err != nil {
			return repository.Voice{}, err
		}
		llmModel = nilIfEmpty(model)
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

		provider, engine, err := e.ttsFor(ctx, actor, language)
		if err != nil {
			return repository.Voice{}, err
		}
		if engine != nil {
			engineID = &engine.ID
		}

		ttsLang, err := e.speechLanguage(ctx, provider, language, autoDetected)
		if err != nil {
			return repository.Voice{}, err
		}
		audio, err = provider.Synthesize(ctx, spoken, ttsLang)
		if err != nil {
			return repository.Voice{}, fmt.Errorf("TTS (%s): %w", provider.Name(), err)
		}

		// Đóng dấu key vừa đọc xong — cột "dùng gần đây" ở màn AI Engine. Chỉ
		// đánh dấu khi TTS THÀNH CÔNG: key sai mà vẫn hiện "vừa dùng" thì người
		// dùng tưởng key còn sống. Ghi hỏng cũng không ảnh hưởng voice.
		if engineID != nil {
			if err := e.q.TouchAIEngineUsed(ctx, *engineID); err != nil {
				e.log.WarnContext(ctx, "không ghi được last_used_at của API key",
					"error", err, "ai_engine_id", *engineID)
			}
		}
	}

	if len(audio) == 0 {
		return repository.Voice{}, fmt.Errorf("không tạo được dữ liệu audio")
	}

	info := e.probe(ctx, audio, voiceID)

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
		// Metadata người dùng đã điền ở màn tạo Voice phải THẮNG thứ fetch được:
		// họ gõ tiêu đề riêng, chọn "không có ảnh", thêm hashtag của mình rồi mới
		// bấm Đăng — ghi đè bằng dữ liệu bài gốc là xoá đúng thứ họ vừa quyết.
		title = keepUserValue(current.Title, title)
		hashtag = ptrOrNil(mergeHashtags(deref(current.Hashtag), fetched.Meta.Hashtags))
		image = coverFor(current, image)

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
			LlmModelUsed:    llmModel,
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
		SourcePostID:    &post.ID,
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

// nonAutoLanguage trả về ngôn ngữ chỉ khi nó là lựa chọn thật của người dùng;
// "auto" hay rỗng nghĩa là chưa chọn.
func nonAutoLanguage(language string) string {
	if language == "" || domain.IsAutoLanguage(language) {
		return ""
	}
	return language
}

// keepUserValue giữ giá trị người dùng đã điền; chỉ dùng giá trị fetch được
// khi họ để trống.
func keepUserValue(userValue, fetched *string) *string {
	if strings.TrimSpace(deref(userValue)) != "" {
		return userValue
	}
	return fetched
}

// coverFor chọn ảnh bìa cho voice vừa tạo xong:
//
//	người dùng đã đưa ảnh        -> giữ nguyên ảnh đó
//	người dùng chọn "không ảnh"  -> không ảnh
//	còn lại                      -> ảnh bìa của bài gốc
func coverFor(current repository.Voice, fetched *string) *string {
	switch {
	case current.ImageUrl != nil:
		return current.ImageUrl
	case current.NoImage:
		return nil
	default:
		return fetched
	}
}

// ptrOrNil: chuỗi rỗng -> nil, để FinishVoice không ghi đè bằng giá trị trống.
func ptrOrNil(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
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

// textFor lấy text cho Mode B/C: caption/phụ đề nền tảng trả về, không có thì
// nghe audio gốc qua STT, cuối cùng mới tới text đã lưu từ lần chạy trước.
//
// Mode C viết lại phần text lấy được qua LLM theo Prompt mẫu.
//
// Trả về (text nguồn, text sẽ đọc). Mode B hai giá trị bằng nhau; mode C giá
// trị thứ hai là bản LLM đã viết lại.
func (e *Engine) textFor(
	ctx context.Context,
	post repository.SourcePost,
	mode domain.CollectMode,
	fetched domain.FetchedContent,
	llmSetID *uuid.UUID,
	owner uuid.UUID,
) (sourceText string, spokenText string, model string, err error) {
	sourceText = strings.TrimSpace(fetched.Text)

	if sourceText == "" && len(fetched.AudioBytes) > 0 {
		transcribed, err := e.stt.Transcribe(ctx, fetched.AudioBytes, post.Language)
		if err != nil {
			return "", "", "", fmt.Errorf("STT (%s): %w", e.stt.Name(), err)
		}
		sourceText = strings.TrimSpace(transcribed)
	}
	if sourceText == "" {
		// Chạy lại Bài Post đã fetch trước đó — dùng text đã lưu.
		sourceText = strings.TrimSpace(deref(post.ExtractedText))
	}
	if sourceText == "" {
		return "", "", "", domain.Permanent(domain.ErrNoTextExtracted)
	}

	return e.rewriteIfNeeded(ctx, post.PromptID, mode, sourceText, llmSetID, owner)
}

// rewriteIfNeeded là bước cuối chung cho mọi nguồn text: mode B đọc nguyên
// văn, mode C đưa qua LLM theo Prompt mẫu trước.
//
// Tách riêng vì text giờ tới từ 2 đường (Bài Post và Voice gõ tay) nhưng luật
// "mode C thì viết lại" chỉ được có một bản cài đặt.
// Trả về thêm MODEL thật đã viết lại, để lưu lên voice.llm_model_used: cùng
// một bộ API, hôm nay chạy model rẻ nhất, mai hết quota thì chạy mắt xích sau —
// không ghi lại thì không đối chiếu được chất lượng hay chi phí của voice đó.
func (e *Engine) rewriteIfNeeded(
	ctx context.Context,
	promptID *uuid.UUID,
	mode domain.CollectMode,
	sourceText string,
	llmSetID *uuid.UUID,
	owner uuid.UUID,
) (string, string, string, error) {
	if mode != domain.ModePromptToVoice {
		return sourceText, sourceText, "", nil
	}

	if promptID == nil {
		return "", "", "", domain.Permanent(domain.ErrPromptRequired)
	}
	prompt, err := e.q.GetPrompt(ctx, *promptID)
	if err != nil {
		return "", "", "", domain.Permanent(wrapNotFound(err, "prompt "+promptID.String()))
	}

	res, err := e.llm.Generate(ctx, llmSetID, owner, prompt.Content, sourceText)
	if err != nil {
		return "", "", "", fmt.Errorf("LLM: %w", err)
	}
	generated := strings.TrimSpace(res.Text)
	if generated == "" {
		return "", "", "", fmt.Errorf("LLM trả về nội dung rỗng")
	}
	return sourceText, generated, res.Model, nil
}

// ttsFor chọn API key TTS dùng cho voice này: key của chính người tạo voice.
//
// Vì sao theo người tạo mà không phải một key chung: mỗi người tự khai key của
// mình ở màn AI Engine, nên quota và hoá đơn 3voices rơi đúng vào người dùng
// nó. Admin xem được key của mọi người nhưng vẫn chạy bằng key của mình.
//
// Chưa khai key thì rơi về provider cấu hình trong .env — thực tế chỉ có ở dev
// (TTS_PROVIDER=mock). Production không đặt key chung, nên không có key nghĩa
// là lỗi vĩnh viễn kèm câu hướng dẫn, chờ người dùng khai key rồi chạy lại.
func (e *Engine) ttsFor(
	ctx context.Context,
	owner uuid.UUID,
	language string,
) (domain.TTSProvider, *repository.AiEngine, error) {
	engine, err := e.q.GetAIEngineForUser(ctx, owner)
	if errors.Is(err, pgx.ErrNoRows) {
		if e.tts == nil {
			return nil, nil, domain.Permanent(domain.Explain(
				"Bạn chưa khai API key TTS — vào mục AI Engine thêm key 3voices rồi chạy lại",
				fmt.Errorf("%w: user %s chưa có ai_engine", domain.ErrInvalidInput, owner)))
		}
		e.log.WarnContext(ctx, "user chưa khai API key TTS, dùng provider mặc định trong .env",
			"user_id", owner, "provider", e.tts.Name())
		return e.tts, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("lấy ai_engine của user %s: %w", owner, err)
	}

	apiKey, err := e.box.Decrypt(engine.ApiKeyEncrypted)
	if err != nil {
		return nil, nil, domain.Permanent(domain.Explain(
			"Không giải mã được API key TTS — khai lại key ở mục AI Engine",
			fmt.Errorf("giải mã api key ai_engine %s: %w", engine.ID, err)))
	}

	provider, err := e.ttsFactory.For(domain.TTSCredential{
		Provider: engine.Provider,
		APIKey:   apiKey,
		VoiceID:  deref(engine.VoiceID),
	})
	if err != nil {
		return nil, nil, domain.Permanent(err)
	}

	return provider, &engine, nil
}

// speechLanguage chốt mã ngôn ngữ gửi cho TTS, và quyết định khi nào thì từ
// chối đọc.
//
// Điểm mấu chốt là PHÂN BIỆT ai chọn ngôn ngữ đó:
//
//   - Người dùng tự chọn mà provider không đọc được -> từ chối, và nói rõ
//     tiếng gì cùng danh sách đọc được. Đọc bằng tiếng khác là làm sai ý họ.
//   - Hệ thống TỰ nhận diện từ nền tảng (bài YouTube tiếng Nga chẳng hạn) ->
//     KHÔNG từ chối. Người dùng chỉ yêu cầu "đọc bài này", họ không chọn tiếng
//     Nga; chặn ở đây biến một suy đoán của hệ thống thành lỗi cứng của họ.
//     Gửi chuỗi rỗng để 3voices tự nhận diện từ chính nội dung.
//
// Trả về chuỗi rỗng nghĩa là "để provider tự quyết".
func (e *Engine) speechLanguage(
	ctx context.Context,
	provider domain.TTSProvider,
	language string,
	autoDetected bool,
) (string, error) {
	if domain.IsAutoLanguage(language) {
		return "", nil
	}
	if languageSupported(language, provider.SupportedLanguages()) {
		return language, nil
	}
	if autoDetected {
		e.log.WarnContext(ctx, "ngôn ngữ tự nhận diện không nằm trong danh sách của TTS, để provider tự xử",
			"language", language, "tts", provider.Name())
		return "", nil
	}
	return "", domain.Permanent(domain.Explain(
		fmt.Sprintf("%s không đọc được tiếng %q — chọn ngôn ngữ khác (%s) hoặc để Tự nhận diện",
			provider.Name(), language, strings.Join(provider.SupportedLanguages(), ", ")),
		fmt.Errorf("%w: %s không đọc được %s",
			domain.ErrLangUnsupported, provider.Name(), language)))
}

// isTextPost: Bài Post nhập tay bằng text (không có URL nguồn, không adapter).
func isTextPost(post repository.SourcePost) bool {
	return post.Platform == string(domain.PlatformText)
}

// textContent dựng nội dung cho Bài Post nhập tay: text người dùng gõ đã nằm
// sẵn ở extracted_text từ lúc tạo bài, không phải gọi mạng lần nào.
func textContent(post repository.SourcePost) (domain.FetchedContent, error) {
	text := domain.NormalizeTTSText(deref(post.ExtractedText))
	if text == "" {
		return domain.FetchedContent{}, domain.Permanent(fmt.Errorf(
			"%w: bài nhập tay không còn nội dung text", domain.ErrNoTextExtracted))
	}
	return domain.FetchedContent{
		ContentType: domain.ContentPost,
		Text:        text,
		Meta:        domain.TextPostMetadata(text),
	}, nil
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
// voice:text
// ---------------------------------------------------------------------------

// ProcessTextVoice đọc lời đọc đã chốt trên chính record Voice (`input_text`).
//
// Hai trường hợp dùng nó: Voice gõ tay (không có Bài Post nào đứng sau), và
// Voice được người dùng sửa lời đọc rồi bấm tạo lại — kể cả Voice vốn sinh ra
// từ Bài Post. Cả hai đều không fetch gì: nội dung đã nằm sẵn, đường đi chỉ còn
// (LLM nếu mode C) -> TTS -> storage. Mọi bước dùng chung helper với luồng Bài
// Post, kể cả luật "mode C thì viết lại" và cách chọn API key TTS.
func (e *Engine) ProcessTextVoice(ctx context.Context, voiceID, actor uuid.UUID) error {
	// Claim để idempotent khi Asynq retry hoặc 2 worker cùng nhận task; voice
	// đã ra file rồi thì không đọc đè lên.
	voice, err := e.q.ClaimVoiceForProcessing(ctx, voiceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			e.log.InfoContext(ctx, "voice:text bỏ qua — voice không ở trạng thái chạy được",
				"voice_id", voiceID)
			return nil
		}
		return fmt.Errorf("claim voice %s: %w", voiceID, err)
	}

	if err := e.buildTextVoice(ctx, voice, actor); err != nil {
		msg := domain.UserMessage(err)
		e.log.ErrorContext(ctx, "voice:text thất bại", "error", err, "voice_id", voiceID)
		if _, serr := e.q.SetVoicePublishStatus(ctx, repository.SetVoicePublishStatusParams{
			ID:            voiceID,
			PublishStatus: domain.PublishFailed,
			LastError:     &msg,
		}); serr != nil {
			e.log.ErrorContext(ctx, "không cập nhật được voice failed", "error", serr, "voice_id", voiceID)
		}
		return err
	}

	e.audit.Record(ctx, actor, domain.AuditCreate, domain.ObjectVoice, voiceID, map[string]any{
		"collect_mode": deref(voice.CollectMode),
		"language":     voice.Language,
		"source":       "text",
	})
	return nil
}

func (e *Engine) buildTextVoice(
	ctx context.Context,
	voice repository.Voice,
	actor uuid.UUID,
) error {
	mode := domain.CollectMode(deref(voice.CollectMode))
	if !mode.NeedsTTS() {
		return domain.Permanent(fmt.Errorf(
			"%w: voice đọc từ text chỉ chạy được hình thức B hoặc C (đang là %q)",
			domain.ErrInvalidInput, deref(voice.CollectMode)))
	}

	sourceText := domain.NormalizeTTSText(deref(voice.InputText))
	if sourceText == "" {
		return domain.Permanent(domain.Explain(
			"Voice này không còn nội dung text để đọc",
			fmt.Errorf("%w: voice %s có input_text rỗng", domain.ErrNoTextExtracted, voice.ID)))
	}

	_, spoken, llmModel, err := e.rewriteIfNeeded(
		ctx, voice.PromptID, mode, sourceText, voice.LlmApiSetID, actor)
	if err != nil {
		return err
	}

	provider, engine, err := e.ttsFor(ctx, actor, voice.Language)
	if err != nil {
		return err
	}
	var engineID *uuid.UUID
	if engine != nil {
		engineID = &engine.ID
	}

	// Voice gõ tay: ngôn ngữ là do người dùng chọn ở form, không phải hệ thống
	// đoán -> chọn tiếng provider không đọc được thì báo lỗi thẳng.
	ttsLang, err := e.speechLanguage(ctx, provider, voice.Language, false)
	if err != nil {
		return err
	}
	audio, err := provider.Synthesize(ctx, spoken, ttsLang)
	if err != nil {
		return fmt.Errorf("TTS (%s): %w", provider.Name(), err)
	}
	if engineID != nil {
		if err := e.q.TouchAIEngineUsed(ctx, *engineID); err != nil {
			e.log.WarnContext(ctx, "không ghi được last_used_at của API key",
				"error", err, "ai_engine_id", *engineID)
		}
	}

	info := e.probe(ctx, audio, voice.ID)

	key := fmt.Sprintf("voices/text/%s-%d.mp3", voice.ID, time.Now().Unix())
	fileURL, err := e.storage.Put(ctx, key, audio, info.MimeType)
	if err != nil {
		return fmt.Errorf("lưu file voice lên storage: %w", err)
	}

	// Tiêu đề: chỉ đặt khi voice CHƯA có tiêu đề nào.
	//
	// Voice tạo lại thì tiêu đề là thứ người dùng đã sửa tay ở tab Thông tin và
	// sẽ hiện trên multime — đọc lại lời khác không phải lý do để xoá nó đi.
	// nil ở đây nghĩa là giữ nguyên (query FinishVoice dùng COALESCE).
	var title *string
	if strings.TrimSpace(deref(voice.Title)) == "" {
		title = nilIfEmpty(domain.VoiceTitle(spoken))
	}

	if _, err := e.q.FinishVoice(ctx, repository.FinishVoiceParams{
		ID:              voice.ID,
		AiEngineID:      engineID,
		VoiceFileUrl:    &fileURL,
		DurationSeconds: nilIfZero(int32(info.DurationSeconds)),
		Title:           title,
		Language:        voice.Language,
		MimeType:        nilIfEmpty(info.MimeType),
		SizeBytes:       ptr(info.SizeBytes),
		SampleRate:      nilIfZero(int32(info.SampleRate)),
		LlmModelUsed:    nilIfEmpty(llmModel),
	}); err != nil {
		e.cleanupOrphan(ctx, key)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Permanent(fmt.Errorf(
				"%w: voice %s đã bị xoá trong lúc xử lý", domain.ErrNotFound, voice.ID))
		}
		return fmt.Errorf("cập nhật voice: %w", err)
	}

	// Tạo lại voice: file cũ không còn ai trỏ tới nữa. Xoá SAU khi DB đã ghi
	// file mới — xoá trước mà ghi hỏng thì voice mất cả file lẫn đường quay lại.
	if old := deref(voice.VoiceFileUrl); old != "" && old != fileURL {
		if oldKey := e.storage.KeyFromURL(old); oldKey != "" && oldKey != key {
			if err := e.storage.Delete(ctx, oldKey); err != nil {
				e.log.WarnContext(ctx, "không xoá được file voice cũ sau khi tạo lại",
					"error", err, "voice_id", voice.ID, "key", oldKey)
			}
		}
	}
	return nil
}

// probe đo metadata kỹ thuật của audio — multime.ai cần duration/size/mime/
// sample_rate để tạo audio asset. ffprobe hỏng thì chỉ mất metadata, không
// chặn luồng.
func (e *Engine) probe(ctx context.Context, audio []byte, voiceID uuid.UUID) domain.AudioInfo {
	info := domain.AudioInfo{SizeBytes: int64(len(audio)), MimeType: "audio/mpeg"}
	if e.prober != nil {
		probed, err := e.prober.Probe(ctx, audio)
		if err != nil {
			e.log.WarnContext(ctx, "không đo được metadata audio", "error", err, "voice_id", voiceID)
		} else {
			info = probed
		}
	}
	if info.MimeType == "" || info.MimeType == "application/octet-stream" {
		info.MimeType = "audio/mpeg"
	}
	return info
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
	// Bốc tài khoản đứng tên bài đăng NGAY TRƯỚC khi đăng, không phải lúc người
	// dùng chọn giới tính trên form.
	//
	// Vì sao lùi tới đây: chọn giới tính là một thao tác của form, còn bốc là
	// một lần gọi sang Strongbody. Gộp hai thứ khiến mỗi lần đổi ý về giới tính
	// là một lần gọi mạng và một lần chờ, trong khi kết quả bốc chỉ có ý nghĩa ở
	// đúng thời điểm đăng — bốc sớm rồi voice nằm trong hàng đợi vài phút thì
	// tài khoản đó cũng chẳng "giữ chỗ" được gì bên Strongbody.
	voice, err := e.ensureAuthor(ctx, voice)
	if err != nil {
		return err
	}

	key := e.storage.KeyFromURL(*voice.VoiceFileUrl)
	audio, err := e.storage.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("đọc file voice %s: %w", key, err)
	}

	// Ảnh bìa: API nhận file, không nhận URL — phải có sẵn bytes trước.
	var imageBytes []byte
	var imageName string
	if url := deref(voice.ImageUrl); url != "" {
		imageBytes, imageName, err = e.coverImage(ctx, voice)
		if err != nil {
			// Thiếu ảnh bìa không đáng để chặn việc đăng.
			e.log.WarnContext(ctx, "không lấy được ảnh bìa, đăng không kèm ảnh",
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
		// AuthorID do người dùng chọn (bắt buộc) — bài lên multime dưới tên tài
		// khoản đó, không phải tài khoản của người bấm nút đăng.
		AuthorID: deref(voice.AuthorID),
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

	// Ảnh bìa tải từ máy: multime đã giữ một bản trong bài đăng, bản trong
	// bucket của mình không còn ai đọc -> xoá cho đỡ tốn dung lượng (câu lệnh
	// MarkVoicePublished ở trên đã gỡ link).
	if voice.ImageUploaded && voice.ImageUrl != nil {
		if imgKey := e.storage.KeyFromURL(*voice.ImageUrl); imgKey != "" {
			if err := e.storage.Delete(ctx, imgKey); err != nil {
				e.log.WarnContext(ctx, "đã publish nhưng không xoá được ảnh bìa",
					"error", err, "voice_id", voice.ID, "key", imgKey)
			}
		}
	}
	return nil
}

// publishAs đăng voice bằng credential của user đã tạo ra nó — voice xuất hiện
// trên multime dưới đúng tài khoản đó, không phải một tài khoản hệ thống dùng chung.
//
// Access token hết hạn thì refresh 1 lần rồi thử lại; refresh cũng thất bại thì
// trả lỗi vĩnh viễn yêu cầu user đăng nhập lại (không có cách tự khắc phục).
// ensureAuthor bốc tài khoản đứng tên bài nếu voice mới chỉ có giới tính.
//
// Voice đã có author_id (người dùng bốc tay ở bảng Voice) thì giữ nguyên: đó là
// lựa chọn tường minh của họ, bốc đè lên là làm sai ý.
func (e *Engine) ensureAuthor(
	ctx context.Context,
	voice repository.Voice,
) (repository.Voice, error) {
	if voice.AuthorID != nil && *voice.AuthorID > 0 {
		return voice, nil
	}

	gender := domain.Gender(deref(voice.AuthorGender))
	if !gender.Valid() {
		return voice, domain.Permanent(domain.Explain(
			"Voice chưa chọn giới tính tài khoản đứng tên bài đăng (author)",
			fmt.Errorf("%w: voice %s không có author_id lẫn author_gender",
				domain.ErrInvalidInput, voice.ID)))
	}
	if e.authors == nil {
		return voice, domain.Permanent(fmt.Errorf(
			"%w: engine chưa được cấu hình danh bạ author", domain.ErrInvalidInput))
	}

	// Bốc bằng token của NGƯỜI TẠO VOICE, giống hệt bước đăng: quyền xem danh bạ
	// là quyền Strongbody cấp cho tài khoản đó, tool không mượn quyền của ai.
	user, err := e.authors.Random(ctx, voice.CreatedBy, gender, deref(voice.AuthorCountryID))
	if err != nil {
		return voice, fmt.Errorf("bốc tài khoản đứng tên bài đăng: %w", err)
	}

	// Ghi lại ngay: đăng lỗi rồi retry thì dùng đúng tài khoản đã bốc, không bốc
	// ra người khác ở lần thử thứ hai.
	if err := e.q.SetVoiceAuthor(ctx, repository.SetVoiceAuthorParams{
		ID: voice.ID, AuthorID: &user.ID, AuthorEmail: nilIfEmpty(user.Email),
	}); err != nil {
		return voice, fmt.Errorf("lưu tài khoản đã bốc cho voice %s: %w", voice.ID, err)
	}

	e.log.InfoContext(ctx, "đã bốc tài khoản đứng tên bài đăng",
		"voice_id", voice.ID, "author_id", user.ID, "gender", gender)

	voice.AuthorID = &user.ID
	voice.AuthorEmail = nilIfEmpty(user.Email)
	return voice, nil
}

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

// coverImage lấy bytes ảnh bìa để đính kèm vào request publish.
//
// Hai nguồn, hai đường đọc: ảnh người dùng tải từ máy nằm trong bucket RIÊNG TƯ
// của tool — tải qua HTTP sẽ bị từ chối, phải đọc thẳng bằng storage client.
// Ảnh lấy từ bài gốc là URL công khai của nền tảng khác nên phải tải về.
func (e *Engine) coverImage(ctx context.Context, voice repository.Voice) ([]byte, string, error) {
	url := deref(voice.ImageUrl)
	if !voice.ImageUploaded {
		return e.fetchImage(ctx, url)
	}

	key := e.storage.KeyFromURL(url)
	if key == "" {
		return nil, "", fmt.Errorf("không đọc được key ảnh bìa từ %s", url)
	}
	data, err := e.storage.Get(ctx, key)
	if err != nil {
		return nil, "", fmt.Errorf("đọc ảnh bìa %s: %w", key, err)
	}
	return data, path.Base(key), nil
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
