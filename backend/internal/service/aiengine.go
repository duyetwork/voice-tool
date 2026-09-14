package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/pkg/secret"
	"github.com/strongbody/voice-tool/backend/internal/repository"
)

// AI Engine (A2) — thực chất là SỔ API KEY TTS, không phải danh mục engine.
//
// Chỉ còn 1 nhà cung cấp TTS (3voices) nên không có gì để "chọn": mỗi người
// khai API key của chính mình, worker chạy TTS bằng key của chủ sở hữu voice,
// nhờ vậy quota và hoá đơn 3voices rơi đúng vào người dùng nó.
//
// Bản ghi vì thế chỉ còn 4 thông tin: key, chủ sở hữu, người khai, lần dùng
// gần nhất — tên gọi / provider / danh sách ngôn ngữ đã bỏ ở migration 000010.
//
// Luật xem/sửa: admin thấy và quản lý key của mọi người (kể cả khai hộ và gán
// lại chủ sở hữu); các vai trò khác chỉ thấy và sửa key của chính mình. Chặn ở
// service chứ không chỉ ở UI — gọi thẳng API cũng không đọc được key người khác.

// Actor là người đang thao tác, đủ để quyết định "được đụng vào bản ghi của
// ai". Tách thành kiểu riêng để handler không phải truyền 2 tham số rời rạc và
// dễ quên tham số role ở một endpoint nào đó.
type Actor struct {
	ID   uuid.UUID
	Role domain.Role
}

func (a Actor) IsAdmin() bool { return a.Role == domain.RoleAdmin }

// mayManage: admin qua hết; còn lại chỉ bản ghi của chính mình.
//
// Dùng chung cho cả API key TTS lẫn Bộ API key LLM, nên câu lỗi nói "bản ghi"
// chứ không nói "API key": người dùng bị chặn ở màn Bộ API mà đọc thấy "API key
// này thuộc về người khác" sẽ đi tìm nhầm chỗ.
func (a Actor) mayManage(ownerID uuid.UUID) error {
	if a.IsAdmin() || a.ID == ownerID {
		return nil
	}
	return fmt.Errorf("%w: bản ghi này thuộc về người khác", domain.ErrForbidden)
}

// AIEngineService quản lý API key TTS.
type AIEngineService struct {
	q *repository.Queries
	// box mã hoá API key trước khi ghi DB — cùng khoá với token multime.
	box *secret.Box
}

func NewAIEngineService(q *repository.Queries, box *secret.Box) *AIEngineService {
	return &AIEngineService{q: q, box: box}
}

// AIEngineCreate là input thêm key.
type AIEngineCreate struct {
	APIKey string
	// Owners là những người được gán key này; rỗng = gán cho chính người đang
	// thao tác. Chỉ admin gán được cho người khác.
	//
	// Nhận nhiều người vì admin thường mua 1 key rồi phát cho cả nhóm: mỗi
	// người nhận một bản ghi riêng để sau này đổi/thu hồi key của từng người mà
	// không đụng tới những người còn lại.
	Owners []uuid.UUID
}

// AIEngineUpdate là input sửa key — cả hai trường đều tuỳ chọn.
type AIEngineUpdate struct {
	// APIKey rỗng = giữ key cũ: form không hiển thị key thật nên không có gì
	// để gửi lại.
	APIKey string
	// Owner khác nil = gán key cho người khác. Chỉ admin làm được.
	Owner *uuid.UUID
}

// AIEngineView là bản trả ra API của ai_engine.
//
// Không trả thẳng repository.AiEngine vì bản ghi đó có cột api_key_encrypted:
// trả nguyên struct là đưa ciphertext ra ngoài. Ở đây chỉ có bản che 4 ký tự
// cuối — đủ để người dùng nhận ra mình đã dán đúng key nào.
type AIEngineView struct {
	ID uuid.UUID `json:"id"`
	// UserID/UserEmail — CHỦ SỞ HỮU: voice của người này đọc bằng key này.
	UserID    uuid.UUID `json:"user_id"`
	UserEmail string    `json:"user_email"`
	// CreatedBy/CreatedByEmail — NGƯỜI KHAI (admin khai hộ thì khác chủ).
	CreatedBy      uuid.UUID  `json:"created_by"`
	CreatedByEmail string     `json:"created_by_email"`
	APIKeyMasked   string     `json:"api_key_masked"`
	CreatedAt      time.Time  `json:"created_at"`
	LastUsedAt     *time.Time `json:"last_used_at"`
}

