package handler

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/strongbody/voice-tool/backend/internal/domain"

	"github.com/strongbody/voice-tool/backend/internal/pkg/httpx"
	"github.com/strongbody/voice-tool/backend/internal/repository"
	"github.com/strongbody/voice-tool/backend/internal/service"
	"github.com/strongbody/voice-tool/backend/internal/transport/http/middleware"
)

type Voice struct {
	svc *service.Voice
}

func NewVoice(svc *service.Voice) *Voice { return &Voice{svc: svc} }

// createVoiceRequest — tạo Voice thẳng từ text gõ tay.
//
// Đây là đường DUY NHẤT tạo Voice không qua Bài Post: bài lấy từ URL vẫn phải
// đi qua `POST /source-posts` rồi `/source-posts/:id/run` (business rule #1),
// vì chỉ khi đó Voice mới truy vết được về bài gốc. Text gõ tay không có bài
// gốc nào để truy vết nên Bài Post ở giữa chỉ là bản ghi rỗng.
type createVoiceRequest struct {
	Text string `json:"text" binding:"required"`
	// CollectMode: B (đọc nguyên văn) hoặc C (LLM viết lại theo Prompt rồi
	// đọc). Không nhận A — gõ text thì không có audio gốc để tách.
	CollectMode string     `json:"collect_mode" binding:"required,oneof=B C"`
	PromptID    *uuid.UUID `json:"prompt_id"`
	Language    string     `json:"language"`
	// LLMAPISetID: Bộ API key viết lại nội dung — chỉ hình thức C mới cần.
	LLMAPISetID *uuid.UUID `json:"llm_api_set_id"`
}

// Create tạo Voice từ text và đưa vào hàng đợi đọc luôn — trả về record
// `processing` để UI hiện dòng voice đang chạy ngay.
func (h *Voice) Create(c *gin.Context) {
	var req createVoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, err)
		return
	}
	voice, err := h.svc.CreateFromText(c.Request.Context(), middleware.ActorID(c), service.TextVoiceInput{
		Text:        req.Text,
		CollectMode: domain.CollectMode(req.CollectMode),
		PromptID:    req.PromptID,
		Language:    req.Language,
		LLMAPISetID: req.LLMAPISetID,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, voice)
}

func (h *Voice) List(c *gin.Context) {
	sourcePostID, err := queryUUID(c, "source_post_id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	createdBy, err := queryUUID(c, "created_by")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	createdFrom, err := queryTime(c, "created_from", false)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	createdTo, err := queryTime(c, "created_to", true)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	publishedFrom, err := queryTime(c, "published_from", false)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	publishedTo, err := queryTime(c, "published_to", true)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	limit, offset := pagination(c)
	sortCol, dir := sorting(c)

	items, total, err := h.svc.List(c.Request.Context(), service.VoiceFilter{
		PublishStatus: queryString(c, "publish_status"),
		SourcePostID:  sourcePostID,
		Platform:      queryString(c, "platform"),
		Language:      queryString(c, "language"),
		Sort:          sortCol,
		Dir:           dir,
		CreatedBy:     createdBy,
		CreatedFrom:   createdFrom,
		CreatedTo:     createdTo,
		PublishedFrom: publishedFrom,
		PublishedTo:   publishedTo,
		Limit:         limit,
		Offset:        offset,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, httpx.Page[repository.ListVoicesRow]{
		Items: items, Total: total, Limit: limit, Offset: offset,
	})
}

// Audio trả file voice để nghe thử / tải về trước khi đăng (prompt.md mục 2).
// File nằm trong bucket riêng tư nên bắt buộc đi qua API, không lộ URL storage.
//
// Trả bằng http.ServeContent, KHÔNG phải c.Data: nó tự xử lý header Range và
// phát ra `Accept-Ranges: bytes`. Thiếu cái này thì thanh phát của trình duyệt
// không tua được — nó phải xin đúng byte offset mới nhảy tới giữa file.
func (h *Voice) Audio(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	file, err := h.svc.Audio(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}

	disposition := "inline"
	if c.Query("download") != "" {
		disposition = "attachment"
	}
	c.Header("Content-Disposition",
		mime.FormatMediaType(disposition, map[string]string{"filename": file.FileName}))
	c.Header("Cache-Control", "private, max-age=300")
	// ServeContent chỉ tự đoán Content-Type khi tên file có đuôi; ở đây đặt sẵn
	// theo mime_type lưu trong DB nên truyền tên rỗng.
	c.Header("Content-Type", file.MimeType)
	http.ServeContent(c.Writer, c.Request, "", time.Time{}, bytes.NewReader(file.Data))
}

func (h *Voice) Get(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	voice, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, voice)
}

type updateVoiceRequest struct {
	Title    *string `json:"title"`
	Hashtag  *string `json:"hashtag"`
	Language *string `json:"language"`
	ImageURL *string `json:"image_url"`
	// AuthorID là tài khoản Strongbody đứng tên bài đăng (bắt buộc trước khi
	// đăng); AuthorEmail/AuthorGender đi kèm chỉ để bảng Voice hiện lại được
	// "Female - solr@example.com" mà không phải hỏi Strongbody từng dòng.
	AuthorID     *int64  `json:"author_id"`
	AuthorEmail  *string `json:"author_email"`
	AuthorGender *string `json:"author_gender"`
	// AuthorCountryID: đổi quốc gia lọc author. Đổi nó (hoặc đổi giới tính) mà
	// không kèm author_id thì tài khoản đã bốc trước đó bị bỏ, để bước đăng bốc lại.
	AuthorCountryID *int64 `json:"author_country_id"`
}

func (h *Voice) Update(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req updateVoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, err)
		return
	}

	voice, err := h.svc.UpdateMetadata(c.Request.Context(), middleware.ActorID(c), id,
		service.UpdateMetadataInput{
			Title:           req.Title,
			Hashtag:         req.Hashtag,
			Language:        req.Language,
			ImageURL:        req.ImageURL,
			AuthorID:        req.AuthorID,
			AuthorEmail:     req.AuthorEmail,
			AuthorGender:    req.AuthorGender,
			AuthorCountryID: req.AuthorCountryID,
		})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, voice)
}

