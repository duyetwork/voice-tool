package service

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/pkg/secret"
	"github.com/strongbody/voice-tool/backend/internal/repository"
)

// Bộ API key LLM (tab "LLM Model" của màn AI Engine).
//
// Khác hẳn ai_engine dù nhìn qua thì giống: ai_engine là 1 key của 1 người cho
// 1 nhà (TTS). Bộ API LLM là 1 TÚI KEY của NHIỀU nhà, dùng chung cho NHIỀU
// người — vì chuỗi dự phòng chỉ có ý nghĩa khi trong tay có key của nhiều nhà
// cùng lúc.
//
// Luật xem/sửa, chặn ở service chứ không chỉ ở UI:
//   - Xem:   bộ mình tạo + bộ được chia sẻ + bộ admin đã bật `visible_to_users`.
//   - Sửa:   người tạo hoặc admin.
//   - Toggle `visible_to_users`: CHỈ admin — bật nó lên là mở hạn mức của một
//     nhóm cho toàn bộ hệ thống dùng.
type LLMAPISetService struct {
	q *repository.Queries
	// box mã hoá API key trước khi ghi DB — cùng khoá với ai_engine.
	box *secret.Box
}

func NewLLMAPISetService(q *repository.Queries, box *secret.Box) *LLMAPISetService {
	return &LLMAPISetService{q: q, box: box}
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// LLMAPIKeyView là 1 key trong bộ. Không bao giờ mang key thật — chỉ bản che 4
// ký tự cuối, đủ để người dùng nhận ra mình đã dán đúng key nào.
type LLMAPIKeyView struct {
	ID           uuid.UUID `json:"id"`
	SetID        uuid.UUID `json:"set_id"`
	Provider     string    `json:"provider"`
	Label        *string   `json:"label"`
	Priority     int32     `json:"priority"`
	APIKeyMasked string    `json:"api_key_masked"`

	// Health gộp 3 cột sức khoẻ thành một chữ cho UI: ok | cooldown | disabled.
	// Tách disabled khỏi cooldown vì hai thứ này người dùng phải xử lý khác
	// nhau hoàn toàn — một cái cần dán key mới, một cái chỉ cần chờ.
	Health              string     `json:"health"`
	DisabledAt          *time.Time `json:"disabled_at"`
	CooldownUntil       *time.Time `json:"cooldown_until"`
	ConsecutiveFailures int32      `json:"consecutive_failures"`
	LastUsedAt          *time.Time `json:"last_used_at"`
	LastError           *string    `json:"last_error"`
	CreatedAt           time.Time  `json:"created_at"`
}

// LLMAPISetUserView — 1 người được dùng chung bộ.
type LLMAPISetUserView struct {
	UserID uuid.UUID `json:"user_id"`
	Email  string    `json:"email"`
}

type LLMAPISetView struct {
	ID             uuid.UUID `json:"id"`
	Name           string    `json:"name"`
	Note           *string   `json:"note"`
	VisibleToUsers bool      `json:"visible_to_users"`

	CreatedBy      uuid.UUID  `json:"created_by"`
	CreatedByEmail string     `json:"created_by_email"`
	CreatedAt      time.Time  `json:"created_at"`
	LastUsedAt     *time.Time `json:"last_used_at"`

	// KeyCounts: số key theo từng nhà, để bảng danh sách hiện được "gemini 2,
	// openai 1" mà không phải tải toàn bộ key về.
	KeyCounts map[string]int64 `json:"key_counts"`
	// Users: ai đang dùng chung bộ này.
	Users []LLMAPISetUserView `json:"users"`
	// CanManage: người đang xem có sửa được bộ này không. Bộ `visible_to_users`
	// thì ai cũng THẤY nhưng chỉ chủ/admin mới SỬA — UI cần biết để ẩn nút.
	CanManage bool `json:"can_manage"`

	// Keys chỉ có ở Get (mở 1 bộ ra), không có ở danh sách.
	Keys []LLMAPIKeyView `json:"keys,omitempty"`
}

// ---------------------------------------------------------------------------
// Bộ
// ---------------------------------------------------------------------------

type LLMAPISetCreate struct {
	Name string
	Note string
	// VisibleToUsers chỉ admin đặt được; người khác gửi lên gì cũng thành false.
	VisibleToUsers bool
	// Users: chia sẻ bộ cho ai ngay lúc tạo.
	Users []uuid.UUID
	// Keys: tạo bộ rỗng rồi thêm key sau cũng được, nhưng thêm luôn ở đây thì
	// người dùng không phải đi qua 2 hộp thoại cho một việc.
	Keys []LLMAPIKeyInput
}

type LLMAPIKeyInput struct {
	Provider string
	APIKey   string
	Label    string
	Priority int32
}

func (s *LLMAPISetService) Create(
	ctx context.Context,
	actor Actor,
	in LLMAPISetCreate,
) (LLMAPISetView, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return LLMAPISetView{}, fmt.Errorf("%w: tên bộ API là bắt buộc", domain.ErrInvalidInput)
	}

	set, err := s.q.CreateLLMAPISet(ctx, repository.CreateLLMAPISetParams{
		Name: name,
		Note: nilIfEmpty(strings.TrimSpace(in.Note)),
		// Chỉ admin mới mở bộ ra cho cả hệ thống. Không tin cờ gửi lên, kể cả
		// khi UI đã ẩn ô đó.
		VisibleToUsers: in.VisibleToUsers && actor.IsAdmin(),
		CreatedBy:      actor.ID,
	})
	if err != nil {
		return LLMAPISetView{}, wrapDuplicateName(err, name)
	}

	if err := s.replaceUsers(ctx, set.ID, in.Users); err != nil {
		return LLMAPISetView{}, err
	}
	for _, k := range in.Keys {
		if _, err := s.addKey(ctx, set.ID, k); err != nil {
			return LLMAPISetView{}, err
		}
	}
	return s.Get(ctx, actor, set.ID)
}

