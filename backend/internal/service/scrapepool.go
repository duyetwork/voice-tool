package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/pkg/secret"
	"github.com/strongbody/voice-tool/backend/internal/repository"
)

// ---------------------------------------------------------------------------
// Bộ cấp phát via/proxy
// ---------------------------------------------------------------------------

// ScrapeLimits là các ngưỡng của máy trạng thái via/proxy, lấy từ .env.
type ScrapeLimits struct {
	// ViaLoginErrors: bao nhiêu lỗi `login_required` LIÊN TIẾP thì via vào
	// cooldown. Mặc định 3.
	//
	// Không phải 1: một lỗi đòi đăng nhập lẻ tẻ có thể do chính bài đó bị giới
	// hạn chứ không phải phiên hỏng, và cất via đi vì một bài như vậy là vứt
	// một tài khoản còn tốt.
	ViaLoginErrors int
	// ViaCooldown: via nghỉ bao lâu trước khi được thử lại. Mặc định 6h.
	ViaCooldown time.Duration
	// ProxyBlocks: bao nhiêu lần `bot_block` LIÊN TIẾP thì proxy bị hạ cấp.
	ProxyBlocks int
}

func (l ScrapeLimits) withDefaults() ScrapeLimits {
	if l.ViaLoginErrors <= 0 {
		l.ViaLoginErrors = 3
	}
	if l.ViaCooldown <= 0 {
		l.ViaCooldown = 6 * time.Hour
	}
	if l.ProxyBlocks <= 0 {
		l.ProxyBlocks = 3
	}
	return l
}

// scrapeStore là phần repository mà bộ cấp phát dùng tới.
//
// Khai hẹp ở phía NGƯỜI DÙNG thay vì nhận cả repository.Querier (hơn 200
// phương thức): quyết định "lỗi loại này thì gọi câu nào" là phần dễ sai nhất
// và đáng test nhất của tệp này, mà giả lập một interface 200 phương thức thì
// không ai viết nổi. Hẹp thế này thì bản giả chỉ vài chục dòng.
type scrapeStore interface {
	ClaimScrapeVia(ctx context.Context, platform string) (repository.ScrapeVia, error)
	UpdateScrapeVia(ctx context.Context, arg repository.UpdateScrapeViaParams) (repository.ScrapeVia, error)
	MarkScrapeViaUsed(ctx context.Context, id uuid.UUID) error
	MarkScrapeViaLoginError(ctx context.Context, arg repository.MarkScrapeViaLoginErrorParams) (repository.ScrapeVia, error)
	NoteScrapeViaError(ctx context.Context, arg repository.NoteScrapeViaErrorParams) error

	ClaimScrapeProxy(ctx context.Context, platform *string) (repository.ScrapeProxy, error)
	MarkScrapeProxyUsed(ctx context.Context, id uuid.UUID) error
	MarkScrapeProxyBlocked(ctx context.Context, arg repository.MarkScrapeProxyBlockedParams) (repository.ScrapeProxy, error)
	NoteScrapeProxyError(ctx context.Context, arg repository.NoteScrapeProxyErrorParams) error

	CreateViaUsageLog(ctx context.Context, arg repository.CreateViaUsageLogParams) error
}

// ScrapePool cấp (via, proxy) cho từng lượt quét và giữ máy trạng thái của
// chúng.
//
// Đây là nơi DUY NHẤT trạng thái via/proxy đổi. Adapter chỉ nhận một phiên, bảo
// nó chạy, rồi trả kết quả về — không adapter nào được tự quyết một via đã chết
// hay chưa, vì mỗi adapter chỉ nhìn thấy một lượt gọi còn kết luận đó cần nhìn
// cả chuỗi.
type ScrapePool struct {
	q      scrapeStore
	box    *secret.Box
	log    *slog.Logger
	limits ScrapeLimits
}

var (
	_ domain.ScrapePool = (*ScrapePool)(nil)
	_ scrapeStore       = (*repository.Queries)(nil)
)

func NewScrapePool(
	q scrapeStore, box *secret.Box, log *slog.Logger, limits ScrapeLimits,
) *ScrapePool {
	return &ScrapePool{q: q, box: box, log: log, limits: limits.withDefaults()}
}

// Acquire cấp một phiên cho nền tảng này.
//
// Via là bắt buộc; proxy thì không. Không còn proxy nào khoẻ thì request đi
// thẳng bằng IP máy chủ: với một kênh lẻ, đi thẳng vẫn hơn là không quét được
// gì, và số lần `bot_block` sau đó sẽ tự nói ra rằng cần thêm proxy — đúng cách
// bảng fetch_error_stat vẫn dùng để quyết định.
func (p *ScrapePool) Acquire(
	ctx context.Context, platform domain.Platform,
) (*domain.ScrapeSession, error) {
	via, err := p.q.ClaimScrapeVia(ctx, string(platform))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", domain.ErrNoViaAvailable, platform)
	}
	if err != nil {
		return nil, fmt.Errorf("nhận via cho %s: %w", platform, err)
	}

	cookies, err := p.box.Decrypt(via.CookiesEncrypted)
	if err != nil {
		// Giải mã hỏng nghĩa là TOKEN_ENCRYPTION_KEY đã đổi sau khi via được
		// lưu. Via này không bao giờ dùng lại được, nên đánh dấu luôn thay vì
		// để bộ chọn cấp nó ra ở mọi lượt quét rồi hỏng ở đúng chỗ này.
		p.markViaBroken(ctx, via.ID, "không giải mã được cookies — TOKEN_ENCRYPTION_KEY đã đổi?")
		return nil, fmt.Errorf("giải mã cookies via %s: %w", via.Label, err)
	}

	s := &domain.ScrapeSession{
		ViaID:   via.ID.String(),
		ViaName: via.Label,
		Cookies: cookies,
	}
	p.attachProxy(ctx, s, platform)
	return s, nil
}

