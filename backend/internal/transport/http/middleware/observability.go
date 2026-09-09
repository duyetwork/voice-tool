package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/strongbody/voice-tool/backend/internal/pkg/httpx"
)

const headerRequestID = "X-Request-ID"

// RequestID gắn request id vào context + response header để trace log.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(headerRequestID)
		if id == "" {
			id = uuid.NewString()
		}
		c.Set("request_id", id)
		c.Writer.Header().Set(headerRequestID, id)
		c.Next()
	}
}

// Logger ghi log truy cập dạng structured.
func Logger(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		attrs := []any{
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", c.GetString("request_id"),
		}
		if len(c.Errors) > 0 {
			attrs = append(attrs, "errors", c.Errors.String())
		}

		switch {
		case c.Writer.Status() >= http.StatusInternalServerError:
			log.Error("http request", attrs...)
		case c.Writer.Status() >= http.StatusBadRequest:
			log.Warn("http request", attrs...)
		default:
			log.Info("http request", attrs...)
		}
	}
}

// Recovery bắt panic, trả 500 thay vì làm chết process.
func Recovery(log *slog.Logger) gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, recovered any) {
		log.Error("panic trong handler",
			"panic", recovered,
			"path", c.Request.URL.Path,
			"request_id", c.GetString("request_id"))
		c.AbortWithStatusJSON(http.StatusInternalServerError, httpx.ErrorBody{Error: "internal_error"})
	})
}