type LLMAPISetUpdate struct {
	Name           *string
	Note           *string
	VisibleToUsers *bool
	// Users khác nil = thay TOÀN BỘ danh sách chia sẻ (không phải thêm dồn):
	// form gửi lên đúng những ai được tick, nên bỏ tick phải là gỡ quyền.
	Users *[]uuid.UUID
}

func (s *LLMAPISetService) Update(
	ctx context.Context,
	actor Actor,
	id uuid.UUID,
	in LLMAPISetUpdate,
) (LLMAPISetView, error) {
	set, err := s.q.GetLLMAPISet(ctx, id)
	if err != nil {
		return LLMAPISetView{}, wrapNotFound(err, "llm_api_set "+id.String())
	}
	if err := actor.mayManage(set.CreatedBy); err != nil {
		return LLMAPISetView{}, err
	}

	visible := in.VisibleToUsers
	if visible != nil && !actor.IsAdmin() {
		return LLMAPISetView{}, fmt.Errorf(
			"%w: chỉ admin bật/tắt được hiển thị bộ API cho người khác", domain.ErrForbidden)
	}

	var name *string
	if in.Name != nil {
		trimmed := strings.TrimSpace(*in.Name)
		if trimmed == "" {
			return LLMAPISetView{}, fmt.Errorf("%w: tên bộ API là bắt buộc", domain.ErrInvalidInput)
		}
		name = &trimmed
	}

	if _, err := s.q.UpdateLLMAPISet(ctx, repository.UpdateLLMAPISetParams{
		ID: id, Name: name, Note: in.Note, VisibleToUsers: visible,
	}); err != nil {
		return LLMAPISetView{}, wrapDuplicateName(err, deref(name))
	}

	if in.Users != nil {
		if err := s.replaceUsers(ctx, id, *in.Users); err != nil {
			return LLMAPISetView{}, err
		}
	}
	return s.Get(ctx, actor, id)
}

func (s *LLMAPISetService) Delete(ctx context.Context, actor Actor, id uuid.UUID) error {
	set, err := s.q.GetLLMAPISet(ctx, id)
	if err != nil {
		return wrapNotFound(err, "llm_api_set "+id.String())
	}
	if err := actor.mayManage(set.CreatedBy); err != nil {
		return err
	}
	// Key con đi theo (ON DELETE CASCADE); voice đã sinh ra thì giữ nguyên, chỉ
	// mất con trỏ tới bộ (ON DELETE SET NULL) — xem migration 000016.
	if _, err := s.q.DeleteLLMAPISet(ctx, id); err != nil {
		return fmt.Errorf("xoá llm_api_set: %w", err)
	}
	return nil
}

