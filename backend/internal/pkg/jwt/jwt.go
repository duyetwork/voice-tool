// Package jwt phát hành và xác thực access/refresh token.
package jwt

import (
	"errors"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var ErrInvalidToken = errors.New("token không hợp lệ hoặc đã hết hạn")

type TokenType string

const (
	Access  TokenType = "access"
	Refresh TokenType = "refresh"
)

type Claims struct {
	UserID uuid.UUID `json:"uid"`
	Email  string    `json:"email"`
	Role   string    `json:"role"`
	Type   TokenType `json:"typ"`
	jwtlib.RegisteredClaims
}

type Manager struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewManager(secret string, accessTTL, refreshTTL time.Duration) *Manager {
	return &Manager{secret: []byte(secret), accessTTL: accessTTL, refreshTTL: refreshTTL}
}

type Pair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

func (m *Manager) Issue(userID uuid.UUID, email, role string) (Pair, error) {
	access, err := m.sign(userID, email, role, Access, m.accessTTL)
	if err != nil {
		return Pair{}, err
	}
	refresh, err := m.sign(userID, email, role, Refresh, m.refreshTTL)
	if err != nil {
		return Pair{}, err
	}
	return Pair{AccessToken: access, RefreshToken: refresh, ExpiresIn: int64(m.accessTTL.Seconds())}, nil
}

func (m *Manager) sign(userID uuid.UUID, email, role string, typ TokenType, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: userID,
		Email:  email,
		Role:   role,
		Type:   typ,
		RegisteredClaims: jwtlib.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwtlib.NewNumericDate(now),
			ExpiresAt: jwtlib.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, claims).SignedString(m.secret)
}

// Parse xác thực token và kiểm tra đúng loại (access vs refresh).
func (m *Manager) Parse(token string, want TokenType) (*Claims, error) {
	parsed, err := jwtlib.ParseWithClaims(token, &Claims{}, func(t *jwtlib.Token) (any, error) {
		return m.secret, nil
	}, jwtlib.WithValidMethods([]string{jwtlib.SigningMethodHS256.Alg()}))
	if err != nil {
		return nil, ErrInvalidToken
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid || claims.Type != want {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
