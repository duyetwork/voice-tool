package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/strongbody/voice-tool/backend/internal/pkg/httpx"
	"github.com/strongbody/voice-tool/backend/internal/service"
	"github.com/strongbody/voice-tool/backend/internal/transport/http/middleware"
)

// User là các endpoint quản lý tài khoản — router gắn RequireAdmin cho nhóm này.
type User struct {
	svc *service.User
}

func NewUser(svc *service.User) *User { return &User{svc: svc} }

func (h *User) Register(r gin.IRoutes) {
	r.GET("/users", h.list)
	r.PATCH("/users/:id/role", h.setRole)
	r.PATCH("/users/:id/active", h.setActive)
}

// publicUser ẩn password_hash khỏi response.
type publicUser struct {
	ID       string  `json:"id"`
	Email    string  `json:"email"`
	FullName *string `json:"full_name"`
	Role     string  `json:"role"`
	IsActive bool    `json:"is_active"`
	Created  string  `json:"created_at"`
}

func (h *User) list(c *gin.Context) {
	limit, offset := pagination(c)
	_, dir := sorting(c)

	users, total, err := h.svc.List(c.Request.Context(), limit, offset, dir)
	if err != nil {
		httpx.Fail(c, err)
		return
	}

	items := make([]publicUser, 0, len(users))
	for _, u := range users {
		items = append(items, publicUser{
			ID:       u.ID.String(),
			Email:    u.Email,
			FullName: u.FullName,
			Role:     u.Role,
			IsActive: u.IsActive,
			Created:  u.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	httpx.OK(c, httpx.Page[publicUser]{Items: items, Total: total, Limit: limit, Offset: offset})
}

type setRoleRequest struct {
	Role string `json:"role" binding:"required,oneof=admin editor user"`
}

func (h *User) setRole(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req setRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, err)
		return
	}

	user, err := h.svc.SetRole(c.Request.Context(), middleware.ActorID(c), id, req.Role)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, publicUser{
		ID: user.ID.String(), Email: user.Email, FullName: user.FullName,
		Role: user.Role, IsActive: user.IsActive,
	})
}

type setActiveRequest struct {
	IsActive *bool `json:"is_active" binding:"required"`
}

func (h *User) setActive(c *gin.Context) {
	id, err := pathUUID(c, "id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	var req setActiveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, err)
		return
	}

	user, err := h.svc.SetActive(c.Request.Context(), middleware.ActorID(c), id, *req.IsActive)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, publicUser{
		ID: user.ID.String(), Email: user.Email, FullName: user.FullName,
		Role: user.Role, IsActive: user.IsActive,
	})
}