func (s *LLMAPISetService) Get(
	ctx context.Context,
	actor Actor,
	id uuid.UUID,
) (LLMAPISetView, error) {
	row, err := s.q.GetLLMAPISet(ctx, id)
	if err != nil {
		return LLMAPISetView{}, wrapNotFound(err, "llm_api_set "+id.String())
	}

	users, err := s.usersOf(ctx, []uuid.UUID{id})
	if err != nil {
		return LLMAPISetView{}, err
	}
	if err := s.mayView(actor, row, users[id]); err != nil {
		return LLMAPISetView{}, err
	}

	keys, err := s.q.ListLLMAPIKeys(ctx, id)
	if err != nil {
		return LLMAPISetView{}, fmt.Errorf("đọc key của bộ API: %w", err)
	}

	view := s.view(repository.LlmApiSet{
		ID: row.ID, Name: row.Name, Note: row.Note, VisibleToUsers: row.VisibleToUsers,
		CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt, LastUsedAt: row.LastUsedAt,
	}, row.CreatedByEmail, actor)
	view.Users = users[id]
	view.KeyCounts = map[string]int64{}
	view.Keys = []LLMAPIKeyView{}
	for _, k := range keys {
		view.KeyCounts[k.Provider]++
		view.Keys = append(view.Keys, s.keyView(k))
	}
	return view, nil
}

