package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/pkg/secret"
	"github.com/strongbody/voice-tool/backend/internal/repository"
)

// ---------------------------------------------------------------------------
// Bản giả của kho dữ liệu
// ---------------------------------------------------------------------------

// fakeScrapeStore ghi lại ĐÃ GỌI CÂU NÀO, không mô phỏng lại SQL.
//
// Phần quyết định trạng thái (active -> cooldown -> dead) nằm trong chính câu
// UPDATE và được kiểm bằng test chạy trên Postgres thật, vì mô phỏng lại nó
// bằng Go chỉ tạo ra một bản sao có thể lệch với bản thật. Cái test ở đây bảo
// vệ thứ khác, và là thứ dễ sai hơn nhiều: LỖI LOẠI NÀY THÌ GỌI CÂU NÀO — đánh
// via chết vì một IP bị chặn là vứt đi một tài khoản còn tốt.
type fakeScrapeStore struct {
	via   repository.ScrapeVia
	proxy repository.ScrapeProxy

	viaErr   error
	proxyErr error

	// Sau mỗi lần MarkScrapeViaLoginError, trả về via với trạng thái này.
	loginErrResult repository.ScrapeVia

	calls    []string
	lastLog  repository.CreateViaUsageLogParams
	lastMark repository.MarkScrapeViaLoginErrorParams
}

func (f *fakeScrapeStore) note(name string) { f.calls = append(f.calls, name) }

func (f *fakeScrapeStore) did(name string) bool {
	for _, c := range f.calls {
		if c == name {
			return true
		}
	}
	return false
}

func (f *fakeScrapeStore) ClaimScrapeVia(context.Context, string) (repository.ScrapeVia, error) {
	f.note("ClaimScrapeVia")
	return f.via, f.viaErr
}

func (f *fakeScrapeStore) UpdateScrapeVia(
	context.Context, repository.UpdateScrapeViaParams,
) (repository.ScrapeVia, error) {
	f.note("UpdateScrapeVia")
	return f.via, nil
}

func (f *fakeScrapeStore) MarkScrapeViaUsed(context.Context, uuid.UUID) error {
	f.note("MarkScrapeViaUsed")
	return nil
}

func (f *fakeScrapeStore) MarkScrapeViaLoginError(
	_ context.Context, arg repository.MarkScrapeViaLoginErrorParams,
) (repository.ScrapeVia, error) {
	f.note("MarkScrapeViaLoginError")
	f.lastMark = arg
	return f.loginErrResult, nil
}

func (f *fakeScrapeStore) NoteScrapeViaError(
	context.Context, repository.NoteScrapeViaErrorParams,
) error {
	f.note("NoteScrapeViaError")
	return nil
}

func (f *fakeScrapeStore) ClaimScrapeProxy(
	context.Context, *string,
) (repository.ScrapeProxy, error) {
	f.note("ClaimScrapeProxy")
	return f.proxy, f.proxyErr
}

func (f *fakeScrapeStore) MarkScrapeProxyUsed(context.Context, uuid.UUID) error {
	f.note("MarkScrapeProxyUsed")
	return nil
}

func (f *fakeScrapeStore) MarkScrapeProxyBlocked(
	context.Context, repository.MarkScrapeProxyBlockedParams,
) (repository.ScrapeProxy, error) {
	f.note("MarkScrapeProxyBlocked")
	return f.proxy, nil
}

func (f *fakeScrapeStore) NoteScrapeProxyError(
	context.Context, repository.NoteScrapeProxyErrorParams,
) error {
	f.note("NoteScrapeProxyError")
	return nil
}

func (f *fakeScrapeStore) CreateViaUsageLog(
	_ context.Context, arg repository.CreateViaUsageLogParams,
) error {
	f.note("CreateViaUsageLog")
	f.lastLog = arg
	return nil
}

// ---------------------------------------------------------------------------

const testEncryptionKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="