// Create thêm key. Admin gán được cho nhiều người một lượt (mỗi người 1 bản
// ghi); các vai trò khác luôn tạo cho chính mình, bất kể gửi lên gì.
func (s *AIEngineService) Create(
	ctx context.Context,
	actor Actor,
	in AIEngineCreate,
) ([]AIEngineView, error) {
	encrypted, err := s.encryptKey(in.APIKey, true)
	if err != nil {
		return nil, err
	}

	owners, err := s.resolveOwners(ctx, actor, in.Owners)
	if err != nil {
		return nil, err
	}
	author, err := s.q.GetUserByID(ctx, actor.ID)
	if err != nil {
		return nil, wrapNotFound(err, "app_user "+actor.ID.String())
	}

	out := make([]AIEngineView, 0, len(owners))
	for _, owner := range owners {
		engine, err := s.q.CreateAIEngine(ctx, repository.CreateAIEngineParams{
			UserID:          owner.ID,
			ApiKeyEncrypted: *encrypted,
			CreatedBy:       actor.ID,
		})
		if err != nil {
			return nil, fmt.Errorf("tạo ai_engine: %w", err)
		}
		out = append(out, s.view(engine, owner.Email, author.Email))
	}
	return out, nil
}

func (s *AIEngineService) Get(ctx context.Context, actor Actor, id uuid.UUID) (AIEngineView, error) {
	row, err := s.q.GetAIEngine(ctx, id)
	if err != nil {
		return AIEngineView{}, wrapNotFound(err, "ai_engine "+id.String())
	}
	if err := actor.mayManage(row.UserID); err != nil {
		return AIEngineView{}, err
	}
	return s.view(engineOf(row), row.UserEmail, row.CreatedByEmail), nil
}

// List: admin xem được của tất cả (lọc thêm bằng `owner` nếu muốn), các vai
// trò khác luôn bị ép về chính mình.
func (s *AIEngineService) List(
	ctx context.Context,
	actor Actor,
	owner *uuid.UUID,
) ([]AIEngineView, error) {
	if !actor.IsAdmin() {
		owner = &actor.ID
	}
	rows, err := s.q.ListAIEngines(ctx, owner)
	if err != nil {
		return nil, fmt.Errorf("list ai_engine: %w", err)
	}

	out := make([]AIEngineView, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.view(repository.AiEngine{
			ID: r.ID, Provider: r.Provider, CreatedAt: r.CreatedAt,
			ApiKeyEncrypted: r.ApiKeyEncrypted, VoiceID: r.VoiceID,
			CreatedBy: r.CreatedBy, UserID: r.UserID, LastUsedAt: r.LastUsedAt,
		}, r.UserEmail, r.CreatedByEmail))
	}
	return out, nil
}

func (s *AIEngineService) Update(
	ctx context.Context,
	actor Actor,
	id uuid.UUID,
	in AIEngineUpdate,
) (AIEngineView, error) {
	before, err := s.q.GetAIEngine(ctx, id)
	if err != nil {
		return AIEngineView{}, wrapNotFound(err, "ai_engine "+id.String())
	}
	if err := actor.mayManage(before.UserID); err != nil {
		return AIEngineView{}, err
	}

	// Key rỗng = giữ nguyên key cũ (query dùng COALESCE), không phải xoá key.
	encrypted, err := s.encryptKey(in.APIKey, false)
	if err != nil {
		return AIEngineView{}, err
	}

	ownerEmail := before.UserEmail
	owner := in.Owner
	switch {
	case owner == nil || *owner == before.UserID:
		owner = nil // không đổi chủ
	case !actor.IsAdmin():
		return AIEngineView{}, fmt.Errorf(
			"%w: chỉ admin gán được API key cho người khác", domain.ErrForbidden)
	default:
		user, err := s.q.GetUserByID(ctx, *owner)
		if err != nil {
			return AIEngineView{}, wrapNotFound(err, "app_user "+owner.String())
		}
		ownerEmail = user.Email
	}

	engine, err := s.q.UpdateAIEngine(ctx, repository.UpdateAIEngineParams{
		ID:              id,
		ApiKeyEncrypted: encrypted,
		UserID:          owner,
	})
	if err != nil {
		return AIEngineView{}, wrapNotFound(err, "ai_engine "+id.String())
	}
	return s.view(engine, ownerEmail, before.CreatedByEmail), nil
}

