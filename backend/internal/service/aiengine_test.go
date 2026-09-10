package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/pkg/secret"
)

func newTestEngineService(t *testing.T) *AIEngineService {
	t.Helper()

	key, err := secret.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey lỗi: %v", err)
	}
	box, err := secret.NewBox(key)
	if err != nil {
		t.Fatalf("NewBox lỗi: %v", err)
	}
	return NewAIEngineService(nil, box)
}

// Quyền đụng vào API key: admin quản lý key của mọi người, các vai trò khác
// chỉ key của chính mình — kể cả editor (editor có quyền xoá dữ liệu nghiệp
// vụ, nhưng key TTS là credential riêng của từng người).
func TestActorMayManage(t *testing.T) {
	me, other := uuid.New(), uuid.New()

	cases := []struct {
		name    string
		actor   Actor
		owner   uuid.UUID
		wantErr bool
	}{
		{"admin sửa key người khác", Actor{ID: me, Role: domain.RoleAdmin}, other, false},
		{"editor sửa key người khác", Actor{ID: me, Role: domain.RoleEditor}, other, true},
		{"user sửa key người khác", Actor{ID: me, Role: domain.RoleUser}, other, true},
		{"user sửa key của mình", Actor{ID: me, Role: domain.RoleUser}, me, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.actor.mayManage(tc.owner)
			if tc.wantErr && !errors.Is(err, domain.ErrForbidden) {
				t.Errorf("mayManage() = %v, muốn ErrForbidden", err)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("mayManage() = %v, muốn nil", err)
			}
		})
	}
}

// API key không bao giờ ra khỏi backend: view chỉ có 4 ký tự cuối, không có
// ciphertext lẫn key gốc.
func TestAIEngineMaskKhongLoKey(t *testing.T) {
	s := newTestEngineService(t)
	const key = "sk-ov-1234567890abcdWXYZ"

	encrypted, err := s.encryptKey(key, true)
	if err != nil {
		t.Fatalf("encryptKey lỗi: %v", err)
	}

	masked := s.mask(*encrypted)
	if masked != "••••WXYZ" {
		t.Errorf("mask() = %q, muốn %q", masked, "••••WXYZ")
	}
	if strings.Contains(masked, key) || strings.Contains(masked, *encrypted) {
		t.Errorf("bản che vẫn chứa key hoặc ciphertext: %q", masked)
	}
	// Ciphertext hỏng (đổi khoá, sửa tay trong DB) -> che rỗng, không bao giờ
	// để lọt ciphertext ra ngoài.
	if got := s.mask("khong-phai-ciphertext"); got != "" {
		t.Errorf("ciphertext hỏng thì mask() phải rỗng, được %q", got)
	}
}

// Lúc tạo bắt buộc có key; lúc sửa bỏ trống nghĩa là giữ key cũ (nil -> query
// COALESCE giữ nguyên giá trị đang có).
func TestAIEngineEncryptKey(t *testing.T) {
	s := newTestEngineService(t)

	if _, err := s.encryptKey("", true); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("tạo mà không có key phải lỗi ErrInvalidInput, được %v", err)
	}
	got, err := s.encryptKey("  ", false)
	if err != nil || got != nil {
		t.Errorf("sửa với key rỗng phải trả (nil, nil), được (%v, %v)", got, err)
	}
	if _, err := s.encryptKey("sk-ov-123", true); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("key quá ngắn phải bị chặn, được %v", err)
	}
}
