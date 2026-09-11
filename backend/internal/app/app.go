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

	Audit        *service.Audit
	Auth         *service.Auth
	User         *service.User
	SourcePost   *service.SourcePost
	Voice        *service.Voice
	List         *service.List
	Catalog      *service.Catalog
	AIEngine     *service.AIEngineService
	Engine       *service.Engine
	Scan         *service.Scan
	Maintenance  *service.Maintenance
	MultimeCreds *service.MultimeCreds
	MultimeUsers *service.MultimeUsers
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

	runner := platformadapter.NewExecRunner(10*time.Minute, map[string]string{
		"yt-dlp":  cfg.YtDlpPath,
		"ffmpeg":  cfg.FFmpegPath,
		"ffprobe": cfg.FFprobePath,
	})
	// Tất cả nền tảng đều tải qua yt-dlp; adapter chỉ khác cách nhận diện URL.
	platforms := platformadapter.NewRegistry(
		platformadapter.NewYouTube(runner, ""),
		platformadapter.NewFacebook(runner, ""),
		platformadapter.NewTikTok(runner, ""),
		platformadapter.NewInstagram(runner, ""),
		platformadapter.NewX(runner, ""),
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

	box, err := secret.NewBox(cfg.TokenEncryptionKey)
	if err != nil {
		return fail(err)
	}

	tokens := jwt.NewManager(cfg.JWTSecret, cfg.JWTAccessTTL, cfg.JWTRefreshTTL)
	enqueuer := worker.NewEnqueuer(asynqClient)
	audit := service.NewAudit(queries, log)
	multimeCreds := service.NewMultimeCreds(queries, multimeAuth, box, log)

	// Hình thức C viết lại nội dung bằng LLM. Provider còn là mock thì không có
	// gì viết lại được — tắt hẳn mode C thay vì để nó chạy và cho ra voice đọc
	// sai (bản mock trước đây đọc to cả prompt).
	modes := service.ModeGate{Enabled: cfg.EnabledCollectModes()}
	if llmProvider.Name() == "mock" {
		modes = disablePromptMode(modes, log)
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
			queries, platforms, enqueuer, audit, cfg.DefaultLanguage, scanDefaults, modes),
		Catalog:  service.NewCatalog(queries),
		AIEngine: service.NewAIEngineService(queries, box),
		Engine: service.NewEngine(service.EngineDeps{
			Queries:   queries,
			Platforms: platforms,
			// TTS trong .env chỉ là dự phòng cho dev; đường chính là API key
			// của từng user (bảng ai_engine) qua TTSFactory.
			TTS:        ttsProvider,
			TTSFactory: tts.NewFactory(cfg),
			Secret:     box,
			STT:        sttProvider,
			LLM:        llmProvider,
			Storage:    store,
			Prober:     prober,
			Multime:    multimeClient,
			Creds:      multimeCreds,
			Enqueuer:   enqueuer,
			Audit:      audit,
			Logger:     log,
		}),
		Scan:         service.NewScan(queries, platforms, enqueuer, log),
		Maintenance:  service.NewMaintenance(queries, log, cfg.SkippedLogRetention),
		MultimeCreds: multimeCreds,
		MultimeUsers: service.NewMultimeUsers(multimeDir, multimeCreds),
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

// disablePromptMode gỡ hình thức C khỏi danh sách đang bật.
//
// Mode C = "text nguồn -> LLM viết lại theo Prompt mẫu -> TTS đọc bản viết
// lại". Không có LLM thật thì bước giữa không tồn tại; để mode C chạy tiếp
// nghĩa là TTS đọc một thứ không ai viết lại — trước đây là đọc to cả prompt.
// Thà tắt và nói rõ thiếu gì.
func disablePromptMode(gate service.ModeGate, log *slog.Logger) service.ModeGate {
	const reason = "cần LLM thật để viết lại nội dung — đặt LLM_PROVIDER=anthropic " +
		"và ANTHROPIC_API_KEY trong .env rồi khởi động lại"

	enabled := make([]domain.CollectMode, 0, len(gate.Enabled))
	for _, m := range gate.Enabled {
		if m != domain.ModePromptToVoice {
			enabled = append(enabled, m)
		}
	}
	log.Warn("tắt hình thức C: "+reason, "llm", "mock")

	reasons := map[domain.CollectMode]string{domain.ModePromptToVoice: reason}
	for k, v := range gate.Reason {
		if _, taken := reasons[k]; !taken {
			reasons[k] = v
		}
	}
	return service.ModeGate{Enabled: enabled, Reason: reasons}
}
