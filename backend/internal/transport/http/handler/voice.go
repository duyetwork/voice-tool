package handler

import (
	"mime"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/strongbody/voice-tool/backend/internal/pkg/httpx"
	"github.com/strongbody/voice-tool/backend/internal/repository"
	"github.com/strongbody/voice-tool/backend/internal/service"
	"github.com/strongbody/voice-tool/backend/internal/transport/http/middleware"
)

type Voice struct {
	svc *service.Voice
}

func NewVoice(svc *service.Voice) *Voice { return &Voice{svc: svc} }

// Không có POST /voices: Voice chỉ sinh ra từ việc chạy Bài Post
// (business rule #1) — dùng POST /source-posts/:id/run.

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

	items, total, err := h.svc.List(c.Request.Context(), service.VoiceFilter{
		PublishStatus: queryString(c, "publish_status"),
		SourcePostID:  sourcePostID,
		Platform:      queryString(c, "platform"),
		Language:      queryString(c, "language"),
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
	c.Data(http.StatusOK, file.MimeType, file.Data)
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
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Hashtag     *string `json:"hashtag"`
	Language    *string `json:"language"`
	ImageURL    *string `json:"image_url"`
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
			Title:       req.Title,
			Description: req.Description,
			Hashtag:     req.Hashtag,
			Language:    req.Language,
			ImageURL:    req.ImageURL,
		})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, voice)
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