func newTestPool(t *testing.T, store *fakeScrapeStore) *ScrapePool {
	t.Helper()
	box, err := secret.NewBox(testEncryptionKey)
	if err != nil {
		t.Fatalf("dựng hộp mã hoá: %v", err)
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewScrapePool(store, box, quiet, ScrapeLimits{})
}

func sealed(t *testing.T, plaintext string) string {
	t.Helper()
	box, err := secret.NewBox(testEncryptionKey)
	if err != nil {
		t.Fatalf("dựng hộp mã hoá: %v", err)
	}
	out, err := box.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("mã hoá: %v", err)
	}
	return out
}

func storeWithVia(t *testing.T) *fakeScrapeStore {
	t.Helper()
	return &fakeScrapeStore{
		via: repository.ScrapeVia{
			ID:               uuid.New(),
			Label:            "fb-01",
			Platform:         string(domain.PlatformFacebook),
			Status:           domain.ViaActive,
			CookiesEncrypted: sealed(t, "c_user=100; xs=abc"),
		},
		proxy: repository.ScrapeProxy{
			ID:                uuid.New(),
			Label:             "gw-01",
			Status:            domain.ProxyActive,
			EndpointEncrypted: sealed(t, "http://u:p@gw.example.net:8000"),
		},
	}
}

func TestScrapeLimitsDefaults(t *testing.T) {
	got := ScrapeLimits{}.withDefaults()
	if got.ViaLoginErrors != 3 {
		t.Errorf("ViaLoginErrors = %d, muốn 3", got.ViaLoginErrors)
	}
	if got.ViaCooldown != 6*time.Hour {
		t.Errorf("ViaCooldown = %s, muốn 6h", got.ViaCooldown)
	}
	if got.ProxyBlocks != 3 {
		t.Errorf("ProxyBlocks = %d, muốn 3", got.ProxyBlocks)
	}
}

func TestAcquire(t *testing.T) {
	ctx := context.Background()

	t.Run("cấp được via kèm proxy, cookies đã giải mã", func(t *testing.T) {
		store := storeWithVia(t)
		s, err := newTestPool(t, store).Acquire(ctx, domain.PlatformFacebook)
		if err != nil {
			t.Fatalf("lỗi bất ngờ: %v", err)
		}
		if s.Cookies != "c_user=100; xs=abc" {
			t.Errorf("cookies = %q", s.Cookies)
		}
		if s.ProxyURL != "http://u:p@gw.example.net:8000" {
			t.Errorf("proxy = %q", s.ProxyURL)
		}
	})

	t.Run("hết via thì trả ErrNoViaAvailable", func(t *testing.T) {
		// Đây KHÔNG phải sự cố: hết hạn mức ngày là trạng thái vận hành bình
		// thường, và vòng quét phải phân biệt được nó với lỗi thật để bỏ qua
		// kênh đó thay vì làm hỏng cả mẻ.
		store := storeWithVia(t)
		store.viaErr = pgx.ErrNoRows
		_, err := newTestPool(t, store).Acquire(ctx, domain.PlatformFacebook)
		if !errors.Is(err, domain.ErrNoViaAvailable) {
			t.Fatalf("muốn ErrNoViaAvailable, được %v", err)
		}
	})

	t.Run("hết proxy vẫn chạy, chỉ là đi thẳng", func(t *testing.T) {
		store := storeWithVia(t)
		store.proxyErr = pgx.ErrNoRows
		s, err := newTestPool(t, store).Acquire(ctx, domain.PlatformFacebook)
		if err != nil {
			t.Fatalf("thiếu proxy không được làm hỏng lượt quét: %v", err)
		}
		if s.ProxyID != "" || s.ProxyURL != "" {
			t.Errorf("muốn phiên không có proxy, được %+v", s)
		}
	})

	t.Run("cookies không giải mã được thì via bị cất đi", func(t *testing.T) {
		// Khoá mã hoá đã đổi sau khi via được lưu: via này không bao giờ dùng
		// lại được, để nó trong vòng xoay là mỗi lượt quét lại hỏng một lần.
		store := storeWithVia(t)
		store.via.CookiesEncrypted = "khong-phai-ciphertext"
		if _, err := newTestPool(t, store).Acquire(ctx, domain.PlatformFacebook); err == nil {
			t.Fatal("muốn lỗi khi không giải mã được cookies")
		}
		if !store.did("UpdateScrapeVia") {
			t.Error("via hỏng phải được cất ra khỏi vòng xoay")
		}
	})
}