// List trả về các bộ người đang xem được phép thấy. Admin thấy tất cả.
func (s *LLMAPISetService) List(ctx context.Context, actor Actor) ([]LLMAPISetView, error) {
	var viewer *uuid.UUID
	if !actor.IsAdmin() {
		viewer = &actor.ID
	}
	rows, err := s.q.ListLLMAPISets(ctx, viewer)
	if err != nil {
		return nil, fmt.Errorf("list llm_api_set: %w", err)
	}

	ids := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}

	users, err := s.usersOf(ctx, ids)
	if err != nil {
		return nil, err
	}
	counts, err := s.keyCountsOf(ctx, ids)
	if err != nil {
		return nil, err
	}

	out := make([]LLMAPISetView, 0, len(rows))
	for _, r := range rows {
		view := s.view(repository.LlmApiSet{
			ID: r.ID, Name: r.Name, Note: r.Note, VisibleToUsers: r.VisibleToUsers,
			CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt, LastUsedAt: r.LastUsedAt,
		}, r.CreatedByEmail, actor)
		view.Users = users[r.ID]
		view.KeyCounts = counts[r.ID]
		out = append(out, view)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Key trong bộ
// ---------------------------------------------------------------------------

func (s *LLMAPISetService) AddKey(
	ctx context.Context,
	actor Actor,
	setID uuid.UUID,
	in LLMAPIKeyInput,
) (LLMAPIKeyView, error) {
	if err := s.assertManage(ctx, actor, setID); err != nil {
		return LLMAPIKeyView{}, err
	}
	key, err := s.addKey(ctx, setID, in)
	if err != nil {
		return LLMAPIKeyView{}, err
	}
	return s.keyView(key), nil
}

type LLMAPIKeyUpdate struct {
	Provider *string
	// APIKey rỗng = giữ key cũ (form không hiển thị key thật nên không có gì để
	// gửi lại). Dán key mới thì query cũng RESET luôn sức khoẻ — xem
	// UpdateLLMAPIKey trong queries/llm_api_set.sql.
	APIKey   string
	Label    *string
	Priority *int32
}

func (s *LLMAPISetService) UpdateKey(
	ctx context.Context,
	actor Actor,
	keyID uuid.UUID,
	in LLMAPIKeyUpdate,
) (LLMAPIKeyView, error) {
	before, err := s.q.GetLLMAPIKey(ctx, keyID)
	if err != nil {
		return LLMAPIKeyView{}, wrapNotFound(err, "llm_api_key "+keyID.String())
	}
	if err := s.assertManage(ctx, actor, before.SetID); err != nil {
		return LLMAPIKeyView{}, err
	}

	if in.Provider != nil {
		if !domain.LLMProviderName(*in.Provider).Valid() {
			return LLMAPIKeyView{}, fmt.Errorf(
				"%w: nhà cung cấp LLM %q không hợp lệ", domain.ErrInvalidInput, *in.Provider)
		}
	}
	encrypted, err := s.encryptKey(in.APIKey, false)
	if err != nil {
		return LLMAPIKeyView{}, err
	}

	key, err := s.q.UpdateLLMAPIKey(ctx, repository.UpdateLLMAPIKeyParams{
		ID:              keyID,
		Provider:        in.Provider,
		ApiKeyEncrypted: encrypted,
		Label:           in.Label,
		Priority:        in.Priority,
	})
	if err != nil {
		return LLMAPIKeyView{}, wrapNotFound(err, "llm_api_key "+keyID.String())
	}
	return s.keyView(key), nil
}

func (s *LLMAPISetService) DeleteKey(ctx context.Context, actor Actor, keyID uuid.UUID) error {
	before, err := s.q.GetLLMAPIKey(ctx, keyID)
	if err != nil {
		return wrapNotFound(err, "llm_api_key "+keyID.String())
	}
	if err := s.assertManage(ctx, actor, before.SetID); err != nil {
		return err
	}
	if _, err := s.q.DeleteLLMAPIKey(ctx, keyID); err != nil {
		return fmt.Errorf("xoá llm_api_key: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------

func (s *LLMAPISetService) addKey(
	ctx context.Context,
	setID uuid.UUID,
	in LLMAPIKeyInput,
) (repository.LlmApiKey, error) {
	provider := domain.LLMProviderName(strings.TrimSpace(in.Provider))
	if !provider.Valid() {
		return repository.LlmApiKey{}, fmt.Errorf(
			"%w: nhà cung cấp LLM %q không hợp lệ", domain.ErrInvalidInput, in.Provider)
	}
	encrypted, err := s.encryptKey(in.APIKey, true)
	if err != nil {
		return repository.LlmApiKey{}, err
	}

	key, err := s.q.CreateLLMAPIKey(ctx, repository.CreateLLMAPIKeyParams{
		SetID:           setID,
		Provider:        string(provider),
		ApiKeyEncrypted: *encrypted,
		Label:           nilIfEmpty(strings.TrimSpace(in.Label)),
		Priority:        in.Priority,
	})
	if err != nil {
		return repository.LlmApiKey{}, fmt.Errorf("tạo llm_api_key: %w", err)
	}
	return key, nil
}

// assertManage: chỉ người tạo bộ hoặc admin mới đụng được vào key bên trong.
func (s *LLMAPISetService) assertManage(ctx context.Context, actor Actor, setID uuid.UUID) error {
	set, err := s.q.GetLLMAPISet(ctx, setID)
	if err != nil {
		return wrapNotFound(err, "llm_api_set "+setID.String())
	}
	return actor.mayManage(set.CreatedBy)
}

// mayView: được chia sẻ thì XEM được (để biết bộ có còn key khoẻ không), nhưng
// không sửa được — CanManage trong view nói rõ điều đó.
func (s *LLMAPISetService) mayView(
	actor Actor,
	row repository.GetLLMAPISetRow,
	shared []LLMAPISetUserView,
) error {
	if actor.IsAdmin() || row.VisibleToUsers || row.CreatedBy == actor.ID {
		return nil
	}
	if slices.ContainsFunc(shared, func(u LLMAPISetUserView) bool { return u.UserID == actor.ID }) {
		return nil
	}
	return fmt.Errorf("%w: bộ API này chưa được chia sẻ cho bạn", domain.ErrForbidden)
}

func (s *LLMAPISetService) replaceUsers(
	ctx context.Context,
	setID uuid.UUID,
	users []uuid.UUID,
) error {
	if err := s.q.ClearLLMAPISetUsers(ctx, setID); err != nil {
		return fmt.Errorf("xoá chia sẻ cũ của bộ API: %w", err)
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range users {
		if seen[id] {
			continue
		}
		seen[id] = true
		if err := s.q.AddLLMAPISetUser(ctx, repository.AddLLMAPISetUserParams{
			SetID: setID, UserID: id,
		}); err != nil {
			return wrapNotFound(err, "app_user "+id.String())
		}
	}
	return nil
}

// usersOf trả về map ĐÃ ĐIỀN SẴN mảng rỗng cho mọi bộ được hỏi.
//
// Bộ chưa chia sẻ cho ai mà trả nil thì JSON ra `"users": null`, và phía UI
// `set.users.length` là lỗi runtime — trong khi "chưa chia sẻ cho ai" là trạng
// thái bình thường nhất của một bộ mới tạo.
func (s *LLMAPISetService) usersOf(
	ctx context.Context,
	ids []uuid.UUID,
) (map[uuid.UUID][]LLMAPISetUserView, error) {
	out := make(map[uuid.UUID][]LLMAPISetUserView, len(ids))
	for _, id := range ids {
		out[id] = []LLMAPISetUserView{}
	}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.q.ListLLMAPISetUsers(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("đọc người dùng chung bộ API: %w", err)
	}
	for _, r := range rows {
		out[r.SetID] = append(out[r.SetID], LLMAPISetUserView{UserID: r.UserID, Email: r.Email})
	}
	return out, nil
}

func (s *LLMAPISetService) keyCountsOf(
	ctx context.Context,
	ids []uuid.UUID,
) (map[uuid.UUID]map[string]int64, error) {
	out := make(map[uuid.UUID]map[string]int64, len(ids))
	for _, id := range ids {
		out[id] = map[string]int64{}
	}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.q.CountLLMAPIKeysBySet(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("đếm key của bộ API: %w", err)
	}
	for _, r := range rows {
		out[r.SetID][r.Provider] = r.Total
	}
	return out, nil
}

func (s *LLMAPISetService) view(
	set repository.LlmApiSet,
	authorEmail string,
	actor Actor,
) LLMAPISetView {
	return LLMAPISetView{
		ID:             set.ID,
		Name:           set.Name,
		Note:           set.Note,
		VisibleToUsers: set.VisibleToUsers,
		CreatedBy:      set.CreatedBy,
		CreatedByEmail: authorEmail,
		CreatedAt:      set.CreatedAt,
		LastUsedAt:     set.LastUsedAt,
		KeyCounts:      map[string]int64{},
		Users:          []LLMAPISetUserView{},
		CanManage:      actor.IsAdmin() || set.CreatedBy == actor.ID,
	}
}

func (s *LLMAPISetService) keyView(k repository.LlmApiKey) LLMAPIKeyView {
	return LLMAPIKeyView{
		ID:                  k.ID,
		SetID:               k.SetID,
		Provider:            k.Provider,
		Label:               k.Label,
		Priority:            k.Priority,
		APIKeyMasked:        maskSecret(s.box, k.ApiKeyEncrypted),
		Health:              keyHealth(k),
		DisabledAt:          k.DisabledAt,
		CooldownUntil:       k.CooldownUntil,
		ConsecutiveFailures: k.ConsecutiveFailures,
		LastUsedAt:          k.LastUsedAt,
		LastError:           k.LastError,
		CreatedAt:           k.CreatedAt,
	}
}

// keyHealth gộp 3 cột sức khoẻ thành 1 chữ. Cooldown ĐÃ QUA thì lại là `ok`:
// cột cooldown_until không được xoá sau khi hết hạn (router chỉ so với now()),
// nên đọc thẳng cột đó sẽ hiện "đang nghỉ" cho một key đã khoẻ lại từ lâu.
func keyHealth(k repository.LlmApiKey) string {
	switch {
	case k.DisabledAt != nil:
		return "disabled"
	case k.CooldownUntil != nil && k.CooldownUntil.After(time.Now()):
		return "cooldown"
	default:
		return "ok"
	}
}

// encryptKey mã hoá key trước khi ghi DB. `required` phân biệt lúc tạo (bắt
// buộc có key) với lúc sửa (rỗng = giữ key cũ). Cùng ngưỡng độ dài với
// AIEngineService: chỉ chặn dán thiếu, không kiểm tra định dạng thật.
func (s *LLMAPISetService) encryptKey(raw string, required bool) (*string, error) {
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

// wrapDuplicateName đổi lỗi unique của cột name thành câu người dùng hiểu
// được: "llm_api_set_name_key" không nói với ai điều gì.
func wrapDuplicateName(err error, name string) error {
	if err == nil {
		return nil
	}
	if isUniqueViolation(err) {
		return fmt.Errorf("%w: đã có bộ API tên %q", domain.ErrInvalidInput, name)
	}
	return fmt.Errorf("ghi llm_api_set: %w", err)
}
