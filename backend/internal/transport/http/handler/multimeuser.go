package handler

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/pkg/httpx"
	"github.com/strongbody/voice-tool/backend/internal/service"
	"github.com/strongbody/voice-tool/backend/internal/transport/http/middleware"
)

// MultimeUsers phục vụ ô chọn tác giả bài đăng: bốc ngẫu nhiên 1 tài khoản
// Strongbody theo giới tính.
type MultimeUsers struct {
	svc *service.MultimeUsers
	// catalog phục vụ danh mục ĐÃ LƯU trong DB (quốc gia, hashtag) — modal Tạo
	// Voice đọc từ đây thay vì hỏi Strongbody mỗi lần mở.
	catalog *service.CatalogCache
}

func NewMultimeUsers(svc *service.MultimeUsers, catalog *service.CatalogCache) *MultimeUsers {
	return &MultimeUsers{svc: svc, catalog: catalog}
}

// Random trả 1 tài khoản bất kỳ có giới tính `gender`.
//
// Mỗi lần gọi là một lần bốc mới: người dùng bấm nút random là gọi lại đây,
// nên response không được cache.
func (h *MultimeUsers) Random(c *gin.Context) {
	gender := domain.Gender(strings.ToLower(strings.TrimSpace(c.Query("gender"))))
	if !gender.Valid() {
		httpx.BadRequest(c, fmt.Errorf("gender phải là male, female hoặc other"))
		return
	}
	// country_id tuỳ chọn: bỏ trống thì bốc trong toàn bộ danh bạ.
	countryID, _ := strconv.ParseInt(strings.TrimSpace(c.Query("country_id")), 10, 64)

	user, err := h.svc.Random(c.Request.Context(), middleware.ActorID(c), gender, countryID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"author": user})
}

// Countries trả danh mục quốc gia cho ô chọn quốc gia của author.
//
// Đọc từ bảng `country` trong DB; chỉ khi bảng rỗng hoặc quá cũ mới hỏi lại
// Strongbody. Danh mục này đổi vài năm một lần nên hỏi mỗi lần mở modal là
// buộc một thao tác thường ngày phụ thuộc vào token của hệ thống khác.
func (h *MultimeUsers) Countries(c *gin.Context) {
	countries, err := h.catalog.Countries(c.Request.Context(), middleware.ActorID(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"countries": countries})
}

// Catalog trả CẢ BA danh mục mà modal Tạo Voice cần trong 1 lần gọi: quốc gia,
// hashtag, và thứ tự ưu tiên của ngôn ngữ.
//
// Gộp làm một vì modal cần cả ba cùng lúc, và mỗi lần tải là một lần chờ.
// Frontend cache nguyên khối này nên mở modal lần sau không gọi lại gì.
// Hashtags tìm trong danh mục hashtag ĐÃ LƯU của tool, không hỏi MultiMe.
//
// Có endpoint riêng vì danh mục bên MultiMe có ~94.000 mục: không tải hết về
// trình duyệt để lọc tại chỗ được. `/meta/catalog` trả sẵn phần đầu (tag tuyển
// chọn) cho lần mở modal đầu tiên; ô tìm kiếm gọi đây khi người dùng gõ.
func (h *MultimeUsers) Hashtags(c *gin.Context) {
	items, err := h.catalog.Hashtags(c.Request.Context(), middleware.ActorID(c), c.Query("q"))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *MultimeUsers) Catalog(c *gin.Context) {
	view, err := h.catalog.Catalog(c.Request.Context(), middleware.ActorID(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, view)
}
