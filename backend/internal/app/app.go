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

	Tokens *jwt.Manager
	Secret *secret.Box

	Audit        *service.Audit
	Auth         *service.Auth
	User         *service.User
	SourcePost   *service.SourcePost
	Voice        *service.Voice
	List         *service.List
	Catalog      *service.Catalog
	Engine       *service.Engine
	Scan         *service.Scan
	Maintenance  *service.Maintenance
	MultimeCreds *service.MultimeCreds
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

	ttsProvider, err := tts.New(cfg)
	if err != nil {
		return fail(err)
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
	)
	if cfg.MultimeBaseURL == "" {
		mock := multime.NewMock()
		multimeClient, multimeAuth = mock, mock
		log.Warn("MULTIME_BASE_URL chưa cấu hình — dùng mock cho đăng nhập và publish")
	} else {
		client := multime.New(cfg)
		multimeClient, multimeAuth = client, client
	}

	box, err := secret.NewBox(cfg.TokenEncryptionKey)
	if err != nil {
		return fail(err)
	}

	tokens := jwt.NewManager(cfg.JWTSecret, cfg.JWTAccessTTL, cfg.JWTRefreshTTL)
	enqueuer := worker.NewEnqueuer(asynqClient)
	audit := service.NewAudit(queries, log)
	multimeCreds := service.NewMultimeCreds(queries, multimeAuth, box, log)

	enabledModes := cfg.EnabledCollectModes()

	scanDefaults := service.ScanDefaults{
		Interval:       cfg.BreakingScanInterval,
		Limit:          cfg.ScanLimitDefault,
		MaxPostsPerRun: cfg.MaxPostsPerRunDefault,
	}

	app := &App{
		Config: cfg, Logger: log,
		Pool: pool, Queries: queries,
		Redis: redisOpt, AsynqClient: asynqClient,
		Platforms: platforms, Storage: store, Prober: prober,
		TTS: ttsProvider, STT: sttProvider, LLM: llmProvider,
		Multime: multimeClient, MultimeAuth: multimeAuth,
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
			queries, platforms, enqueuer, audit, log, cfg.DefaultLanguage, enabledModes),
		Voice: service.NewVoice(queries, store, enqueuer, audit),
		List: service.NewList(
			queries, platforms, enqueuer, audit, cfg.DefaultLanguage, scanDefaults, enabledModes),
		Catalog: service.NewCatalog(queries),
		Engine: service.NewEngine(service.EngineDeps{
			Queries:   queries,
			Platforms: platforms,
			TTS:       ttsProvider,
			STT:       sttProvider,
			LLM:       llmProvider,
			Storage:   store,
			Prober:    prober,
			Multime:   multimeClient,
			Creds:     multimeCreds,
			Enqueuer:  enqueuer,
			Audit:     audit,
			Logger:    log,
		}),
		Scan:         service.NewScan(queries, platforms, enqueuer, log),
		Maintenance:  service.NewMaintenance(queries, log, cfg.SkippedLogRetention),
		MultimeCreds: multimeCreds,
	}

	log.Info("khởi tạo xong",
		"env", cfg.Env,
		"tts", ttsProvider.Name(),
		"stt", sttProvider.Name(),
		"llm", llmProvider.Name(),
		"platforms", platforms.Supported(),
		"collect_modes", enabledModes,
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