func (s *AIEngineService) Delete(ctx context.Context, actor Actor, id uuid.UUID) error {
	before, err := s.q.GetAIEngine(ctx, id)
	if err != nil {
		return wrapNotFound(err, "ai_engine "+id.String())
	}
	if err := actor.mayManage(before.UserID); err != nil {
		return err
	}
	if _, err := s.q.DeleteAIEngine(ctx, id); err != nil {
		return fmt.Errorf("xoá ai_engine: %w", err)
	}
	return nil
}

// resolveOwners chốt danh sách chủ sở hữu cho key mới.
//
// Người không phải admin luôn về chính mình — không tin danh sách gửi lên, kể
// cả khi UI đã ẩn ô chọn người dùng. Admin thì người được gán phải có thật:
// FK cũng chặn, nhưng lỗi FK trả ra 500 khó hiểu, còn ở đây là 404 kèm id sai.
func (s *AIEngineService) resolveOwners(
	ctx context.Context,
	actor Actor,
	requested []uuid.UUID,
) ([]repository.AppUser, error) {
	if !actor.IsAdmin() || len(requested) == 0 {
		self, err := s.q.GetUserByID(ctx, actor.ID)
		if err != nil {
			return nil, wrapNotFound(err, "app_user "+actor.ID.String())
		}
		return []repository.AppUser{self}, nil
	}

	seen := make(map[uuid.UUID]bool, len(requested))
	owners := make([]repository.AppUser, 0, len(requested))
	for _, id := range requested {
		if seen[id] {
			continue
		}
		seen[id] = true
		user, err := s.q.GetUserByID(ctx, id)
		if err != nil {
			return nil, wrapNotFound(err, "app_user "+id.String())
		}
		owners = append(owners, user)
	}
	return owners, nil
}

// minAPIKeyRunes chỉ chặn nhầm lẫn kiểu dán thiếu (key 3voices dạng `sk-ov-…`
// dài hơn nhiều), không phải kiểm tra định dạng thật — chỉ 3voices mới biết
// key có dùng được không, và điều đó lộ ra ở lần chạy TTS đầu tiên.
const minAPIKeyRunes = 12

// encryptKey mã hoá key trước khi ghi DB. `required` phân biệt lúc tạo (bắt
// buộc có key, không thì bản ghi vô dụng) với lúc sửa (rỗng = giữ key cũ).
func (s *AIEngineService) encryptKey(raw string, required bool) (*string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if required {
			return nil, fmt.Errorf("%w: API key là bắt buộc", domain.ErrInvalidInput)
		}
		return nil, nil
	}
	if len([]rune(raw)) < minAPIKeyRunes {
		return nil, fmt.Errorf("%w: API key trông không hợp lệ (quá ngắn)", domain.ErrInvalidInput)
	}
	encrypted, err := s.box.Encrypt(raw)
	if err != nil {
		return nil, fmt.Errorf("mã hoá API key: %w", err)
	}
	return &encrypted, nil
}

func (s *AIEngineService) view(e repository.AiEngine, ownerEmail, authorEmail string) AIEngineView {
	return AIEngineView{
		ID:             e.ID,
		UserID:         e.UserID,
		UserEmail:      ownerEmail,
		CreatedBy:      e.CreatedBy,
		CreatedByEmail: authorEmail,
		APIKeyMasked:   maskSecret(s.box, e.ApiKeyEncrypted),
		CreatedAt:      e.CreatedAt,
		LastUsedAt:     e.LastUsedAt,
	}
}

// engineOf đổi row của GetAIEngine (có kèm email) về đúng struct bảng.
func engineOf(r repository.GetAIEngineRow) repository.AiEngine {
	return repository.AiEngine{
		ID: r.ID, Provider: r.Provider, CreatedAt: r.CreatedAt,
		ApiKeyEncrypted: r.ApiKeyEncrypted, VoiceID: r.VoiceID,
		CreatedBy: r.CreatedBy, UserID: r.UserID, LastUsedAt: r.LastUsedAt,
	}
}
