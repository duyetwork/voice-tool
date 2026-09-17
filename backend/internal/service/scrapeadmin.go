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

// ScrapeAdmin là mặt quản trị của hạ tầng via/proxy — thứ màn Cài đặt gọi tới.
//
// Tách khỏi ScrapePool vì hai bên có hai ràng buộc trái ngược: pool chạy trong
// worker và phải nhanh, không được lỗi; còn cái này chạy theo thao tác của
// người dùng và phải kiểm đầu vào thật chặt. Gộp lại thì đường nóng phải mang
// theo cả đống việc kiểm tra mà nó không bao giờ dùng.
//
// LUẬT XEM/SỬA, giống hệt API key TTS (xem AIEngineService): admin thấy và
// quản lý via/proxy của mọi người, kể cả khai hộ và gán lại chủ sở hữu; editor
// chỉ thấy và sửa của chính mình. Chặn ở ĐÂY chứ không chỉ ở giao diện — gọi
// thẳng API cũng không đọc được via của người khác.
//
// Vai trò `user` không vào tới đây: route nằm sau middleware.RequireOperate.
//
// KHÔNG ĐỔI: bộ chọn của worker (ScrapePool) vẫn lấy via/proxy trên TOÀN hệ
// thống, không giới hạn theo chủ sở hữu của kênh đang quét. Chủ sở hữu ở đây
// trả lời câu hỏi "ai được nhìn và sửa bản ghi này", không phải "lượt quét của
// ai được dùng via nào" — ghép hai thứ đó lại sẽ làm kênh của người chưa nuôi
// via im lặng ngừng ra bài, mà không có chỗ nào trên giao diện nói vì sao.
type ScrapeAdmin struct {
	q     *repository.Queries
	box   *secret.Box
	audit *Audit
}

func NewScrapeAdmin(q *repository.Queries, box *secret.Box, audit *Audit) *ScrapeAdmin {
	return &ScrapeAdmin{q: q, box: box, audit: audit}
}

// ---------------------------------------------------------------------------
// Kiểu trả về cho giao diện
// ---------------------------------------------------------------------------

