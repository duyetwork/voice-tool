package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/strongbody/voice-tool/backend/internal/pkg/httpx"
	"github.com/strongbody/voice-tool/backend/internal/service"
)

// Auth chỉ có đăng nhập và refresh — hệ thống KHÔNG có đăng ký.
// Tài khoản dùng để đăng nhập là tài khoản strongbody/multime, và cũng chính là
// tài khoản mà voice sẽ được đăng lên.
type Auth struct {
	svc *service.Auth
}

func NewAuth(svc *service.Auth) *Auth { return &Auth{svc: svc} }

func (h *Auth) Register(r gin.IRoutes) {
	r.POST("/auth/login", h.signIn)
	r.POST("/auth/refresh", h.refresh)
}

type signInRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

func (h *Auth) signIn(c *gin.Context) {
	var req signInRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, err)
		return
	}

	pair, user, err := h.svc.SignIn(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{
		"token": pair,
		"user": gin.H{
			"id":        user.ID,
			"email":     user.Email,
			"full_name": user.FullName,
			"avatar":    user.AvatarUrl,
			"role":      user.Role,
		},
	})
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

func (h *Auth) refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, err)
		return
	}

	pair, err := h.svc.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"token": pair})
}
