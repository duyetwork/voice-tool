package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/strongbody/voice-tool/backend/internal/pkg/httpx"
	"github.com/strongbody/voice-tool/backend/internal/service"
)

// AuditLog chỉ đọc — audit_log là append-only (specs 1.6).
type AuditLog struct {
	svc *service.Audit
}

func NewAuditLog(svc *service.Audit) *AuditLog { return &AuditLog{svc: svc} }

func (h *AuditLog) Register(r gin.IRoutes) {
	r.GET("/audit-log", h.list)
}

func (h *AuditLog) list(c *gin.Context) {
	objectID, err := queryUUID(c, "object_id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	userID, err := queryUUID(c, "user_id")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	limit, offset := pagination(c)

	items, err := h.svc.List(c.Request.Context(), service.AuditFilter{
		ObjectType: queryString(c, "object_type"),
		ObjectID:   objectID,
		UserID:     userID,
		Limit:      limit,
		Offset:     offset,
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": items, "limit": limit, "offset": offset})
}