// attachProxy gắn proxy vào phiên nếu có cái nào khoẻ. Mọi nhánh hỏng đều chỉ
// ghi log rồi đi tiếp: thiếu proxy làm tăng rủi ro bị chặn chứ không làm lượt
// quét thành không hợp lệ.
func (p *ScrapePool) attachProxy(
	ctx context.Context, s *domain.ScrapeSession, platform domain.Platform,
) {
	proxy, err := p.q.ClaimScrapeProxy(ctx, ptr(string(platform)))
	if errors.Is(err, pgx.ErrNoRows) {
		p.log.WarnContext(ctx, "không còn proxy khoẻ — request sẽ đi thẳng bằng IP máy chủ",
			"platform", platform)
		return
	}
	if err != nil {
		p.log.WarnContext(ctx, "không nhận được proxy, đi thẳng", "error", err, "platform", platform)
		return
	}

	endpoint, err := p.box.Decrypt(proxy.EndpointEncrypted)
	if err != nil || endpoint == "" {
		p.log.WarnContext(ctx, "không giải mã được endpoint proxy, đi thẳng",
			"error", err, "proxy", proxy.Label)
		return
	}
	s.ProxyID, s.ProxyURL = proxy.ID.String(), endpoint
}

// Release đóng một lượt quét: cập nhật máy trạng thái và ghi nhật ký dùng via.
//
// KHÔNG trả lỗi. Nó chạy ở đường thoát của một lượt quét (thường là `defer`),
// và một lỗi ghi sổ ở đó không được phép thay thế lỗi thật của lượt quét.
func (p *ScrapePool) Release(
	ctx context.Context,
	s *domain.ScrapeSession,
	owner domain.ScrapeOwner,
	postsFound int,
	cause error,
) {
	if s == nil {
		return
	}
	// Context riêng: lượt quét hỏng vì context bị huỷ là đúng lúc trạng thái
	// via cần được ghi nhất — chạy bằng context đã chết thì mọi lỗi đều mất
	// dấu, và via hỏng vẫn nằm nguyên trong vòng xoay.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()

	viaID, err := uuid.Parse(s.ViaID)
	if err != nil {
		return
	}
	result := scrapeResultOf(cause)

	switch result {
	case domain.ScrapeResultSuccess:
		p.noteViaSuccess(ctx, viaID)
	case string(domain.FetchBlockLogin):
		p.noteViaLoginError(ctx, viaID, s.ViaName, cause)
	default:
		// Mọi lỗi còn lại KHÔNG phải lỗi của via: bị chặn IP, rate-limit, bài
		// bị xoá, mạng đứt. Đánh via chết vì chúng là thay nhầm thứ đang hỏng.
		p.noteViaOther(ctx, viaID, cause)
	}
	p.releaseProxy(ctx, s.ProxyID, result, cause)
	p.logUsage(ctx, s, owner, result, postsFound, cause)
}

func (p *ScrapePool) noteViaSuccess(ctx context.Context, viaID uuid.UUID) {
	if err := p.q.MarkScrapeViaUsed(ctx, viaID); err != nil {
		p.log.WarnContext(ctx, "không ghi được lượt dùng via", "error", err, "via_id", viaID)
	}
}

func (p *ScrapePool) noteViaLoginError(
	ctx context.Context, viaID uuid.UUID, label string, cause error,
) {
	after, err := p.q.MarkScrapeViaLoginError(ctx, repository.MarkScrapeViaLoginErrorParams{
		ID:        viaID,
		Threshold: int32(p.limits.ViaLoginErrors),
		Cooldown:  intervalOf(p.limits.ViaCooldown),
		LastError: nullable(domain.UserMessage(cause)),
	})
	if err != nil {
		p.log.WarnContext(ctx, "không cập nhật được trạng thái via", "error", err, "via_id", viaID)
		return
	}
	// Chỉ ồn ào khi via ĐỔI trạng thái. Mỗi lỗi lẻ một dòng WARN thì đúng lúc
	// via chết hàng loạt, log ngập và không ai thấy dòng quan trọng.
	switch after.Status {
	case domain.ViaDead:
		p.log.ErrorContext(ctx, "via đã chết — cần thay cookies mới",
			"via", label, "via_id", viaID, "lỗi_liên_tiếp", after.ConsecutiveLoginErrors)
	case domain.ViaCooldown:
		p.log.WarnContext(ctx, "via vào cooldown sau nhiều lần bị đòi đăng nhập",
			"via", label, "via_id", viaID, "thử_lại_lúc", after.CooldownUntil)
	}
}