// regenerateVoiceRequest — sửa lời đọc rồi tạo lại chính Voice đó (ghi đè file
// cũ). Dùng cho cả Voice gõ tay lẫn Voice sinh từ Bài Post: cái người dùng sửa
// là NỘI DUNG SẼ ĐỌC, không phải bài gốc trên nền tảng.
type regenerateVoiceRequest struct {
	Text        string     `json:"text" binding:"required"`
	CollectMode string     `json:"collect_mode" binding:"required,oneof=B C"`
	PromptID    *uuid.UUID `json:"prompt_id"`
	Language    string     `json:"language"`
	// LLMAPISetID rỗng = giữ bộ API voice đang dùng.
	LLMAPISetID *uuid.UUID `json:"llm_api_set_id"`
}

func (h *Voice) Regenerate(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req regenerateVoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, err)
		return
	}
	voice, err := h.svc.Regenerate(c.Request.Context(), middleware.ActorID(c), id,
		service.RegenerateInput{
			Text:        req.Text,
			CollectMode: domain.CollectMode(req.CollectMode),
			PromptID:    req.PromptID,
			Language:    req.Language,
			LLMAPISetID: req.LLMAPISetID,
		})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusAccepted, voice)
}

// maxImageUploadBytes là trần của request tải ảnh bìa — hơn hẳn giới hạn ảnh
// thật (8MB) để phần bao multipart không làm ảnh đúng cỡ bị chặn oan.
const maxImageUploadBytes = 12 << 20

// UploadImage nhận ảnh bìa tải từ máy (multipart, field `file`).
//
// Không dùng JSON base64: ảnh vài MB qua base64 phình thêm 33% và phải nằm gọn
// trong bộ nhớ ở cả 2 đầu, trong khi multipart là thứ trình duyệt gửi sẵn.
func (h *Voice) UploadImage(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}

	data, mimeType, err := readImageUpload(c)
	if err != nil {
		httpx.BadRequest(c, err)
		return
	}

	voice, err := h.svc.SetImage(c.Request.Context(), middleware.ActorID(c), id, service.ImageInput{
		Data:     data,
		MimeType: mimeType,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, voice)
}

// UploadPendingImage nhận ảnh bìa cho voice CHƯA tồn tại (màn tạo Voice).
//
// Trả về URL để client gửi kèm khi tạo Bài Post; ảnh được gắn vào voice sinh ra
// và bị xoá khỏi storage sau khi đăng, giống ảnh tải lên từ màn sửa.
func (h *Voice) UploadPendingImage(c *gin.Context) {
	data, mimeType, err := readImageUpload(c)
	if err != nil {
		httpx.BadRequest(c, err)
		return
	}

	url, err := h.svc.UploadImage(c.Request.Context(), middleware.ActorID(c), service.ImageInput{
		Data:     data,
		MimeType: mimeType,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, gin.H{"image_url": url, "image_uploaded": true})
}

// readImageUpload đọc file ảnh từ request multipart (field `file`).
func readImageUpload(c *gin.Context) ([]byte, string, error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxImageUploadBytes)
	header, err := c.FormFile("file")
	if err != nil {
		return nil, "", fmt.Errorf("cần file ảnh ở field `file`: %w", err)
	}

	file, err := header.Open()
	if err != nil {
		return nil, "", err
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxImageUploadBytes))
	if err != nil {
		return nil, "", err
	}
	// Content-Type của trình duyệt có thể rỗng/sai (vd application/octet-stream
	// khi kéo thả) -> tự đoán lại từ chính nội dung file.
	return data, http.DetectContentType(data), nil
}

// CoverImage trả ảnh bìa đã tải lên để thẻ <img> hiển thị được.
//
// Cùng lý do với Audio: bucket riêng tư và host storage chỉ tồn tại trong mạng
// nội bộ, nên ảnh phải đi qua API. Thẻ <img> cũng không gắn được header
// Authorization -> route này nhận token qua query param (middleware.AuthMedia).
func (h *Voice) CoverImage(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	file, err := h.svc.CoverImage(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}

	// Ảnh không đổi nội dung trong 1 lần tải lên (đổi ảnh là ra key mới), nên
	// cache được — FE gắn thêm tham số phiên bản để lần tải mới hiện ngay.
	c.Header("Cache-Control", "private, max-age=300")
	c.Data(http.StatusOK, file.MimeType, file.Data)
}

func (h *Voice) Delete(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.Delete(c.Request.Context(), middleware.ActorID(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

func (h *Voice) Ready(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	voice, err := h.svc.MarkReady(c.Request.Context(), middleware.ActorID(c), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, voice)
}

func (h *Voice) Publish(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.Publish(c.Request.Context(), middleware.ActorID(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "queued", "voice_id": id})
}
