package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/strongbody/voice-tool/backend/internal/pkg/httpx"
	"github.com/strongbody/voice-tool/backend/internal/service"
	"github.com/strongbody/voice-tool/backend/internal/transport/http/middleware"
)

// Scrape là mặt HTTP của hạ tầng via/proxy — toàn bộ nằm dưới màn Cài đặt.
//
// Mọi route đều là route ADMIN: via là bí mật đăng nhập và proxy là lối ra mạng
// của cả hệ thống, không phải dữ liệu nghiệp vụ của từng người dùng.
type Scrape struct {
	svc *service.ScrapeAdmin
}

func NewScrape(svc *service.ScrapeAdmin) *Scrape { return &Scrape{svc: svc} }

// Register gắn toàn bộ route vào nhóm admin.
func (h *Scrape) Register(admin *gin.RouterGroup) {
	admin.GET("/settings/vias", h.ListVias)
	admin.POST("/settings/vias", h.CreateVia)
	admin.PATCH("/settings/vias/:id", h.UpdateVia)
	admin.DELETE("/settings/vias/:id", h.DeleteVia)

	admin.GET("/settings/proxies", h.ListProxies)
	admin.POST("/settings/proxies", h.CreateProxy)
	admin.PATCH("/settings/proxies/:id", h.UpdateProxy)
	admin.DELETE("/settings/proxies/:id", h.DeleteProxy)

	// Bảng tổng quan: bao nhiêu via còn sống theo từng nền tảng, và lượt quét
	// rải ra sao trong ngày.
	// Nền tảng nào cần cookie gì — form thêm via đọc đây để hiện đúng mẫu.
	// Đi từ server vì chính server là bên từ chối khi thiếu.
	admin.GET("/settings/via-cookie-specs", h.CookieSpecs)

	admin.GET("/settings/scrape-health", h.Health)
	admin.GET("/settings/scrape-load", h.HourlyLoad)
}

// ---------------------------------------------------------------------------
// Via
// ---------------------------------------------------------------------------

// viaRequest — form thêm/sửa via.
//
// `cookies` chỉ đi MỘT CHIỀU: gửi lên được, không bao giờ trả về. Bỏ trống khi
// sửa nghĩa là giữ nguyên cookies cũ, vì giao diện không có bản thật để gửi lại.
type viaRequest struct {
	Platform string `json:"platform"`
	Label    string `json:"label"`
	Cookies  string `json:"cookies"`
	// DailyQuota: số lượt quét kênh tối đa/ngày của via này.
	DailyQuota *int32 `json:"daily_quota" binding:"omitempty,min=1,max=10000"`
	// Status: chỉ `active` / `disabled`. `cooldown` và `dead` là kết luận của
	// hệ thống, đặt tay được thì bảng tổng quan hết ý nghĩa.
	Status *string `json:"status" binding:"omitempty,oneof=active disabled"`
}

func (h *Scrape) ListVias(c *gin.Context) {
	items, err := h.svc.ListVias(c.Request.Context(), queryString(c, "platform"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Scrape) CreateVia(c *gin.Context) {
	var req viaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, err)
		return
	}
	out, err := h.svc.CreateVia(c.Request.Context(), middleware.ActorID(c), service.ViaInput{
		Platform:   req.Platform,
		Label:      req.Label,
		Cookies:    req.Cookies,
		DailyQuota: req.DailyQuota,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, out)
}

func (h *Scrape) UpdateVia(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req viaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, err)
		return
	}
	out, err := h.svc.UpdateVia(c.Request.Context(), middleware.ActorID(c), id, service.ViaInput{
		Label:      req.Label,
		Cookies:    req.Cookies,
		DailyQuota: req.DailyQuota,
		Status:     req.Status,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Scrape) DeleteVia(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.DeleteVia(c.Request.Context(), middleware.ActorID(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

// ---------------------------------------------------------------------------
// Proxy
// ---------------------------------------------------------------------------

// proxyRequest — form thêm/sửa proxy. `endpoint` đi một chiều như `cookies`.
type proxyRequest struct {
	Label string `json:"label"`
	// Platform rỗng = dùng chung mọi nền tảng (gateway residential xoay IP).
	Platform string `json:"platform"`
	Endpoint string `json:"endpoint"`
	Kind     string `json:"kind" binding:"omitempty,oneof=residential mobile datacenter"`
	Status   *string
}

// proxyUpdateRequest tách riêng vì chỉ lúc SỬA mới đặt được trạng thái — và đó
// là đường DUY NHẤT để bật lại một proxy đã bị hạ cấp, vì hệ thống cố tình
// không tự hồi sinh proxy.
type proxyUpdateRequest struct {
	proxyRequest
	Status *string `json:"status" binding:"omitempty,oneof=active disabled"`
}

func (h *Scrape) ListProxies(c *gin.Context) {
	items, err := h.svc.ListProxies(c.Request.Context(), queryString(c, "platform"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Scrape) CreateProxy(c *gin.Context) {
	var req proxyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, err)
		return
	}
	out, err := h.svc.CreateProxy(c.Request.Context(), middleware.ActorID(c), service.ProxyInput{
		Label:    req.Label,
		Platform: req.Platform,
		Endpoint: req.Endpoint,
		Kind:     req.Kind,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.Created(c, out)
}

func (h *Scrape) UpdateProxy(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req proxyUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, err)
		return
	}
	out, err := h.svc.UpdateProxy(c.Request.Context(), middleware.ActorID(c), id, service.ProxyInput{
		Label:    req.Label,
		Endpoint: req.Endpoint,
		Kind:     req.Kind,
		Status:   req.Status,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, out)
}

func (h *Scrape) DeleteProxy(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := h.svc.DeleteProxy(c.Request.Context(), middleware.ActorID(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.NoContent(c)
}

// ---------------------------------------------------------------------------
// Tổng quan
// ---------------------------------------------------------------------------

func (h *Scrape) CookieSpecs(c *gin.Context) {
	httpx.OK(c, gin.H{"items": service.ViaCookieSpecs()})
}

func (h *Scrape) Health(c *gin.Context) {
	items, err := h.svc.Health(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Scrape) HourlyLoad(c *gin.Context) {
	days, err := daysParam(c, 7)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	items, err := h.svc.HourlyLoad(c.Request.Context(), days)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": items, "days": days})
}
