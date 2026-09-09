package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/pkg/secret"
	"github.com/strongbody/voice-tool/backend/internal/repository"
)

// MultimeCreds giải mã và làm mới credential multime của từng user.
//
// Voice được đăng bằng chính tài khoản của người tạo ra nó (yêu cầu: "tài khoản
// up voice chính là tài khoản đang dùng"), nên worker cần token của user đó.
type MultimeCreds struct {
	q    *repository.Queries
	auth domain.MultimeAuthenticator
	box  *secret.Box
	log  *slog.Logger
}

func NewMultimeCreds(
	q *repository.Queries,
	auth domain.MultimeAuthenticator,
	box *secret.Box,
	log *slog.Logger,
) *MultimeCreds {
	return &MultimeCreds{q: q, auth: auth, box: box, log: log}
}

// For trả về credential đang lưu của user.
func (m *MultimeCreds) For(ctx context.Context, userID uuid.UUID) (domain.MultimeCredentials, error) {
	user, err := m.q.GetUserByID(ctx, userID)
	if err != nil {
		return domain.MultimeCredentials{}, wrapNotFound(err, "user "+userID.String())
	}
	return m.fromUser(user)
}

// Refresh đổi refresh token thành access token mới và lưu lại.
//
// Thất bại thì xoá token đang lưu và trả ErrReloginRequired — chỉ chính user đó
// đăng nhập lại mới khôi phục được, nên không có cách nào tự sửa.
func (m *MultimeCreds) Refresh(ctx context.Context, userID uuid.UUID) (domain.MultimeCredentials, error) {
	user, err := m.q.GetUserByID(ctx, userID)
	if err != nil {
		return domain.MultimeCredentials{}, wrapNotFound(err, "user "+userID.String())
	}

	refreshToken, err := m.box.DecryptPtr(user.MultimeRefreshToken)
	if err != nil {
		return domain.MultimeCredentials{}, fmt.Errorf("giải mã refresh token: %w", err)
	}
	if refreshToken == "" {
		return domain.MultimeCredentials{}, relogin(user.Email)
	}

	accessToken, err := m.auth.RefreshAccessToken(ctx, refreshToken)
	if err != nil {
		if errors.Is(err, domain.ErrReloginRequired) {
			m.clearToken(ctx, user)
			return domain.MultimeCredentials{}, relogin(user.Email)
		}
		return domain.MultimeCredentials{}, fmt.Errorf("refresh token multime: %w", err)
	}

	encrypted, err := m.box.EncryptPtr(accessToken)
	if err != nil {
		return domain.MultimeCredentials{}, fmt.Errorf("mã hoá access token: %w", err)
	}
	if err := m.q.SetUserMultimeToken(ctx, repository.SetUserMultimeTokenParams{
		ID: user.ID, MultimeAccessToken: encrypted,
	}); err != nil {
		return domain.MultimeCredentials{}, fmt.Errorf("lưu access token: %w", err)
	}

	m.log.InfoContext(ctx, "đã refresh token multime", "email", user.Email)
	return domain.MultimeCredentials{
		AccessToken: accessToken,
		AuthorID:    deref(user.StrongbodyUserID),
	}, nil
}

func (m *MultimeCreds) fromUser(user repository.AppUser) (domain.MultimeCredentials, error) {
	accessToken, err := m.box.DecryptPtr(user.MultimeAccessToken)
	if err != nil {
		// Sai TOKEN_ENCRYPTION_KEY (vd đổi khoá mà không migrate) — token cũ
		// không giải mã được, coi như phải đăng nhập lại.
		m.log.Error("không giải mã được access token multime",
			"error", err, "email", user.Email)
		return domain.MultimeCredentials{}, relogin(user.Email)
	}

	authorID := deref(user.StrongbodyUserID)
	if accessToken == "" || authorID == 0 {
		return domain.MultimeCredentials{}, relogin(user.Email)
	}
	return domain.MultimeCredentials{AccessToken: accessToken, AuthorID: authorID}, nil
}

func (m *MultimeCreds) clearToken(ctx context.Context, user repository.AppUser) {
	if err := m.q.ClearUserMultimeToken(ctx, user.ID); err != nil {
		m.log.WarnContext(ctx, "không xoá được token multime đã hết hiệu lực",
			"error", err, "email", user.Email)
	}
}

func relogin(email string) error {
	return domain.Permanent(fmt.Errorf("%w: tài khoản %s cần đăng nhập lại vào voice-tool",
		domain.ErrReloginRequired, email))
}
