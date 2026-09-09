package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/repository"
)

// User là service quản lý tài khoản — chỉ role admin được gọi (guard ở
// middleware).
type User struct {
	q   *repository.Queries
	log *slog.Logger
}

func NewUser(q *repository.Queries, log *slog.Logger) *User {
	return &User{q: q, log: log}
}

func (u *User) List(ctx context.Context, limit, offset int32) ([]repository.AppUser, int64, error) {
	limit, offset = clampPage(limit, offset)

	users, err := u.q.ListUsers(ctx, repository.ListUsersParams{Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, fmt.Errorf("list user: %w", err)
	}
	total, err := u.q.CountUsers(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count user: %w", err)
	}
	return users, total, nil
}

// SetRole nâng/hạ quyền. Admin không tự hạ quyền chính mình để tránh khoá luôn
// khả năng quản trị hệ thống.
func (u *User) SetRole(ctx context.Context, actor, target uuid.UUID, role string) (repository.AppUser, error) {
	r := domain.Role(strings.ToLower(strings.TrimSpace(role)))
	if !r.Valid() {
		return repository.AppUser{}, fmt.Errorf("%w: role phải là admin, user hoặc viewer",
			domain.ErrInvalidInput)
	}
	if actor == target && r != domain.RoleAdmin {
		return repository.AppUser{}, fmt.Errorf("%w: không thể tự hạ quyền admin của chính mình",
			domain.ErrInvalidInput)
	}

	// Hạ quyền admin cuối cùng = khoá luôn khả năng quản trị hệ thống.
	if r != domain.RoleAdmin {
		if err := u.ensureNotLastAdmin(ctx, target); err != nil {
			return repository.AppUser{}, err
		}
	}

	user, err := u.q.SetUserRole(ctx, repository.SetUserRoleParams{ID: target, Role: string(r)})
	if err != nil {
		return repository.AppUser{}, wrapNotFound(err, "user "+target.String())
	}
	return user, nil
}

// SetActive bật/tắt tài khoản (thay cho xoá — giữ nguyên audit trail).
func (u *User) SetActive(ctx context.Context, actor, target uuid.UUID, active bool) (repository.AppUser, error) {
	if actor == target && !active {
		return repository.AppUser{}, fmt.Errorf("%w: không thể tự vô hiệu hoá tài khoản của mình",
			domain.ErrInvalidInput)
	}
	if !active {
		if err := u.ensureNotLastAdmin(ctx, target); err != nil {
			return repository.AppUser{}, err
		}
	}

	user, err := u.q.SetUserActive(ctx, repository.SetUserActiveParams{ID: target, IsActive: active})
	if err != nil {
		return repository.AppUser{}, wrapNotFound(err, "user "+target.String())
	}
	return user, nil
}

// ensureNotLastAdmin chặn việc vô hiệu hoá/hạ quyền admin cuối cùng.
func (u *User) ensureNotLastAdmin(ctx context.Context, target uuid.UUID) error {
	user, err := u.q.GetUserByID(ctx, target)
	if err != nil {
		return wrapNotFound(err, "user "+target.String())
	}
	if user.Role != string(domain.RoleAdmin) || !user.IsActive {
		return nil
	}

	admins, err := u.q.CountAdmins(ctx)
	if err != nil {
		return fmt.Errorf("đếm admin: %w", err)
	}
	if admins <= 1 {
		return fmt.Errorf("%w: đây là admin duy nhất — cấp quyền admin cho người khác trước",
			domain.ErrInvalidInput)
	}
	return nil
}