func (p *ScrapePool) noteViaOther(ctx context.Context, viaID uuid.UUID, cause error) {
	if err := p.q.NoteScrapeViaError(ctx, repository.NoteScrapeViaErrorParams{
		ID: viaID, LastError: nullable(domain.UserMessage(cause)),
	}); err != nil {
		p.log.WarnContext(ctx, "không ghi được lỗi của via", "error", err, "via_id", viaID)
	}
}

// markViaBroken cất via không giải mã được ra khỏi vòng xoay.
func (p *ScrapePool) markViaBroken(ctx context.Context, viaID uuid.UUID, reason string) {
	status := domain.ViaDead
	if _, err := p.q.UpdateScrapeVia(ctx, repository.UpdateScrapeViaParams{
		ID: viaID, Status: &status,
	}); err != nil {
		p.log.WarnContext(ctx, "không cất được via hỏng", "error", err, "via_id", viaID)
	}
	p.log.ErrorContext(ctx, "via không dùng được", "via_id", viaID, "lý_do", reason)
}

// releaseProxy cập nhật proxy theo kết quả. Chỉ `bot_block` mới tính là lỗi của
// proxy — đó là định nghĩa của nhãn đó: nền tảng nghi IP, không nghi tài khoản.
func (p *ScrapePool) releaseProxy(ctx context.Context, rawID, result string, cause error) {
	if rawID == "" {
		return
	}
	proxyID, err := uuid.Parse(rawID)
	if err != nil {
		return
	}

	switch result {
	case string(domain.FetchBlockBot):
		after, err := p.q.MarkScrapeProxyBlocked(ctx, repository.MarkScrapeProxyBlockedParams{
			ID:        proxyID,
			Threshold: int32(p.limits.ProxyBlocks),
			LastError: nullable(domain.UserMessage(cause)),
		})
		if err != nil {
			p.log.WarnContext(ctx, "không cập nhật được trạng thái proxy", "error", err)
			return
		}
		if after.Status != domain.ProxyActive {
			p.log.WarnContext(ctx, "proxy bị hạ cấp vì liên tục bị chặn IP",
				"proxy", after.Label, "trạng_thái", after.Status,
				"lần_liên_tiếp", after.ConsecutiveBlocks)
		}
	case domain.ScrapeResultSuccess:
		if err := p.q.MarkScrapeProxyUsed(ctx, proxyID); err != nil {
			p.log.WarnContext(ctx, "không ghi được lượt dùng proxy", "error", err)
		}
	default:
		if err := p.q.NoteScrapeProxyError(ctx, repository.NoteScrapeProxyErrorParams{
			ID: proxyID, LastError: nullable(domain.UserMessage(cause)),
		}); err != nil {
			p.log.WarnContext(ctx, "không ghi được lỗi của proxy", "error", err)
		}
	}
}

func (p *ScrapePool) logUsage(
	ctx context.Context,
	s *domain.ScrapeSession,
	owner domain.ScrapeOwner,
	result string,
	postsFound int,
	cause error,
) {
	viaID, err := uuid.Parse(s.ViaID)
	if err != nil {
		return
	}
	var detail *string
	if cause != nil {
		detail = nullable(domain.UserMessage(cause))
	}
	if err := p.q.CreateViaUsageLog(ctx, repository.CreateViaUsageLogParams{
		ViaID:           viaID,
		ProxyID:         parseUUIDPtr(s.ProxyID),
		ListBreakingID:  parseUUIDPtr(derefOrEmpty(owner.Breaking)),
		ListScheduledID: parseUUIDPtr(derefOrEmpty(owner.Scheduled)),
		Platform:        string(owner.Platform),
		Result:          result,
		PostsFound:      int32(postsFound),
		Detail:          detail,
	}); err != nil {
		p.log.WarnContext(ctx, "không ghi được nhật ký dùng via", "error", err)
	}
}

// ---------------------------------------------------------------------------

// scrapeResultOf đổi lỗi của một lượt quét thành nhãn lưu trong via_usage_log.
//
// Dùng lại đúng bộ nhãn của domain.FetchBlockKind thay vì tự đặt tên: một lỗi
// chỉ nên có một tên trên toàn hệ thống, và nhờ vậy bảng này đối chiếu thẳng
// được với fetch_error_stat mà không cần bảng quy đổi nào.
func scrapeResultOf(cause error) string {
	if cause == nil {
		return domain.ScrapeResultSuccess
	}
	if kind, ok := domain.FetchBlockKindOf(cause); ok {
		return string(kind)
	}
	return string(domain.FetchBlockOther)
}

func intervalOf(d time.Duration) pgtype.Interval {
	return pgtype.Interval{Microseconds: d.Microseconds(), Valid: true}
}

func parseUUIDPtr(raw string) *uuid.UUID {
	if raw == "" {
		return nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil
	}
	return &id
}

func derefOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
