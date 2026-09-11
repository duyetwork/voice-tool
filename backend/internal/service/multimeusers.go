package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// MultimeUsers tra danh bạ tài khoản Strongbody để chọn tác giả bài đăng.
//
// Tách khỏi Voice service vì đây là dữ liệu của hệ thống khác: tool không lưu
// bản sao nào của danh bạ, mỗi lần bốc là hỏi thẳng Strongbody — danh sách bên
// đó đổi thì kết quả đổi theo, không có gì để đồng bộ.
type MultimeUsers struct {
	dir   domain.MultimeDirectory
	creds *MultimeCreds
}

func NewMultimeUsers(dir domain.MultimeDirectory, creds *MultimeCreds) *MultimeUsers {
	return &MultimeUsers{dir: dir, creds: creds}
}

// Random bốc 1 tài khoản theo giới tính, dùng token Strongbody của chính người
// đang thao tác — quyền xem danh bạ là quyền bên Strongbody cấp cho tài khoản
// đó, tool không mượn quyền của ai.
//
// Token hết hạn thì refresh 1 lần rồi thử lại, giống đường publish.
func (m *MultimeUsers) Random(
	ctx context.Context,
	actor uuid.UUID,
	gender domain.Gender,
) (domain.MultimeUser, error) {
	creds, err := m.creds.For(ctx, actor)
	if err != nil {
		return domain.MultimeUser{}, err
	}

	user, err := m.dir.RandomUser(ctx, creds.AccessToken, gender)
	if err == nil {
		return user, nil
	}
	if !errors.Is(err, domain.ErrTokenExpired) {
		return domain.MultimeUser{}, err
	}

	refreshed, refreshErr := m.creds.Refresh(ctx, actor)
	if refreshErr != nil {
		return domain.MultimeUser{}, refreshErr
	}
	return m.dir.RandomUser(ctx, refreshed.AccessToken, gender)
}