// TestReleaseRoutesByErrorKind là test quan trọng nhất của tệp này: mỗi loại
// lỗi phải đi vào đúng câu cập nhật.
//
// Sai ở đây không gây lỗi biên dịch, không gây lỗi chạy, và không ai thấy — chỉ
// có via chết dần vì lỗi của proxy, hoặc via hỏng sống mãi trong vòng xoay.
func TestReleaseRoutesByErrorKind(t *testing.T) {
	ctx := context.Background()
	owner := domain.ScrapeOwner{Platform: domain.PlatformFacebook}

	tests := []struct {
		name string
		//nolint:errname // đây là lỗi dựng cho test, không phải kiểu lỗi mới
		cause error
		// Câu PHẢI được gọi, và câu KHÔNG ĐƯỢC gọi.
		want       string
		mustNotUse string
		wantResult string
	}{
		{
			name:  "thành công thì trừ hạn mức via",
			cause: nil, want: "MarkScrapeViaUsed",
			mustNotUse: "MarkScrapeViaLoginError", wantResult: "success",
		},
		{
			// Đòi đăng nhập là lỗi CỦA VIA — phiên hỏng.
			name:  "đòi đăng nhập thì chạy máy trạng thái của via",
			cause: domain.FetchBlocked(domain.FetchBlockLogin, errors.New("login required")),
			want:  "MarkScrapeViaLoginError", mustNotUse: "MarkScrapeViaUsed",
			wantResult: "login_required",
		},
		{
			// Chặn IP là lỗi CỦA PROXY. Đánh via chết vì nó là thay nhầm thứ
			// đang hỏng, và ta vừa vứt đi một tài khoản còn dùng được.
			name:  "chặn IP thì hạ cấp proxy, không đụng via",
			cause: domain.FetchBlocked(domain.FetchBlockBot, errors.New("not a bot")),
			want:  "MarkScrapeProxyBlocked", mustNotUse: "MarkScrapeViaLoginError",
			wantResult: "bot_block",
		},
		{
			name:  "rate-limit không giết via lẫn proxy",
			cause: domain.FetchBlocked(domain.FetchBlockRateLimit, errors.New("429")),
			want:  "NoteScrapeViaError", mustNotUse: "MarkScrapeViaLoginError",
			wantResult: "rate_limit",
		},
		{
			// Lỗi không phân loại được (mạng đứt, parser hỏng) không được phép
			// đánh chết thứ gì: ta không biết ai sai.
			name:  "lỗi lạ chỉ được ghi lại",
			cause: errors.New("dial tcp: i/o timeout"),
			want:  "NoteScrapeViaError", mustNotUse: "MarkScrapeProxyBlocked",
			wantResult: "other",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := storeWithVia(t)
			pool := newTestPool(t, store)
			s, err := pool.Acquire(ctx, domain.PlatformFacebook)
			if err != nil {
				t.Fatalf("Acquire: %v", err)
			}
			store.calls = nil

			pool.Release(ctx, s, owner, 5, tc.cause)

			if !store.did(tc.want) {
				t.Errorf("muốn gọi %s, thực tế gọi %v", tc.want, store.calls)
			}
			if store.did(tc.mustNotUse) {
				t.Errorf("KHÔNG được gọi %s, thực tế gọi %v", tc.mustNotUse, store.calls)
			}
			if !store.did("CreateViaUsageLog") {
				t.Error("mọi lượt quét đều phải để lại một dòng nhật ký")
			}
			if store.lastLog.Result != tc.wantResult {
				t.Errorf("result = %q, muốn %q", store.lastLog.Result, tc.wantResult)
			}
		})
	}
}

