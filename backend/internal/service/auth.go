package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/pkg/jwt"
	"github.com/strongbody/voice-tool/backend/internal/pkg/secret"
	"github.com/strongbody/voice-tool/backend/internal/repository"
)

// Auth đăng nhập bằng tài khoản strongbody/multime (SSO).
//
// Hệ thống KHÔNG có đăng ký và KHÔNG lưu mật khẩu: ai đăng nhập được vào
// multime thì vào được đây. Tài khoản đó cũng chính là tài khoản đăng voice,
// nên access/refresh token của họ được lưu lại (mã hoá) để worker publish thay.
type Auth struct {
	q           *repository.Queries
	tokens      *jwt.Manager
	multimeAuth domain.MultimeAuthenticator
	box         *secret.Box
	log         *slog.Logger

	// defaultRole áp cho tài khoản đăng nhập lần đầu.
	defaultRole domain.Role
	// bootstrapAdminEmail luôn được nâng lên admin khi đăng nhập.
	bootstrapAdminEmail string
}

type AuthDeps struct {
	Queries             *repository.Queries
	Tokens              *jwt.Manager
	MultimeAuth         domain.MultimeAuthenticator
	Box                 *secret.Box
	Logger              *slog.Logger
	DefaultRole         domain.Role
	BootstrapAdminEmail string
}

func NewAuth(d AuthDeps) *Auth {
	role := d.DefaultRole
	if !role.Valid() {
		role = domain.RoleUser
	}
	return &Auth{
		q: d.Queries, tokens: d.Tokens, multimeAuth: d.MultimeAuth,
		box: d.Box, log: d.Logger, defaultRole: role,
		bootstrapAdminEmail: strings.ToLower(strings.TrimSpace(d.BootstrapAdminEmail)),
	}
}

// SignIn xác thực với strongbody rồi cấp JWT của hệ thống này.
func (a *Auth) SignIn(ctx context.Context, email, password string) (jwt.Pair, repository.AppUser, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || password == "" {
		return jwt.Pair{}, repository.AppUser{},
			fmt.Errorf("%w: email và password là bắt buộc", domain.ErrInvalidInput)
	}

	session, err := a.multimeAuth.Login(ctx, email, password)
	if err != nil {
		// ErrUnauthorized / ErrTOTPRequired trả nguyên để handler map đúng status.
		if errors.Is(err, domain.ErrUnauthorized) || errors.Is(err, domain.ErrTOTPRequired) {
			return jwt.Pair{}, repository.AppUser{}, err
		}
		return jwt.Pair{}, repository.AppUser{}, fmt.Errorf("xác thực với multime: %w", err)
	}

	accessToken, err := a.box.EncryptPtr(session.AccessToken)
	if err != nil {
		return jwt.Pair{}, repository.AppUser{}, fmt.Errorf("mã hoá access token: %w", err)
	}
	refreshToken, err := a.box.EncryptPtr(session.RefreshToken)
	if err != nil {
		return jwt.Pair{}, repository.AppUser{}, fmt.Errorf("mã hoá refresh token: %w", err)
	}

	user, err := a.q.UpsertUserFromSSO(ctx, repository.UpsertUserFromSSOParams{
		StrongbodyUserID:    &session.UserID,
		Email:               firstNonEmptyString(session.Email, email),
		FullName:            nilIfEmpty(session.FullName),
		AvatarUrl:           nilIfEmpty(session.AvatarURL),
		Role:                string(a.roleFor(email)),
		MultimeAccessToken:  accessToken,
		MultimeRefreshToken: refreshToken,
	})
	if err != nil {
		return jwt.Pair{}, repository.AppUser{}, fmt.Errorf("lưu user: %w", err)
	}
	if !user.IsActive {
		return jwt.Pair{}, repository.AppUser{}, domain.ErrUnauthorized
	}

	// Email bootstrap phải là admin kể cả khi bản ghi đã tồn tại với role thấp hơn.
	if a.bootstrapAdminEmail != "" && user.Email == a.bootstrapAdminEmail &&
		user.Role != string(domain.RoleAdmin) {
		promoted, err := a.q.SetUserRole(ctx, repository.SetUserRoleParams{
			ID: user.ID, Role: string(domain.RoleAdmin),
		})
		if err != nil {
			a.log.WarnContext(ctx, "không nâng được quyền admin cho tài khoản bootstrap",
				"error", err, "email", user.Email)
		} else {
			user = promoted
		}
	}

	pair, err := a.tokens.Issue(user.ID, user.Email, user.Role)
	if err != nil {
		return jwt.Pair{}, repository.AppUser{}, fmt.Errorf("phát hành token: %w", err)
	}

	a.log.InfoContext(ctx, "đăng nhập thành công",
		"email", user.Email, "role", user.Role, "strongbody_user_id", session.UserID)
	return pair, user, nil
}

// Refresh làm mới JWT của hệ thống này (không liên quan token của multime).
func (a *Auth) Refresh(ctx context.Context, refreshToken string) (jwt.Pair, error) {
	claims, err := a.tokens.Parse(refreshToken, jwt.Refresh)
	if err != nil {
		return jwt.Pair{}, domain.ErrUnauthorized
	}

	user, err := a.q.GetUserByID(ctx, claims.UserID)
	if err != nil || !user.IsActive {
		return jwt.Pair{}, domain.ErrUnauthorized
	}

	pair, err := a.tokens.Issue(user.ID, user.Email, user.Role)
	if err != nil {
		return jwt.Pair{}, fmt.Errorf("phát hành token: %w", err)
	}
	return pair, nil
}

// roleFor quyết định role của lần đăng nhập ĐẦU TIÊN. Các lần sau, ON CONFLICT
// không ghi đè role nên quyền admin đã cấp vẫn giữ nguyên.
func (a *Auth) roleFor(email string) domain.Role {
	if a.bootstrapAdminEmail != "" && email == a.bootstrapAdminEmail {
		return domain.RoleAdmin
	}
	return a.defaultRole
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
