// Package app wiring toàn bộ dependency dùng chung giữa các binary
// (api, worker, scheduler).
package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strongbody/voice-tool/backend/internal/config"
	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/infra/ai/llm"
	"github.com/strongbody/voice-tool/backend/internal/infra/ai/stt"
	"github.com/strongbody/voice-tool/backend/internal/infra/ai/tts"
	"github.com/strongbody/voice-tool/backend/internal/infra/audio"
	"github.com/strongbody/voice-tool/backend/internal/infra/multime"
	platformadapter "github.com/strongbody/voice-tool/backend/internal/infra/platform"
	"github.com/strongbody/voice-tool/backend/internal/infra/postgres"
	"github.com/strongbody/voice-tool/backend/internal/infra/storage"
	"github.com/strongbody/voice-tool/backend/internal/pkg/jwt"
	"github.com/strongbody/voice-tool/backend/internal/pkg/logger"
	"github.com/strongbody/voice-tool/backend/internal/pkg/secret"
	"github.com/strongbody/voice-tool/backend/internal/repository"
	"github.com/strongbody/voice-tool/backend/internal/service"
	"github.com/strongbody/voice-tool/backend/internal/worker"
)

// App giữ mọi thành phần đã khởi tạo. Đóng bằng Close().
type App struct {
	Config *config.Config
	Logger *slog.Logger
	// Modes: hình thức thu thập nào đang dùng được + vì sao cái còn lại không.
	// Router trả nguyên cái này cho FE qua /meta/collect-modes.
	Modes service.ModeGate

	Pool    *pgxpool.Pool
	Queries *repository.Queries

	Redis       asynq.RedisConnOpt
	AsynqClient *asynq.Client

	Platforms   *platformadapter.Registry
	Storage     domain.Storage
	Prober      domain.AudioProber
	TTS         domain.TTSProvider
	STT         domain.STTProvider
	LLM         domain.LLMProvider
	Multime     domain.MultimeClient
	MultimeAuth domain.MultimeAuthenticator
	// MultimeDir tra danh bạ tài khoản Strongbody (chọn tác giả bài đăng).
	MultimeDir domain.MultimeDirectory

	Tokens *jwt.Manager
	Secret *secret.Box

	Audit *service.Audit
	Auth  *service.Auth
	// FetchStats đếm số lần từng nền tảng chặn ta — dữ liệu để quyết định có
	// cần proxy hay không (bậc 6 của thang xử lý rủi ro).
	FetchStats *service.FetchStats
	// AIUsage đếm token/ký tự đã tiêu — dữ liệu để trả lời "đổi model có rẻ
	// hơn không", thứ mà màn Cài đặt cho đổi nhưng không cho đo.
	AIUsage *service.AIUsage
	// Health đếm những thứ đang hỏng âm thầm (voice lỗi, token chủ kênh chết).
	Health *service.Health
	// Settings đọc/ghi app_setting (chuỗi dự phòng LLM, batch).
	Settings *service.Settings
	// LLMSets quản lý Bộ API key LLM; LLMRouter là thứ worker gọi khi chạy mode C.
	LLMSets     *service.LLMAPISetService
	LLMRouter   *service.LLMRouter
	User        *service.User
	SourcePost  *service.SourcePost
	Voice       *service.Voice
	List        *service.List
	Catalog     *service.Catalog
	AIEngine    *service.AIEngineService
	Engine      *service.Engine
	Scan        *service.Scan
	Maintenance *service.Maintenance
	// ScrapePool cấp (via, proxy) cho từng lượt quét Facebook/X/Instagram;
	// ScrapeAdmin là mặt quản trị của chúng trên màn Cài đặt.
	ScrapePool   *service.ScrapePool
	ScrapeAdmin  *service.ScrapeAdmin
	MultimeCreds *service.MultimeCreds
	MultimeUsers *service.MultimeUsers
	// CatalogCache: danh mục quốc gia/hashtag lưu trong DB cho modal Tạo Voice.
	// Khác `Catalog` ở trên — cái đó là CRUD Prompt mẫu.
	CatalogCache *service.CatalogCache
}

