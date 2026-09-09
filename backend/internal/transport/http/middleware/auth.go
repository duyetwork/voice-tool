// Package middleware chứa middleware dùng chung của Gin.
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/pkg/httpx"
	"github.com/strongbody/voice-tool/backend/internal/pkg/jwt"
)

const (
	ctxUserID = "auth.user_id"
	ctxEmail  = "auth.email"
	ctxRole   = "auth.role"
)

// Auth xác thực Bearer access token và gắn user vào context.
func Auth(tokens *jwt.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		token, ok := bearer(header)
		if !ok {
			httpx.Fail(c, domain.ErrUnauthorized)
			return
		}

		claims, err := tokens.Parse(token, jwt.Access)
		if err != nil {
			httpx.Fail(c, domain.ErrUnauthorized)
			return
		}

		c.Set(ctxUserID, claims.UserID)
		c.Set(ctxEmail, claims.Email)
		c.Set(ctxRole, domain.Role(claims.Role))
		c.Next()
	}
}

// AuthMedia xác thực như Auth nhưng chấp nhận thêm token trong query param.
//
// Thẻ <audio>/<video> của trình duyệt không gắn được header Authorization, nên
// muốn phát trực tiếp từ URL thì token phải nằm trong URL. Chỉ dùng cho route
// đọc file media: token là access token (TTL ngắn) và URL không rời khỏi máy
// người dùng.
func AuthMedia(tokens *jwt.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearer(c.GetHeader("Authorization"))
		if !ok {
			token = strings.TrimSpace(c.Query("token"))
			ok = token != ""
		}
		if !ok {
			httpx.Fail(c, domain.ErrUnauthorized)
			return
		}

		claims, err := tokens.Parse(token, jwt.Access)
		if err != nil {
			httpx.Fail(c, domain.ErrUnauthorized)
			return
		}

		c.Set(ctxUserID, claims.UserID)
		c.Set(ctxEmail, claims.Email)
		c.Set(ctxRole, domain.Role(claims.Role))
		c.Next()
	}
}

// ActorID lấy user đang thao tác — dùng cho created_by và audit_log.
func ActorID(c *gin.Context) uuid.UUID {
	v, ok := c.Get(ctxUserID)
	if !ok {
		return uuid.Nil
	}
	id, _ := v.(uuid.UUID)
	return id
}

// ActorRole lấy role từ token đã xác thực.
func ActorRole(c *gin.Context) domain.Role {
	v, ok := c.Get(ctxRole)
	if !ok {
		return domain.RoleViewer
	}
	role, _ := v.(domain.Role)
	if !role.Valid() {
		return domain.RoleViewer
	}
	return role
}

// RequireWrite chặn viewer khỏi mọi thao tác tạo/sửa/chạy/đăng.
func RequireWrite() gin.HandlerFunc {
	return requirePermission(func(r domain.Role) bool { return r.CanWrite() },
		"thao tác này cần quyền user hoặc admin")
}

// RequireDelete: chỉ admin được xoá. User bình thường đăng được voice nhưng
// không xoá được dữ liệu.
func RequireDelete() gin.HandlerFunc {
	return requirePermission(func(r domain.Role) bool { return r.CanDelete() },
		"xoá dữ liệu cần quyền admin")
}

// RequireAdmin dành cho quản lý tài khoản.
func RequireAdmin() gin.HandlerFunc {
	return requirePermission(func(r domain.Role) bool { return r.CanManageUsers() },
		"thao tác này cần quyền admin")
}

func requirePermission(allowed func(domain.Role) bool, message string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !allowed(ActorRole(c)) {
			c.AbortWithStatusJSON(http.StatusForbidden, httpx.ErrorBody{
				Error:   "forbidden",
				Message: message,
			})
			return
		}
		c.Next()
	}
}

func bearer(header string) (string, bool) {
	parts := strings.SplitN(strings.TrimSpace(header), " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	token := strings.TrimSpace(parts[1])
	return token, token != ""
}
