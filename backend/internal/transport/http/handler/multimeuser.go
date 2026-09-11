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
}

func NewMultimeUsers(svc *service.MultimeUsers) *MultimeUsers { return &MultimeUsers{svc: svc} }

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
func (h *MultimeUsers) Countries(c *gin.Context) {
	countries, err := h.svc.Countries(c.Request.Context(), middleware.ActorID(c))
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"countries": countries})
}
