package multime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// Mock giả lập multime.ai để chạy được luồng đăng nhập + publish khi chưa cấu
// hình MULTIME_BASE_URL. Vẫn áp dụng đúng các ràng buộc của API thật (title,
// hashtag, độ dài tối thiểu) để lỗi cấu hình lộ ra ở dev, không phải ở production.
type Mock struct{}

var (
	_ domain.MultimeClient        = (*Mock)(nil)
	_ domain.MultimeAuthenticator = (*Mock)(nil)
)

func NewMock() *Mock { return &Mock{} }

// Login nhận mọi email/password có độ dài hợp lệ và sinh user id ổn định theo
// email, để dev đăng nhập được nhiều tài khoản khác nhau.
func (m *Mock) Login(_ context.Context, email, password string) (domain.MultimeSession, error) {
	if email == "" || len(password) < 6 {
		return domain.MultimeSession{}, domain.ErrUnauthorized
	}

	sum := sha256.Sum256([]byte(email))
	id := int64(sum[0])<<16 | int64(sum[1])<<8 | int64(sum[2])
	if id == 0 {
		id = 1
	}

	return domain.MultimeSession{
		UserID:       id,
		Email:        email,
		FullName:     email,
		AccessToken:  "mock-access-" + hex.EncodeToString(sum[:4]),
		RefreshToken: "mock-refresh-" + hex.EncodeToString(sum[:4]),
	}, nil
}

func (m *Mock) RefreshAccessToken(_ context.Context, refreshToken string) (string, error) {
	if refreshToken == "" {
		return "", domain.ErrReloginRequired
	}
	return "mock-access-refreshed", nil
}

func (m *Mock) PublishVoice(
	_ context.Context,
	creds domain.MultimeCredentials,
	audio []byte,
	post domain.VoicePostInput,
) (string, error) {
	if creds.AccessToken == "" || creds.AuthorID == 0 {
		return "", domain.ErrReloginRequired
	}

	validator := &Client{defaultHashtags: []string{"voicetool"}}
	if err := validator.validate(post); err != nil {
		return "", err
	}

	sum := sha256.Sum256(audio)
	return fmt.Sprintf("https://multime.ai/voice/mock-%d-%s",
		creds.AuthorID, hex.EncodeToString(sum[:4])), nil
}
