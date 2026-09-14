// Package http dựng Gin router và gắn middleware.
package http

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"github.com/strongbody/voice-tool/backend/internal/config"
	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/pkg/jwt"
	"github.com/strongbody/voice-tool/backend/internal/service"
	"github.com/strongbody/voice-tool/backend/internal/transport/http/handler"
	"github.com/strongbody/voice-tool/backend/internal/transport/http/middleware"
)

type RouterDeps struct {
	Config     *config.Config
	Logger     *slog.Logger
	Tokens     *jwt.Manager
	Auth       *service.Auth
	SourcePost *service.SourcePost
	Voice      *service.Voice
	List       *service.List
	Catalog    *service.Catalog
	AIEngine   *service.AIEngineService
	// LLMSets: Bộ API key LLM (tab "LLM Model"). Settings/FetchStats là cấu
	// hình chung — chỉ admin.
	LLMSets    *service.LLMAPISetService
	Settings   *service.Settings
	FetchStats *service.FetchStats
	Audit      *service.Audit
	User       *service.User
	// MultimeUsers tra danh bạ tài khoản Strongbody (ô chọn tác giả).
	MultimeUsers *service.MultimeUsers
	// CatalogCache: danh mục quốc gia/hashtag đã lưu trong DB cho modal Tạo
	// Voice. Khác `Catalog` ở trên — cái đó là CRUD Prompt mẫu.
	CatalogCache *service.CatalogCache
	Platforms    interface{ Supported() []domain.Platform }
	// Modes: hình thức thu thập đang bật + lý do cái còn lại bị tắt.
	Modes service.ModeGate
}

