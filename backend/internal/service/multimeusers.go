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
	countryID int64,
) (domain.MultimeUser, error) {
	creds, err := m.creds.For(ctx, actor)
	if err != nil {
		return domain.MultimeUser{}, err
	}

	user, err := m.dir.RandomUser(ctx, creds.AccessToken, gender, countryID)
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
	return m.dir.RandomUser(ctx, refreshed.AccessToken, gender, countryID)
}

// VoiceHashtags lấy 1 trang danh mục hashtag của voice, cùng cách xử lý token
// hết hạn như Countries.
func (m *MultimeUsers) VoiceHashtags(
	ctx context.Context,
	actor uuid.UUID,
	page, limit int,
) ([]domain.MultimeHashtag, int, error) {
	creds, err := m.creds.For(ctx, actor)
	if err != nil {
		return nil, 0, err
	}

	tags, total, err := m.dir.VoiceHashtags(ctx, creds.AccessToken, page, limit)
	if err == nil {
		return tags, total, nil
	}
	if !errors.Is(err, domain.ErrTokenExpired) {
		return nil, 0, err
	}

	refreshed, refreshErr := m.creds.Refresh(ctx, actor)
	if refreshErr != nil {
		return nil, 0, refreshErr
	}
	return m.dir.VoiceHashtags(ctx, refreshed.AccessToken, page, limit)
}

// Countries liệt kê quốc gia để người dùng chọn trước khi bốc author.
//
// Danh mục này đổi rất chậm nên phía HTTP đặt cache dài; ở đây vẫn hỏi thẳng
// Strongbody để không phải nuôi một bản sao thứ hai trong DB.
func (m *MultimeUsers) Countries(ctx context.Context, actor uuid.UUID) ([]domain.MultimeCountry, error) {
	creds, err := m.creds.For(ctx, actor)
	if err != nil {
		return nil, err
	}

	countries, err := m.dir.Countries(ctx, creds.AccessToken)
	if err == nil {
		return countries, nil
	}
	if !errors.Is(err, domain.ErrTokenExpired) {
		return nil, err
	}

	refreshed, refreshErr := m.creds.Refresh(ctx, actor)
	if refreshErr != nil {
		return nil, refreshErr
	}
	return m.dir.Countries(ctx, refreshed.AccessToken)
}
