// Package httpx chuẩn hoá format response và map domain error -> HTTP status.
package httpx

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

type ErrorBody struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

type Page[T any] struct {
	Items  []T   `json:"items"`
	Total  int64 `json:"total"`
	Limit  int32 `json:"limit"`
	Offset int32 `json:"offset"`
}

func OK(c *gin.Context, body any)      { c.JSON(http.StatusOK, body) }
func Created(c *gin.Context, body any) { c.JSON(http.StatusCreated, body) }
func NoContent(c *gin.Context)         { c.Status(http.StatusNoContent) }

// Fail map error nghiệp vụ sang status code tương ứng.
// Fail map lỗi sang HTTP status + câu tiếng Việt cho người dùng.
//
// Dùng domain.UserMessage chứ không phải err.Error(): chuỗi lỗi đầy đủ chứa cả
// nguyên văn response của nhà cung cấp (kèm mã lỗi nội bộ của họ, đường dẫn
// API...) — thứ đó thuộc về log, không thuộc về màn hình người dùng.
func Fail(c *gin.Context, err error) {
	status, code := classify(err)
	c.AbortWithStatusJSON(status, ErrorBody{Error: code, Message: domain.UserMessage(err)})
}

// BadRequest dùng cho lỗi bind/validate payload.
func BadRequest(c *gin.Context, err error) {
	c.AbortWithStatusJSON(http.StatusBadRequest, ErrorBody{Error: "invalid_request", Message: err.Error()})
}

func classify(err error) (int, string) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, "not_found"
	case errors.Is(err, domain.ErrUnauthorized):
		return http.StatusUnauthorized, "unauthorized"
	case errors.Is(err, domain.ErrForbidden):
		return http.StatusForbidden, "forbidden"
	case errors.Is(err, domain.ErrAlreadyPublished):
		return http.StatusConflict, "already_published"
	case errors.Is(err, domain.ErrDuplicate):
		return http.StatusConflict, "duplicate_post"
	case errors.Is(err, domain.ErrInvalidInput),
		errors.Is(err, domain.ErrUnsupportedURL),
		errors.Is(err, domain.ErrUnsupportedType),
		errors.Is(err, domain.ErrPromptRequired),
		errors.Is(err, domain.ErrLangUnsupported),
		errors.Is(err, domain.ErrRegexInvalid),
		errors.Is(err, domain.ErrNoVoiceFile),
		// Tài khoản bật 2FA: người dùng phải tự tắt, không phải lỗi hệ thống.
		errors.Is(err, domain.ErrTOTPRequired):
		return http.StatusBadRequest, "invalid_request"
	default:
		return http.StatusInternalServerError, "internal_error"
	}
}