// NewRouter gắn toàn bộ route.
//
// Phân quyền (specs bổ sung, câu 9/10):
//   - user:   đọc mọi thứ + tạo/sửa/chạy/ĐĂNG voice, KHÔNG được xoá
//   - editor: user + xoá — tức toàn quyền nghiệp vụ, TRỪ quản lý tài khoản
//   - admin:  toàn quyền, kể cả quản lý tài khoản
//
// Không có endpoint đăng ký: đăng nhập bằng tài khoản strongbody/multime.
func NewRouter(d RouterDeps) *gin.Engine {
	if d.Config.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(
		middleware.RequestID(),
		middleware.Logger(d.Logger),
		middleware.Recovery(d.Logger),
		cors.New(cors.Config{
			AllowAllOrigins:  true,
			AllowMethods:     []string{"GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"},
			AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-Request-ID"},
			ExposeHeaders:    []string{"X-Request-ID"},
			AllowCredentials: false,
			MaxAge:           12 * time.Hour,
		}),
	)

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	api := r.Group("/api/v1")

	// Public: chỉ đăng nhập + refresh (không có đăng ký).
	handler.NewAuth(d.Auth).Register(api)

	// Đã đăng nhập — mọi role đọc được.
	authed := api.Group("", middleware.Auth(d.Tokens))

	authed.GET("/me", func(c *gin.Context) {
		role := middleware.ActorRole(c)
		c.JSON(http.StatusOK, gin.H{
			"id":   middleware.ActorID(c),
			"role": role,
			"permissions": gin.H{
				"can_write":        role.CanWrite(),
				"can_delete":       role.CanDelete(),
				"can_manage_users": role.CanManageUsers(),
			},
		})
	})
	// FE đọc đây để biết nền tảng nào nhận diện được và hình thức thu thập nào
	// đang bật (B/C chưa hỗ trợ thì hiển thị mờ).
	authed.GET("/meta/platforms", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"platforms": d.Platforms.Supported()})
	})
	// Điều kiện multime.ai đòi hỏi ở 1 bài đăng — FE dùng để biết khi nào được
	// phép bấm Đăng. Không còn hashtag mặc định: mỗi voice phải có hashtag của
	// riêng nó, bỏ trống là không đăng được.
	authed.GET("/meta/publish", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"category_ids":         d.Config.MultimeCategoryIDs(),
			"min_duration_seconds": domain.MinPublishDurationSeconds,
		})
	})
	// Bốc ngẫu nhiên tác giả bài đăng theo giới tính. Đọc trực tiếp từ
	// Strongbody bằng token của người đang đăng nhập — tool không giữ bản sao.
	directory := handler.NewMultimeUsers(d.MultimeUsers, d.CatalogCache)
	authed.GET("/meta/authors/random", directory.Random)
	// Danh mục quốc gia để lọc author theo quốc gia.
	authed.GET("/meta/countries", directory.Countries)
	// Danh mục cho modal Tạo Voice: quốc gia + hashtag + thứ tự ngôn ngữ, 1 lần
	// gọi rồi frontend cache lại — mở modal không gọi API nữa.
	authed.GET("/meta/catalog", directory.Catalog)
	// Tìm hashtag trong danh mục đã lưu — danh mục MultiMe có ~94.000 mục nên
	// không tải hết về trình duyệt được. Lọc chạy trong DB của tool, không gọi
	// sang MultiMe theo từng phím gõ.
	authed.GET("/meta/hashtags", directory.Hashtags)
	// Kèm `reason` khi mode bị tắt: FE hiện đúng lý do (thiếu ANTHROPIC_API_KEY,
	// hay người vận hành tự tắt) thay vì mỗi chữ "chưa hỗ trợ".
	authed.GET("/meta/collect-modes", func(c *gin.Context) {
		modes := make([]gin.H, 0, len(domain.AllCollectModes))
		for _, mode := range domain.AllCollectModes {
			item := gin.H{"mode": mode, "enabled": d.Modes.Allows(mode)}
			if why := d.Modes.Why(mode); why != "" {
				item["reason"] = why
			}
			modes = append(modes, item)
		}
		c.JSON(http.StatusOK, gin.H{"collect_modes": modes})
	})

	registerReadOnly(authed, d)

	// Phát/tải file voice + ảnh bìa: token đi qua query param được vì thẻ
	// <audio>/<img> không gắn được header Authorization.
	media := handler.NewVoice(d.Voice)
	api.GET("/voices/:id/audio", middleware.AuthMedia(d.Tokens), media.Audio)
	api.GET("/voices/:id/image", middleware.AuthMedia(d.Tokens), media.CoverImage)

	// user + admin: tạo/sửa/chạy/đăng.
	writer := api.Group("", middleware.Auth(d.Tokens), middleware.RequireWrite())
	registerWrite(writer, d)

	// Chỉ admin: xoá dữ liệu.
	remover := api.Group("", middleware.Auth(d.Tokens), middleware.RequireDelete())
	registerDelete(remover, d)

	// admin: quản lý tài khoản + cấu hình chung.
	admin := api.Group("", middleware.Auth(d.Tokens), middleware.RequireAdmin())
	handler.NewUser(d.User).Register(admin)

	// Cài đặt: chuỗi dự phòng LLM + batch. Chặn bằng middleware được vì đây là
	// cấu hình của cả hệ thống, không có "chủ sở hữu" nào để so như API key.
	settings := handler.NewSettings(d.Settings, d.FetchStats)
	admin.GET("/settings", settings.Get)
	admin.PATCH("/settings", settings.Update)
	// Số lần bị từng nền tảng chặn — dữ liệu để quyết định có cần proxy không.
	admin.GET("/settings/fetch-stats", settings.FetchStats)

	return r
}

// registerReadOnly gắn các route GET — mọi vai trò đều đọc được.
func registerReadOnly(g *gin.RouterGroup, d RouterDeps) {
	sourcePost := handler.NewSourcePost(d.SourcePost)
	voice := handler.NewVoice(d.Voice)
	list := handler.NewList(d.List)
	catalog := handler.NewCatalog(d.Catalog)

	g.GET("/source-posts", sourcePost.List)
	g.GET("/source-posts/:id", sourcePost.Get)

	g.GET("/voices", voice.List)
	g.GET("/voices/:id", voice.Get)

	g.GET("/lists/breaking", list.ListBreaking)
	g.GET("/lists/breaking/:id", list.GetBreaking)
	g.GET("/lists/scheduled", list.ListScheduled)
	g.GET("/lists/scheduled/:id", list.GetScheduled)

	g.GET("/prompts", catalog.ListPrompts)
	g.GET("/prompts/:id", catalog.GetPrompt)
	// API key TTS: ai xem được của ai do service quyết định theo chủ sở hữu,
	// nên route chỉ cần đăng nhập.
	aiEngine := handler.NewAIEngine(d.AIEngine)
	g.GET("/ai-engines", aiEngine.List)
	g.GET("/ai-engines/:id", aiEngine.Get)

	// Bộ API key LLM: ai thấy bộ nào do service quyết định (bộ mình tạo, bộ
	// được chia sẻ, bộ admin đã bật hiển thị), nên route chỉ cần đăng nhập.
	// Danh sách này cũng là nguồn cho ô "Bộ API" ở form tạo voice.
	llmSets := handler.NewLLMAPISet(d.LLMSets)
	g.GET("/llm-api-sets", llmSets.List)
	g.GET("/llm-api-sets/:id", llmSets.Get)

	handler.NewAuditLog(d.Audit).Register(g)
}