func TestReleasePassesLimitsToStateMachine(t *testing.T) {
	ctx := context.Background()
	store := storeWithVia(t)
	pool := NewScrapePool(store, mustBox(t), slog.New(slog.NewTextHandler(io.Discard, nil)),
		ScrapeLimits{ViaLoginErrors: 5, ViaCooldown: 2 * time.Hour, ProxyBlocks: 2})

	s, err := pool.Acquire(ctx, domain.PlatformFacebook)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	pool.Release(ctx, s, domain.ScrapeOwner{Platform: domain.PlatformFacebook}, 0,
		domain.FetchBlocked(domain.FetchBlockLogin, errors.New("login required")))

	// Ngưỡng và thời gian nghỉ phải xuống tới câu SQL: chúng cấu hình được, và
	// một giá trị bị bỏ quên ở đây thì .env nói một đằng, hệ thống chạy một nẻo.
	if store.lastMark.Threshold != 5 {
		t.Errorf("threshold = %d, muốn 5", store.lastMark.Threshold)
	}
	if got := time.Duration(store.lastMark.Cooldown.Microseconds) * time.Microsecond; got != 2*time.Hour {
		t.Errorf("cooldown = %s, muốn 2h", got)
	}
}

// TestReleaseLogsOwner: nhật ký phải quy được lượt quét về đúng kênh, nếu không
// nó chỉ trả lời "có lỗi" mà không trả lời "ở đâu".
func TestReleaseLogsOwner(t *testing.T) {
	ctx := context.Background()
	store := storeWithVia(t)
	pool := newTestPool(t, store)
	s, err := pool.Acquire(ctx, domain.PlatformFacebook)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	listID := uuid.New().String()
	pool.Release(ctx, s, domain.ScrapeOwner{
		Platform: domain.PlatformFacebook, Scheduled: &listID,
	}, 7, nil)

	if store.lastLog.ListScheduledID == nil || store.lastLog.ListScheduledID.String() != listID {
		t.Errorf("list_scheduled_id = %v, muốn %s", store.lastLog.ListScheduledID, listID)
	}
	if store.lastLog.ListBreakingID != nil {
		t.Error("một lượt quét chỉ thuộc về đúng một loại kênh")
	}
	if store.lastLog.PostsFound != 7 {
		t.Errorf("posts_found = %d, muốn 7", store.lastLog.PostsFound)
	}
}

func TestMaskProxyEndpoint(t *testing.T) {
	tests := []struct{ in, want string }{
		{"http://user:pass@gw.example.net:8000", "http://gw.example.net:8000"},
		{"http://gw.example.net:8000", "http://gw.example.net:8000"},
		{"socks5://u:p@1.2.3.4:1080", "socks5://1.2.3.4:1080"},
		{"", ""},
		// Không parse được thì trả RỖNG, không trả nguyên bản: một endpoint dị
		// dạng vẫn có thể chứa mật khẩu, và hiện nguyên nó lên giao diện là làm
		// rò đúng thứ cột này sinh ra để che.
		{"user:pass@host", ""},
		{"::không-phải-url", ""},
	}
	for _, tc := range tests {
		if got := domain.MaskProxyEndpoint(tc.in); got != tc.want {
			t.Errorf("MaskProxyEndpoint(%q) = %q, muốn %q", tc.in, got, tc.want)
		}
	}
}

func mustBox(t *testing.T) *secret.Box {
	t.Helper()
	box, err := secret.NewBox(testEncryptionKey)
	if err != nil {
		t.Fatalf("dựng hộp mã hoá: %v", err)
	}
	return box
}