// ViaView là 1 via ở dạng giao diện đọc được.
//
// KHÔNG CÓ TRƯỜNG NÀO CHỨA COOKIES. Đó là điều kiện đã chốt: cookie thô không
// được hiện ở bất kỳ đâu trên giao diện sau khi đã lưu. Một trường "để hiển thị
// một phần" cũng không có, vì phần nào của cookie cũng đủ để nhận ra phiên.
type ViaView struct {
	ID       uuid.UUID `json:"id"`
	Platform string    `json:"platform"`
	Label    string    `json:"label"`
	Status   string    `json:"status"`
	// UserID/UserEmail — CHỦ SỞ HỮU: người thấy và sửa được via này.
	UserID    uuid.UUID `json:"user_id"`
	UserEmail string    `json:"user_email"`
	// CreatedBy/CreatedByEmail — NGƯỜI KHAI (admin khai hộ thì khác chủ).
	CreatedBy      uuid.UUID `json:"created_by"`
	CreatedByEmail string    `json:"created_by_email"`
	// ConsecutiveLoginErrors: số lỗi "đòi đăng nhập" liên tiếp. Nhìn nó cùng
	// ngưỡng là biết via còn cách cooldown bao xa.
	ConsecutiveLoginErrors int32      `json:"consecutive_login_errors"`
	CooldownUntil          *time.Time `json:"cooldown_until"`
	DailyQuota             int32      `json:"daily_quota"`
	DailyUsed              int32      `json:"daily_used"`
	// QuotaLeft tính sẵn ở server: bộ đếm tự liền theo ngày, nên
	// `quota - used` ở phía trình duyệt sẽ ra số âm/sai vào đúng lúc giao ngày.
	QuotaLeft   int32      `json:"quota_left"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	LastErrorAt *time.Time `json:"last_error_at"`
	LastError   string     `json:"last_error"`
	CreatedAt   time.Time  `json:"created_at"`
}

// ProxyView — endpoint chỉ hiện phần đã che (không có user/pass).
type ProxyView struct {
	ID       uuid.UUID `json:"id"`
	Label    string    `json:"label"`
	Platform string    `json:"platform"`
	Endpoint string    `json:"endpoint"`
	Kind     string    `json:"kind"`
	Status   string    `json:"status"`

	UserID         uuid.UUID `json:"user_id"`
	UserEmail      string    `json:"user_email"`
	CreatedBy      uuid.UUID `json:"created_by"`
	CreatedByEmail string    `json:"created_by_email"`

	ConsecutiveBlocks int32      `json:"consecutive_blocks"`
	UsedToday         int32      `json:"used_today"`
	ErrorsToday       int32      `json:"errors_today"`
	LastUsedAt        *time.Time `json:"last_used_at"`
	LastError         string     `json:"last_error"`
	CreatedAt         time.Time  `json:"created_at"`
}

// ViaHealth là bảng tổng quan: mỗi nền tảng bao nhiêu via sống/chết.
type ViaHealth struct {
	Platform string `json:"platform"`
	Active   int64  `json:"active"`
	Cooldown int64  `json:"cooldown"`
	Dead     int64  `json:"dead"`
	Disabled int64  `json:"disabled"`
	Total    int64  `json:"total"`
	// Healthy = tỉ lệ via còn dùng được (0..1). Tính ở server để mọi chỗ hiển
	// thị dùng chung một định nghĩa "khoẻ".
	//
	// `disabled` KHÔNG nằm ở mẫu số: via bị tắt tay là via người ta chủ động
	// cất đi, đếm nó vào đây thì tắt bớt vài via là tự làm tỉ lệ tụt và bật
	// cảnh báo giả.
	Healthy float64 `json:"healthy"`
}

// ListVias: admin xem được của tất cả (lọc thêm bằng `owner` nếu muốn), các
// vai trò khác luôn bị ép về chính mình — giống AIEngineService.List.
func (a *ScrapeAdmin) ListVias(
	ctx context.Context, actor Actor, platform *string, owner *uuid.UUID,
) ([]ViaView, error) {
	if !actor.IsAdmin() {
		owner = &actor.ID
	}
	rows, err := a.q.ListScrapeVias(ctx, repository.ListScrapeViasParams{
		Platform: platform, Owner: owner,
	})
	if err != nil {
		return nil, fmt.Errorf("đọc danh sách via: %w", err)
	}
	out := make([]ViaView, 0, len(rows))
	for _, r := range rows {
		out = append(out, viaView(repository.ScrapeVia{
			ID: r.ID, Platform: r.Platform, Label: r.Label,
			CookiesEncrypted: r.CookiesEncrypted, Status: r.Status,
			ConsecutiveLoginErrors: r.ConsecutiveLoginErrors,
			CooldownUntil:          r.CooldownUntil,
			DailyQuota:             r.DailyQuota, DailyUsed: r.DailyUsed,
			DailyUsedDate: r.DailyUsedDate,
			LastUsedAt:    r.LastUsedAt, LastErrorAt: r.LastErrorAt, LastError: r.LastError,
			UserID: r.UserID, CreatedBy: r.CreatedBy,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}, r.UserEmail, r.CreatedByEmail))
	}
	return out, nil
}

func viaView(v repository.ScrapeVia, ownerEmail, authorEmail string) ViaView {
	used := v.DailyUsed
	// Bộ đếm của hôm qua không phải bộ đếm của hôm nay — xem ClaimScrapeVia.
	// Không xử ở đây thì bảng hiện "hết hạn mức" cho một via đang dùng được,
	// suốt từ nửa đêm tới lần chạy đầu tiên của job bảo trì.
	if v.DailyUsedDate.Valid && v.DailyUsedDate.Time.Before(startOfToday()) {
		used = 0
	}
	left := v.DailyQuota - used
	if left < 0 {
		left = 0
	}
	return ViaView{
		ID: v.ID, Platform: v.Platform, Label: v.Label, Status: v.Status,
		UserID: v.UserID, UserEmail: ownerEmail,
		CreatedBy: v.CreatedBy, CreatedByEmail: authorEmail,
		ConsecutiveLoginErrors: v.ConsecutiveLoginErrors,
		CooldownUntil:          v.CooldownUntil,
		DailyQuota:             v.DailyQuota,
		DailyUsed:              used,
		QuotaLeft:              left,
		LastUsedAt:             v.LastUsedAt,
		LastErrorAt:            v.LastErrorAt,
		LastError:              deref(v.LastError),
		CreatedAt:              v.CreatedAt,
	}
}

func startOfToday() time.Time {
	n := time.Now()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, n.Location())
}

// ViaInput là dữ liệu tạo/sửa một via.
type ViaInput struct {
	Platform string
	Label    string
	// Cookies rỗng khi SỬA nghĩa là giữ nguyên cookies cũ: giao diện không bao
	// giờ nhận lại được cookie thật nên không có gì để gửi lên lại.
	Cookies    string
	DailyQuota *int32
	Status     *string
	// Owner khác nil = gán via cho người này. Chỉ admin làm được; với người
	// khác giá trị bị bỏ qua khi tạo và bị từ chối khi sửa.
	//
	// MỘT chủ sở hữu chứ không phải danh sách như AIEngineCreate.Owners: một
	// API key chia cho cả nhóm chỉ là chia hạn mức, còn một via là MỘT phiên
	// đăng nhập — nhân nó ra cho nhiều người là nhân số lượt đổ lên cùng một
	// tài khoản, đúng cách giết via nhanh nhất.
	Owner *uuid.UUID
}

func (a *ScrapeAdmin) CreateVia(
	ctx context.Context, actor Actor, in ViaInput,
) (ViaView, error) {
	platform := domain.Platform(strings.ToLower(strings.TrimSpace(in.Platform)))
	if !domain.NeedsVia(platform) {
		return ViaView{}, fmt.Errorf(
			"%w: chỉ Facebook / X / Instagram mới cần via — YouTube và TikTok quét được kênh mà không cần đăng nhập",
			domain.ErrInvalidInput)
	}
	label := strings.TrimSpace(in.Label)
	if label == "" {
		return ViaView{}, fmt.Errorf("%w: via phải có tên gợi nhớ", domain.ErrInvalidInput)
	}
	cookies := strings.TrimSpace(in.Cookies)
	if cookies == "" {
		return ViaView{}, fmt.Errorf("%w: chưa dán cookies của phiên đăng nhập", domain.ErrInvalidInput)
	}
	if err := checkCookies(platform, cookies); err != nil {
		return ViaView{}, err
	}

	sealed, err := a.box.Encrypt(cookies)
	if err != nil {
		return ViaView{}, fmt.Errorf("mã hoá cookies: %w", err)
	}

	owner, author, err := a.resolveOwner(ctx, actor, in.Owner)
	if err != nil {
		return ViaView{}, err
	}

	via, err := a.q.CreateScrapeVia(ctx, repository.CreateScrapeViaParams{
		Platform:         string(platform),
		Label:            label,
		CookiesEncrypted: sealed,
		DailyQuota:       quotaOr(in.DailyQuota),
		UserID:           owner.ID,
		CreatedBy:        actor.ID,
	})
	if err != nil {
		return ViaView{}, fmt.Errorf("tạo via: %w", err)
	}

	// Audit KHÔNG ghi cookies, chỉ ghi việc đã thêm một via cho nền tảng nào.
	a.audit.Record(ctx, actor.ID, domain.AuditCreate, domain.ObjectScrapeVia, via.ID, map[string]any{
		"platform": via.Platform, "label": via.Label, "daily_quota": via.DailyQuota,
		"owner": owner.Email,
	})
	return viaView(via, owner.Email, author.Email), nil
}

func (a *ScrapeAdmin) UpdateVia(
	ctx context.Context, actor Actor, id uuid.UUID, in ViaInput,
) (ViaView, error) {
	before, err := a.q.GetScrapeVia(ctx, id)
	if err != nil {
		return ViaView{}, wrapNotFound(err, "scrape_via "+id.String())
	}
	if err := actor.mayManage(before.UserID); err != nil {
		return ViaView{}, err
	}

	owner, ownerEmail, err := a.resolveNewOwner(ctx, actor, in.Owner, before.UserID)
	if err != nil {
		return ViaView{}, err
	}
	author, err := a.q.GetUserByID(ctx, before.CreatedBy)
	if err != nil {
		return ViaView{}, wrapNotFound(err, "app_user "+before.CreatedBy.String())
	}

	params := repository.UpdateScrapeViaParams{ID: id, DailyQuota: in.DailyQuota, UserID: owner}
	if label := strings.TrimSpace(in.Label); label != "" {
		params.Label = &label
	}
	if in.Status != nil {
		st := strings.ToLower(strings.TrimSpace(*in.Status))
		// Chỉ cho đặt tay hai trạng thái. `cooldown` và `dead` là KẾT LUẬN của
		// máy trạng thái; cho phép đặt tay thì con số trên bảng tổng quan không
		// còn nói lên điều gì về sức khoẻ thật của đàn via.
		if st != domain.ViaActive && st != domain.ViaDisabled {
			return ViaView{}, fmt.Errorf(
				"%w: chỉ bật (active) hoặc tắt (disabled) được bằng tay — cooldown/dead do hệ thống tự kết luận",
				domain.ErrInvalidInput)
		}
		params.Status = &st
	}
	if cookies := strings.TrimSpace(in.Cookies); cookies != "" {
		// Nền tảng của via không đổi được, nên đọc từ bản ghi hiện có: form sửa
		// không hỏi lại nền tảng, mà bộ cookie bắt buộc thì khác nhau theo từng
		// nền tảng.
		if err := checkCookies(domain.Platform(before.Platform), cookies); err != nil {
			return ViaView{}, err
		}
		sealed, err := a.box.Encrypt(cookies)
		if err != nil {
			return ViaView{}, fmt.Errorf("mã hoá cookies: %w", err)
		}
		params.CookiesEncrypted = &sealed
	}

	via, err := a.q.UpdateScrapeVia(ctx, params)
	if err != nil {
		return ViaView{}, wrapNotFound(err, "scrape_via "+id.String())
	}
	a.audit.Record(ctx, actor.ID, domain.AuditUpdate, domain.ObjectScrapeVia, id, map[string]any{
		"label": via.Label, "status": via.Status, "daily_quota": via.DailyQuota,
		"cookies_replaced": params.CookiesEncrypted != nil,
		"owner":            ownerEmail,
	})
	return viaView(via, ownerEmail, author.Email), nil
}

func (a *ScrapeAdmin) DeleteVia(ctx context.Context, actor Actor, id uuid.UUID) error {
	// Đọc trước khi xoá để kiểm chủ sở hữu. Không có bước này thì editor xoá
	// được via của người khác chỉ cần đoán đúng id — họ không LIỆT KÊ được id
	// đó, nhưng "không liệt kê được" chưa bao giờ là một lớp bảo vệ.
	before, err := a.q.GetScrapeVia(ctx, id)
	if err != nil {
		return wrapNotFound(err, "scrape_via "+id.String())
	}
	if err := actor.mayManage(before.UserID); err != nil {
		return err
	}
	rows, err := a.q.DeleteScrapeVia(ctx, id)
	if err != nil {
		return fmt.Errorf("xoá via: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("%w: scrape_via %s", domain.ErrNotFound, id)
	}
	a.audit.Record(ctx, actor.ID, domain.AuditDelete, domain.ObjectScrapeVia, id, nil)
	return nil
}

// ---------------------------------------------------------------------------

// ProxyInput là dữ liệu tạo/sửa một proxy.
type ProxyInput struct {
	Label string
	// Platform rỗng = dùng chung cho mọi nền tảng (gateway residential).
	Platform string
	// Endpoint rỗng khi SỬA nghĩa là giữ nguyên endpoint cũ.
	Endpoint string
	Kind     string
	Status   *string
	// Owner khác nil = gán proxy cho người này. Chỉ admin — xem ViaInput.Owner.
	Owner *uuid.UUID
}

// ListProxies: admin xem của tất cả, các vai trò khác bị ép về chính mình.
func (a *ScrapeAdmin) ListProxies(
	ctx context.Context, actor Actor, platform *string, owner *uuid.UUID,
) ([]ProxyView, error) {
	if !actor.IsAdmin() {
		owner = &actor.ID
	}
	rows, err := a.q.ListScrapeProxies(ctx, repository.ListScrapeProxiesParams{
		Platform: platform, Owner: owner,
	})
	if err != nil {
		return nil, fmt.Errorf("đọc danh sách proxy: %w", err)
	}
	out := make([]ProxyView, 0, len(rows))
	for _, r := range rows {
		out = append(out, proxyView(repository.ScrapeProxy{
			ID: r.ID, Label: r.Label, Platform: r.Platform,
			EndpointEncrypted: r.EndpointEncrypted, EndpointMasked: r.EndpointMasked,
			Kind: r.Kind, Status: r.Status,
			ConsecutiveBlocks: r.ConsecutiveBlocks,
			ErrorsToday:       r.ErrorsToday, UsedToday: r.UsedToday, Today: r.Today,
			LastUsedAt: r.LastUsedAt, LastErrorAt: r.LastErrorAt, LastError: r.LastError,
			UserID: r.UserID, CreatedBy: r.CreatedBy,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}, r.UserEmail, r.CreatedByEmail))
	}
	return out, nil
}

func proxyView(p repository.ScrapeProxy, ownerEmail, authorEmail string) ProxyView {
	return ProxyView{
		ID: p.ID, Label: p.Label, Platform: deref(p.Platform),
		Endpoint: p.EndpointMasked, Kind: p.Kind, Status: p.Status,
		UserID: p.UserID, UserEmail: ownerEmail,
		CreatedBy: p.CreatedBy, CreatedByEmail: authorEmail,
		ConsecutiveBlocks: p.ConsecutiveBlocks,
		UsedToday:         p.UsedToday,
		ErrorsToday:       p.ErrorsToday,
		LastUsedAt:        p.LastUsedAt,
		LastError:         deref(p.LastError),
		CreatedAt:         p.CreatedAt,
	}
}

func (a *ScrapeAdmin) CreateProxy(
	ctx context.Context, actor Actor, in ProxyInput,
) (ProxyView, error) {
	label := strings.TrimSpace(in.Label)
	if label == "" {
		return ProxyView{}, fmt.Errorf("%w: proxy phải có tên gợi nhớ", domain.ErrInvalidInput)
	}
	endpoint := domain.NormalizeProxyEndpoint(in.Endpoint)
	masked := domain.MaskProxyEndpoint(endpoint)
	if endpoint == "" || masked == "" {
		return ProxyView{}, errBadProxyEndpoint()
	}
	kind := strings.ToLower(strings.TrimSpace(in.Kind))
	if kind == "" {
		kind = domain.ProxyResidential
	}
	if !domain.ValidProxyKind(kind) {
		return ProxyView{}, fmt.Errorf("%w: loại proxy không hợp lệ", domain.ErrInvalidInput)
	}

	sealed, err := a.box.Encrypt(endpoint)
	if err != nil {
		return ProxyView{}, fmt.Errorf("mã hoá endpoint proxy: %w", err)
	}

	owner, author, err := a.resolveOwner(ctx, actor, in.Owner)
	if err != nil {
		return ProxyView{}, err
	}

	proxy, err := a.q.CreateScrapeProxy(ctx, repository.CreateScrapeProxyParams{
		Label:             label,
		Platform:          scrapePlatformOrNil(in.Platform),
		EndpointEncrypted: sealed,
		EndpointMasked:    masked,
		Kind:              kind,
		UserID:            owner.ID,
		CreatedBy:         actor.ID,
	})
	if err != nil {
		return ProxyView{}, fmt.Errorf("tạo proxy: %w", err)
	}
	a.audit.Record(ctx, actor.ID, domain.AuditCreate, domain.ObjectScrapeProxy, proxy.ID, map[string]any{
		"label": proxy.Label, "kind": proxy.Kind, "endpoint": masked,
		"owner": owner.Email,
	})
	return proxyView(proxy, owner.Email, author.Email), nil
}

func (a *ScrapeAdmin) UpdateProxy(
	ctx context.Context, actor Actor, id uuid.UUID, in ProxyInput,
) (ProxyView, error) {
	before, err := a.q.GetScrapeProxy(ctx, id)
	if err != nil {
		return ProxyView{}, wrapNotFound(err, "scrape_proxy "+id.String())
	}
	if err := actor.mayManage(before.UserID); err != nil {
		return ProxyView{}, err
	}

	owner, ownerEmail, err := a.resolveNewOwner(ctx, actor, in.Owner, before.UserID)
	if err != nil {
		return ProxyView{}, err
	}
	author, err := a.q.GetUserByID(ctx, before.CreatedBy)
	if err != nil {
		return ProxyView{}, wrapNotFound(err, "app_user "+before.CreatedBy.String())
	}

	params := repository.UpdateScrapeProxyParams{ID: id, UserID: owner}
	if label := strings.TrimSpace(in.Label); label != "" {
		params.Label = &label
	}
	if kind := strings.ToLower(strings.TrimSpace(in.Kind)); kind != "" {
		if !domain.ValidProxyKind(kind) {
			return ProxyView{}, fmt.Errorf("%w: loại proxy không hợp lệ", domain.ErrInvalidInput)
		}
		params.Kind = &kind
	}
	if in.Status != nil {
		st := strings.ToLower(strings.TrimSpace(*in.Status))
		// `active` để bật lại một proxy đã bị hạ cấp — đây là đường DUY NHẤT
		// proxy quay lại vòng chọn, vì hệ thống cố tình không tự hồi sinh nó.
		if st != domain.ProxyActive && st != domain.ProxyDisabled {
			return ProxyView{}, fmt.Errorf(
				"%w: chỉ bật (active) hoặc tắt (disabled) được bằng tay",
				domain.ErrInvalidInput)
		}
		params.Status = &st
	}
	if strings.TrimSpace(in.Endpoint) != "" {
		endpoint := domain.NormalizeProxyEndpoint(in.Endpoint)
		masked := domain.MaskProxyEndpoint(endpoint)
		if endpoint == "" || masked == "" {
			return ProxyView{}, errBadProxyEndpoint()
		}
		sealed, err := a.box.Encrypt(endpoint)
		if err != nil {
			return ProxyView{}, fmt.Errorf("mã hoá endpoint proxy: %w", err)
		}
		params.EndpointEncrypted = &sealed
		params.EndpointMasked = &masked
	}

	proxy, err := a.q.UpdateScrapeProxy(ctx, params)
	if err != nil {
		return ProxyView{}, wrapNotFound(err, "scrape_proxy "+id.String())
	}
	a.audit.Record(ctx, actor.ID, domain.AuditUpdate, domain.ObjectScrapeProxy, id, map[string]any{
		"label": proxy.Label, "status": proxy.Status, "kind": proxy.Kind,
		"endpoint_replaced": params.EndpointEncrypted != nil,
		"owner":             ownerEmail,
	})
	return proxyView(proxy, ownerEmail, author.Email), nil
}

func (a *ScrapeAdmin) DeleteProxy(ctx context.Context, actor Actor, id uuid.UUID) error {
	before, err := a.q.GetScrapeProxy(ctx, id)
	if err != nil {
		return wrapNotFound(err, "scrape_proxy "+id.String())
	}
	if err := actor.mayManage(before.UserID); err != nil {
		return err
	}
	rows, err := a.q.DeleteScrapeProxy(ctx, id)
	if err != nil {
		return fmt.Errorf("xoá proxy: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("%w: scrape_proxy %s", domain.ErrNotFound, id)
	}
	a.audit.Record(ctx, actor.ID, domain.AuditDelete, domain.ObjectScrapeProxy, id, nil)
	return nil
}

// ---------------------------------------------------------------------------
// Bảng tổng quan
// ---------------------------------------------------------------------------

// Health đếm via theo trạng thái cho từng nền tảng cần via.
//
// Liệt kê đủ CẢ BA nền tảng kể cả khi chưa có via nào: "chưa thêm via cho X" là
// thông tin cần thấy, mà một bảng chỉ hiện những dòng có dữ liệu thì im lặng
// đúng ở chỗ đó.
func (a *ScrapeAdmin) Health(ctx context.Context, actor Actor) ([]ViaHealth, error) {
	var owner *uuid.UUID
	if !actor.IsAdmin() {
		owner = &actor.ID
	}
	rows, err := a.q.CountScrapeViasByStatus(ctx, owner)
	if err != nil {
		return nil, fmt.Errorf("đếm via theo trạng thái: %w", err)
	}

	byPlatform := make(map[string]*ViaHealth, len(domain.ScrapePlatforms))
	out := make([]ViaHealth, 0, len(domain.ScrapePlatforms))
	for _, p := range domain.ScrapePlatforms {
		out = append(out, ViaHealth{Platform: string(p)})
	}
	for i := range out {
		byPlatform[out[i].Platform] = &out[i]
	}

	for _, r := range rows {
		h, ok := byPlatform[r.Platform]
		if !ok {
			continue
		}
		switch r.Status {
		case domain.ViaActive:
			h.Active = r.Total
		case domain.ViaCooldown:
			h.Cooldown = r.Total
		case domain.ViaDead:
			h.Dead = r.Total
		case domain.ViaDisabled:
			h.Disabled = r.Total
		}
	}
	for i := range out {
		h := &out[i]
		h.Total = h.Active + h.Cooldown + h.Dead + h.Disabled
		// Mẫu số bỏ `disabled` — xem ghi chú ở ViaHealth.Healthy.
		inPlay := h.Active + h.Cooldown + h.Dead
		if inPlay > 0 {
			h.Healthy = float64(h.Active) / float64(inPlay)
		}
	}
	return out, nil
}

// ViaCookieSpec nói cho giao diện biết một nền tảng cần những cookie nào.
//
// Đi từ backend chứ không chép cứng vào frontend, cùng lý do với bộ giá trị
// "Cấu hình giọng đọc": đây là danh sách mà SERVER dùng để từ chối, nên giao
// diện phải nói đúng cái server kiểm. Hai nơi giữ hai bản thì sớm muộn form
// hướng dẫn một đằng, API từ chối một nẻo.
type ViaCookieSpec struct {
	Platform string `json:"platform"`
	// Required: cookie mang danh tính, thiếu là bị từ chối.
	Required []string `json:"required"`
	// Example: mẫu để dán vào ô nhập — đủ hình dạng thật, giá trị là giả.
	Example string `json:"example"`
	// Hint: lấy mấy cookie đó ở đâu.
	Hint string `json:"hint"`
}

// ViaCookieSpecs trả hướng dẫn dán cookies cho cả ba nền tảng.
func ViaCookieSpecs() []ViaCookieSpec {
	hint := "Mở %s trên trình duyệt đã đăng nhập → DevTools (F12) → tab " +
		"Application/Storage → Cookies → chép giá trị của %s."

	out := make([]ViaCookieSpec, 0, len(domain.ScrapePlatforms))
	for _, p := range domain.ScrapePlatforms {
		required := domain.RequiredCookies(p)
		spec := ViaCookieSpec{
			Platform: string(p),
			Required: required,
			Hint:     fmt.Sprintf(hint, viaCookieSite[p], strings.Join(required, " và ")),
			Example:  viaCookieExample[p],
		}
		out = append(out, spec)
	}
	return out
}

// viaCookieSite / viaCookieExample — phần mô tả đi kèm từng nền tảng.
//
// Mẫu để NGUYÊN hình dạng thật (độ dài, ký tự đặc trưng) nhưng giá trị là giả:
// người dán cần nhận ra "à, trông như thế này", và một mẫu quá ngắn gọn thì họ
// không đối chiếu được với thứ đang có trên tay.
var (
	viaCookieSite = map[domain.Platform]string{
		domain.PlatformFacebook:  "facebook.com",
		domain.PlatformX:         "x.com",
		domain.PlatformInstagram: "instagram.com",
	}
	viaCookieExample = map[domain.Platform]string{
		domain.PlatformFacebook: "c_user=100012345678901; " +
			"xs=42%3AAbCdEfGh1234%3A2%3A1700000000%3A-1%3A-1; " +
			"datr=AbCdEfGhIjKlMnOpQrStUvWx",
		domain.PlatformX: "auth_token=a1b2c3d4e5f60718293a4b5c6d7e8f9012345678; " +
			"ct0=9f8e7d6c5b4a39281706f5e4d3c2b1a09f8e7d6c5b4a39281706f5e4d3c2b1a0",
		domain.PlatformInstagram: "sessionid=12345678901%3AAbCdEfGhIjKlMn%3A12%3AAbCd...; " +
			"ds_user_id=12345678901; csrftoken=AbCdEfGhIjKlMnOpQrStUvWxYz012345",
	}
)

// ScrapeHourRow là 1 cột của biểu đồ "số lượt quét theo giờ trong ngày".
type ScrapeHourRow struct {
	Hour     int32  `json:"hour"`
	Platform string `json:"platform"`
	Total    int64  `json:"total"`
	Failed   int64  `json:"failed"`
}

// HourlyLoad trả phân bổ lượt quét theo giờ — dùng để kiểm tra lịch quét có bị
// dồn cục hay không.
func (a *ScrapeAdmin) HourlyLoad(
	ctx context.Context, actor Actor, days int32,
) ([]ScrapeHourRow, error) {
	if days <= 0 {
		days = 7
	}
	var owner *uuid.UUID
	if !actor.IsAdmin() {
		owner = &actor.ID
	}
	rows, err := a.q.ScrapeHourlyLoad(ctx, repository.ScrapeHourlyLoadParams{
		Days: days, Owner: owner,
	})
	if err != nil {
		return nil, fmt.Errorf("đọc phân bổ lượt quét theo giờ: %w", err)
	}
	out := make([]ScrapeHourRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, ScrapeHourRow{
			Hour: r.Hour, Platform: r.Platform, Total: r.Total, Failed: r.Failed,
		})
	}
	return out, nil
}

// ---------------------------------------------------------------------------

// resolveOwner quyết định CHỦ SỞ HỮU của một bản ghi vừa tạo, và trả về cả
// người khai để giao diện hiện được cả hai.
//
// Không phải admin thì `requested` bị BỎ QUA chứ không bị từ chối: form của
// editor không có ô chọn người, nên một giá trị lọt lên chỉ có thể là rác từ
// client cũ — chặn nó bằng lỗi 403 chỉ làm hỏng một thao tác hợp lệ. Cùng cách
// xử lý với AIEngineService.resolveOwners. Lúc SỬA thì ngược lại, xem
// resolveNewOwner: ở đó một chủ sở hữu khác là thay đổi người dùng cố ý yêu
// cầu, và im lặng nuốt nó sẽ báo "đã lưu" cho một việc không hề xảy ra.
func (a *ScrapeAdmin) resolveOwner(
	ctx context.Context, actor Actor, requested *uuid.UUID,
) (owner repository.AppUser, author repository.AppUser, err error) {
	author, err = a.q.GetUserByID(ctx, actor.ID)
	if err != nil {
		return owner, author, wrapNotFound(err, "app_user "+actor.ID.String())
	}
	if !actor.IsAdmin() || requested == nil || *requested == actor.ID {
		return author, author, nil
	}
	owner, err = a.q.GetUserByID(ctx, *requested)
	if err != nil {
		return owner, author, wrapNotFound(err, "app_user "+requested.String())
	}
	return owner, author, nil
}

// resolveNewOwner xử lý ô "gán cho người khác" lúc SỬA.
//
// Trả về (giá trị ghi xuống DB — nil nghĩa là giữ nguyên, email của chủ sau khi
// sửa). Tách khỏi resolveOwner vì ở đây phải đọc được email của chủ CŨ khi
// không đổi chủ, mà không đi thêm một lượt truy vấn cho trường hợp thường gặp.
func (a *ScrapeAdmin) resolveNewOwner(
	ctx context.Context, actor Actor, requested *uuid.UUID, current uuid.UUID,
) (*uuid.UUID, string, error) {
	if requested == nil || *requested == current {
		user, err := a.q.GetUserByID(ctx, current)
		if err != nil {
			return nil, "", wrapNotFound(err, "app_user "+current.String())
		}
		return nil, user.Email, nil
	}
	if !actor.IsAdmin() {
		return nil, "", fmt.Errorf(
			"%w: chỉ admin gán được via/proxy cho người khác", domain.ErrForbidden)
	}
	user, err := a.q.GetUserByID(ctx, *requested)
	if err != nil {
		return nil, "", wrapNotFound(err, "app_user "+requested.String())
	}
	return requested, user.Email, nil
}

// checkCookies chặn via thiếu cookie mang danh tính ngay lúc dán.
//
// Báo tại đây chứ không để vòng quét phát hiện: via thiếu cookie vẫn lưu được,
// vẫn nằm trong vòng xoay, và mãi tới lượt quét đầu tiên mới lộ ra — lúc đó nó
// đã kéo theo một kênh thất bại và một dòng "đòi đăng nhập" trông y hệt một via
// thật sự hết hạn.
func checkCookies(platform domain.Platform, cookies string) error {
	missing := domain.MissingCookies(platform, cookies)
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("%w: chuỗi cookies thiếu %s — đây là cookie mang danh tính phiên, thiếu nó thì %s coi như chưa đăng nhập",
		domain.ErrInvalidInput, strings.Join(missing, ", "), platform)
}

// errBadProxyEndpoint nói ra CẢ BA dạng nhận được, vì dạng người dùng hay có
// sẵn (`ip:port:user:pass` từ nhà bán proxy) lại là dạng ít ai nghĩ là hợp lệ.
func errBadProxyEndpoint() error {
	return fmt.Errorf(
		"%w: endpoint proxy phải ở một trong ba dạng — "+
			"`ip:port:user:pass`, `ip:port`, hoặc URL đầy đủ `http://user:pass@ip:port`",
		domain.ErrInvalidInput)
}

// quotaOr: hạn mức mặc định 50 lượt/ngày cho một via mới.
//
// 50 vì quy mô đã chốt là vài trăm kênh quét MỘT LẦN mỗi ngày: 6 via là đủ cho
// 300 kênh, và con số nhỏ buộc người vận hành nuôi nhiều via thay vì vắt kiệt
// một cái — đúng thứ giữ cho cả đàn sống lâu.
func quotaOr(v *int32) int32 {
	if v == nil || *v <= 0 {
		return 50
	}
	return *v
}

// scrapePlatformOrNil: rỗng = proxy dùng chung mọi nền tảng.
func scrapePlatformOrNil(p string) *string {
	p = strings.ToLower(strings.TrimSpace(p))
	if p == "" {
		return nil
	}
	return &p
}