// registerWrite gắn các route tạo/sửa/chạy/đăng — cần role user hoặc admin.
func registerWrite(g *gin.RouterGroup, d RouterDeps) {
	sourcePost := handler.NewSourcePost(d.SourcePost)
	voice := handler.NewVoice(d.Voice)
	list := handler.NewList(d.List)
	catalog := handler.NewCatalog(d.Catalog)

	g.POST("/source-posts", sourcePost.Create)
	g.PATCH("/source-posts/:id", sourcePost.Update)
	g.POST("/source-posts/:id/run", sourcePost.Run)

	// POST /voices CHỈ nhận text gõ tay — Voice từ URL vẫn phải đi qua Bài Post
	// (business rule #1). Xem handler.createVoiceRequest.
	g.POST("/voices", voice.Create)
	g.PATCH("/voices/:id", voice.Update)
	// Sửa lời đọc rồi tạo lại chính voice đó (ghi đè file cũ).
	g.POST("/voices/:id/regenerate", voice.Regenerate)
	g.POST("/voices/:id/ready", voice.Ready)
	// Ảnh bìa tải từ máy (multipart) — xoá khỏi storage sau khi đăng.
	g.POST("/voices/:id/image", voice.UploadImage)
	// Ảnh bìa cho voice CHƯA tồn tại: màn tạo Voice điền metadata trước khi có
	// bài post nào, nên phải tải ảnh lên trước rồi gửi kèm URL.
	g.POST("/images", voice.UploadPendingImage)
	g.POST("/voices/:id/publish", voice.Publish)

	g.POST("/lists/breaking", list.CreateBreaking)
	g.PATCH("/lists/breaking/:id", list.UpdateBreaking)
	g.POST("/lists/breaking/:id/run", list.RunBreaking)

	g.POST("/lists/scheduled", list.CreateScheduled)
	g.PATCH("/lists/scheduled/:id", list.UpdateScheduled)

	g.POST("/prompts", catalog.CreatePrompt)
	g.PATCH("/prompts/:id", catalog.UpdatePrompt)

	// Thêm/sửa/xoá API key nằm chung nhóm "write": key là của chính người
	// dùng, không phải dữ liệu chung — user thường phải tự xoá được key mình
	// đã khai (service chặn không cho đụng key của người khác).
	aiEngine := handler.NewAIEngine(d.AIEngine)
	g.POST("/ai-engines", aiEngine.Create)
	g.PATCH("/ai-engines/:id", aiEngine.Update)
	g.DELETE("/ai-engines/:id", aiEngine.Delete)

	// Bộ API key LLM nằm chung nhóm "write" vì cùng lý do với ai_engine: key là
	// của chính người dùng. Riêng toggle `visible_to_users` thì service chặn —
	// chỉ admin bật được, vì bật nó lên là mở hạn mức của một nhóm cho cả
	// hệ thống dùng.
	llmSets := handler.NewLLMAPISet(d.LLMSets)
	g.POST("/llm-api-sets", llmSets.Create)
	g.PATCH("/llm-api-sets/:id", llmSets.Update)
	g.DELETE("/llm-api-sets/:id", llmSets.Delete)
	g.POST("/llm-api-sets/:id/keys", llmSets.AddKey)
	g.PATCH("/llm-api-keys/:key_id", llmSets.UpdateKey)
	g.DELETE("/llm-api-keys/:key_id", llmSets.DeleteKey)
}

// registerDelete gắn các route xoá — chỉ admin.
func registerDelete(g *gin.RouterGroup, d RouterDeps) {
	sourcePost := handler.NewSourcePost(d.SourcePost)
	voice := handler.NewVoice(d.Voice)
	list := handler.NewList(d.List)
	catalog := handler.NewCatalog(d.Catalog)

	g.DELETE("/source-posts/:id", sourcePost.Delete)
	g.DELETE("/voices/:id", voice.Delete)
	g.DELETE("/lists/breaking/:id", list.DeleteBreaking)
	g.DELETE("/lists/scheduled/:id", list.DeleteScheduled)
	g.DELETE("/prompts/:id", catalog.DeletePrompt)
}