// New khởi tạo tất cả dependency. Fail-fast nếu Postgres/Redis/S3 chưa sẵn sàng.
func New(ctx context.Context, cfg *config.Config) (*App, error) {
	log := logger.New(cfg.LogLevel, cfg.Env)
	logger.SetDefault(log)

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL, cfg.DBMaxConns)
	if err != nil {
		return nil, err
	}
	queries := repository.New(pool)

	redisOpt := asynq.RedisClientOpt{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	}
	asynqClient := asynq.NewClient(redisOpt)

	// Từ đây trở xuống, lỗi nào cũng phải đóng pool + client.
	fail := func(err error) (*App, error) {
		_ = asynqClient.Close()
		pool.Close()
		return nil, err
	}

	store, err := storage.NewS3(ctx, cfg)
	if err != nil {
		return fail(err)
	}

	// Hộp mã hoá dựng SỚM: cả cookies của via lẫn token multime đều đi qua nó,
	// và adapter Facebook bên dưới cần nó ngay lúc khởi tạo.
	box, err := secret.NewBox(cfg.TokenEncryptionKey)
	if err != nil {
		return fail(err)
	}

	runner := platformadapter.NewExecRunner(10*time.Minute, map[string]string{
		"yt-dlp":  cfg.YtDlpPath,
		"ffmpeg":  cfg.FFmpegPath,
		"ffprobe": cfg.FFprobePath,
	})
	// Tất cả nền tảng đều tải qua yt-dlp; adapter chỉ khác cách nhận diện URL.
	// Proxy khai theo từng nền tảng (YTDLP_PROXY_<NỀN TẢNG>), rỗng thì gọi
	// thẳng — xem config.YtDlpProxyFor.
	proxyFor := func(p domain.Platform) platformadapter.Option {
		return platformadapter.WithProxy(cfg.YtDlpProxyFor(p))
	}
	// Facebook: adapter yt-dlp lo phần lấy từng bài, còn phần LIỆT KÊ TRANG do
	// adapter tự đọc HTML đảm nhiệm (yt-dlp không có extractor nào cho việc đó).
	//
	// Truyền pool vào — tức mở khoá việc thêm kênh Facebook — chỉ khi cả hai
	// điều kiện cùng đúng: có hạ tầng via, và người vận hành đã bật cờ sau khi
	// đối chiếu với một trang thật. Xem config.FacebookChannelScan.
	scrapePool := service.NewScrapePool(queries, box, log, service.ScrapeLimits{
		ViaLoginErrors: cfg.ViaLoginErrorThreshold,
		ViaCooldown:    cfg.ViaCooldown,
		ProxyBlocks:    cfg.ProxyBlockThreshold,
	})
	// poolIf trả pool khi cờ của nền tảng đó đang bật, nil khi tắt — và nil
	// chính là thứ giữ nguyên cờ chặn thêm kênh. Một hàm dùng chung cho cả ba
	// để không có nền tảng nào lỡ được bật bằng một nhánh if viết thiếu.
	fetcher := platformadapter.NewScrapeFetcher()
	poolIf := func(enabled bool, flag string, p domain.Platform) domain.ScrapePool {
		if enabled {
			return scrapePool
		}
		log.Info(flag + "=false — chưa cho thêm kênh " + string(p) +
			"; bật sau khi đã thử via thật trên một trang thật")
		return nil
	}

	facebook := platformadapter.NewFacebookScrape(
		platformadapter.NewFacebook(runner, "", proxyFor(domain.PlatformFacebook)),
		poolIf(cfg.FacebookChannelScan, "FACEBOOK_CHANNEL_SCAN", domain.PlatformFacebook),
		fetcher,
	)
	instagram := platformadapter.NewInstagramScrape(
		platformadapter.NewInstagram(runner, "", proxyFor(domain.PlatformInstagram)),
		poolIf(cfg.InstagramChannelScan, "INSTAGRAM_CHANNEL_SCAN", domain.PlatformInstagram),
		fetcher,
	)
	x := platformadapter.NewXScrape(
		platformadapter.NewX(runner, "", proxyFor(domain.PlatformX)),
		poolIf(cfg.XChannelScan, "X_CHANNEL_SCAN", domain.PlatformX),
		fetcher,
	)

	platforms := platformadapter.NewRegistry(
		platformadapter.NewYouTube(runner, "", proxyFor(domain.PlatformYouTube)),
		facebook,
		platformadapter.NewTikTok(runner, "", proxyFor(domain.PlatformTikTok)),
		instagram,
		x,
	)
	prober := audio.NewProber(runner, "")

	// Provider TTS dự phòng: có thể là nil (không đặt key chung) — lúc đó user
	// nào chưa khai API key riêng thì báo lỗi kèm hướng dẫn thay vì chạy nhờ.
	ttsProvider, err := tts.New(cfg)
	if err != nil {
		return fail(err)
	}
	if ttsProvider == nil {
		log.Info("chưa có API key TTS chung — mỗi user phải tự khai key ở mục AI Engine")
	} else if ttsProvider.Name() == "mock" {
		log.Warn("TTS_PROVIDER=mock — voice mode B/C sẽ là audio im lặng, chỉ dùng khi dev")
	}
	sttProvider, err := stt.New(cfg)
	if err != nil {
		return fail(err)
	}
	llmProvider, err := llm.New(cfg)
	if err != nil {
		return fail(err)
	}
	// 1 client dùng cho cả đăng nhập và đăng voice. Chưa cấu hình voice API thì
	// dùng mock để dev chạy được đầu-cuối.
	var (
		multimeClient domain.MultimeClient
		multimeAuth   domain.MultimeAuthenticator
		multimeDir    domain.MultimeDirectory
	)
	if cfg.MultimeBaseURL == "" {
		mock := multime.NewMock()
		multimeClient, multimeAuth, multimeDir = mock, mock, mock
		log.Warn("MULTIME_BASE_URL chưa cấu hình — dùng mock cho đăng nhập và publish")
	} else {
		client := multime.New(cfg)
		multimeClient, multimeAuth, multimeDir = client, client, client
	}

	tokens := jwt.NewManager(cfg.JWTSecret, cfg.JWTAccessTTL, cfg.JWTRefreshTTL)
	settings := service.NewSettings(queries, log)
	aiUsage := service.NewAIUsage(queries, settings, log)
	// Router đứng TRÊN các adapter LLM: chọn key + model theo Bộ API gắn trên
	// voice, tự chuyển dự phòng khi hết hạn mức. `llmProvider` từ .env chỉ còn
	// là đường dự phòng cho voice không gắn bộ nào (thực tế là dev).
	llmRouter := service.NewLLMRouter(service.LLMRouterDeps{
		Queries:  queries,
		Secret:   box,
		Factory:  llm.NewFactory(),
		Settings: settings,
		Fallback: llmProvider,
		Usage:    aiUsage,
		Logger:   log,
	})
	enqueuer := worker.NewEnqueuer(asynqClient)
	audit := service.NewAudit(queries, log)
	// Giữ nhịp gọi yt-dlp theo từng nền tảng + đếm số lần bị chặn. Cùng 1 gate
	// cho cả quét kênh lẫn tải bài: nền tảng chỉ thấy tổng số request, không
	// quan tâm request đó đến từ luồng nào của ta.
	platformGate := service.NewPlatformGate(cfg.PlatformMinGap)
	// Gate riêng cho TTS: khoá là API key chứ không phải nền tảng, và 3voices
	// cho 2 job đồng thời trên mỗi key chứ không phải 1.
	ttsGate := service.NewTTSGate(cfg.TTSMinGap, cfg.TTSMaxConcurrent)
	// Nhịp riêng cho Facebook/X/Instagram: chậm hơn và ít song song hơn yt-dlp.
	// Một lần tải trang ở đây mang theo cookies của tài khoản thật, nên nó bị
	// soi kỹ hơn hẳn một lần yt-dlp lấy video ẩn danh.
	scrapeGate := service.NewKeyGate(cfg.ScrapeMinGap, cfg.ScrapeConcurrency)
	// Instagram đi làn CHẬM RIÊNG: 1 request tại một thời điểm, nghỉ 30 giây.
	//
	// Số đo ngày 17/09/2026: với nhịp chung (3 request song song, nghỉ 5 giây)
	// endpoint web_profile_info trả 429 "Please wait a few minutes before you
	// try again" gần như mọi lượt — 7 lần liên tiếp trong nhật ký. Facebook và
	// X cùng nhịp đó thì không sao, nên hạ nhịp chung là phạt nhầm hai nền tảng
	// đang chạy được.
	//
	// 30 giây không phải con số thiêng: nó là mức đủ thưa để một vòng quét vài
	// kênh Instagram không còn trông như một đợt dồn dập, mà vẫn quét hết trong
	// ngày. Hạn mức thật của Instagram không công bố, nên đây là mức khởi đầu
	// phải đo lại — nếu vẫn 429 thì thưa thêm, hoặc nuôi thêm via để chia tải.
	scrapeGate.SetKeyPace(string(domain.PlatformInstagram), 30*time.Second, 1)
	fetchStats := service.NewFetchStats(queries, log)
	multimeCreds := service.NewMultimeCreds(queries, multimeAuth, box, log)
	multimeUsers := service.NewMultimeUsers(multimeDir, multimeCreds)

	// Hình thức C viết lại nội dung bằng LLM. Không có LLM thật thì không có gì
	// viết lại được — mode C bị tắt thay vì chạy và cho ra voice đọc sai (bản
	// mock trước đây đọc to cả prompt cho AI).
	//
	// "Có LLM thật" có HAI đường: provider trong .env (dự phòng, chủ yếu cho
	// dev) hoặc Bộ API key trong DB — đường chính. Đường thứ hai thay đổi TRONG
	// LÚC CHẠY: người dùng thêm bộ key qua giao diện bất cứ lúc nào. Nên câu
	// hỏi này phải được hỏi lại mỗi lần, không chốt một lần lúc khởi động —
	// chốt một lần nghĩa là thêm key xong vẫn phải restart mới dùng được mode C.
	modes := service.ModeGate{Enabled: cfg.EnabledCollectModes()}
	if llmProvider.Name() == "mock" {
		modes.LLM = service.NewLLMKeyProbe(queries, log)
	}

	scanDefaults := service.ScanDefaults{
		Interval:       cfg.BreakingScanInterval,
		Limit:          cfg.ScanLimitDefault,
		MaxPostsPerRun: cfg.MaxPostsPerRunDefault,
	}

	app := &App{
		Config: cfg, Logger: log, Modes: modes,
		Pool: pool, Queries: queries,
		Redis: redisOpt, AsynqClient: asynqClient,
		Platforms: platforms, Storage: store, Prober: prober,
		TTS: ttsProvider, STT: sttProvider, LLM: llmProvider,
		Multime: multimeClient, MultimeAuth: multimeAuth, MultimeDir: multimeDir,
		Tokens: tokens, Secret: box,
		Audit: audit,
		Auth: service.NewAuth(service.AuthDeps{
			Queries:             queries,
			Tokens:              tokens,
			MultimeAuth:         multimeAuth,
			Box:                 box,
			Logger:              log,
			DefaultRole:         domain.Role(cfg.DefaultUserRole),
			BootstrapAdminEmail: cfg.BootstrapAdminEmail,
		}),
		User: service.NewUser(queries, log),
		SourcePost: service.NewSourcePost(
			queries, platforms, enqueuer, audit, log, cfg.DefaultLanguage, modes),
		Voice: service.NewVoice(queries, store, enqueuer, audit, cfg.DefaultLanguage, modes),
		List: service.NewList(
			queries, platforms, enqueuer, audit, cfg.DefaultLanguage, scanDefaults, modes, log),
		Catalog:   service.NewCatalog(queries),
		AIEngine:  service.NewAIEngineService(queries, box),
		Settings:  settings,
		LLMSets:   service.NewLLMAPISetService(queries, box),
		LLMRouter: llmRouter,
		Engine: service.NewEngine(service.EngineDeps{
			Queries:   queries,
			Platforms: platforms,
			// TTS trong .env chỉ là dự phòng cho dev; đường chính là API key
			// của từng user (bảng ai_engine) qua TTSFactory.
			TTS:        ttsProvider,
			TTSFactory: tts.NewFactory(cfg),
			Secret:     box,
			STT:        sttProvider,
			LLM:        llmRouter,
			Storage:    store,
			Prober:     prober,
			Multime:    multimeClient,
			Creds:      multimeCreds,
			Enqueuer:   enqueuer,
			Audit:      audit,
			Logger:     log,
			Gate:       platformGate,
			FetchStats: fetchStats,
			TTSGate:    ttsGate,
			Usage:      aiUsage,
			Authors:    multimeUsers,
		}),
		Scan: service.NewScan(service.ScanDeps{
			Queries:    queries,
			Platforms:  platforms,
			Enqueuer:   enqueuer,
			Logger:     log,
			Gate:       platformGate,
			ScrapeGate: scrapeGate,
			FetchStats: fetchStats,
		}),
		FetchStats:  fetchStats,
		AIUsage:     aiUsage,
		Health:      service.NewHealth(queries, log),
		ScrapePool:  scrapePool,
		ScrapeAdmin: service.NewScrapeAdmin(queries, box, audit),
		Maintenance: service.NewMaintenance(
			queries, log, cfg.SkippedLogRetention, cfg.AIUsageRetention,
			cfg.ScanRunRetention, cfg.ViaUsageLogRetention),
		MultimeCreds: multimeCreds,
		MultimeUsers: multimeUsers,
		CatalogCache: service.NewCatalogCache(queries, multimeUsers, log),
	}

	log.Info("khởi tạo xong",
		"env", cfg.Env,
		"tts", providerName(ttsProvider),
		"stt", providerName(sttProvider),
		"llm", providerName(llmProvider),
		"platforms", platforms.Supported(),
		"collect_modes", modes.Enabled,
		"scan_interval", cfg.BreakingScanInterval.String(),
		"scan_limit", cfg.ScanLimitDefault,
	)
	return app, nil
}

func (a *App) Close() error {
	var errs []error
	if a.AsynqClient != nil {
		if err := a.AsynqClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("đóng asynq client: %w", err))
		}
	}
	if a.Pool != nil {
		a.Pool.Close()
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// providerName đọc tên provider, chấp nhận cả trường hợp KHÔNG có provider.
//
// `tts.New` cố tình trả về nil khi không khai key TTS chung: đó là cấu hình
// production bình thường — mỗi user tự khai key của mình ở màn AI Engine, và
// worker báo lỗi kèm hướng dẫn cho ai chưa khai. Gọi thẳng .Name() trên nil là
// panic ngay lúc khởi động, tức là cấu hình đúng lại làm app không chạy được.
func providerName(p interface{ Name() string }) string {
	if p == nil {
		return "(chưa cấu hình)"
	}
	return p.Name()
}
