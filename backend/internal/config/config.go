package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// Config — toàn bộ cấu hình hệ thống, đọc từ .env (viper).
type Config struct {
	Env      string `mapstructure:"APP_ENV"`
	HTTPPort int    `mapstructure:"HTTP_PORT"`
	LogLevel string `mapstructure:"LOG_LEVEL"`

	DatabaseURL     string `mapstructure:"DATABASE_URL"`
	DBMaxConns      int32  `mapstructure:"DB_MAX_CONNS"`
	RedisAddr       string `mapstructure:"REDIS_ADDR"`
	RedisPassword   string `mapstructure:"REDIS_PASSWORD"`
	RedisDB         int    `mapstructure:"REDIS_DB"`
	WorkerConcurrcy int    `mapstructure:"WORKER_CONCURRENCY"`

	JWTSecret     string        `mapstructure:"JWT_SECRET"`
	JWTAccessTTL  time.Duration `mapstructure:"JWT_ACCESS_TTL"`
	JWTRefreshTTL time.Duration `mapstructure:"JWT_REFRESH_TTL"`

	S3Endpoint      string `mapstructure:"S3_ENDPOINT"`
	S3Region        string `mapstructure:"S3_REGION"`
	S3Bucket        string `mapstructure:"S3_BUCKET"`
	S3AccessKey     string `mapstructure:"S3_ACCESS_KEY"`
	S3SecretKey     string `mapstructure:"S3_SECRET_KEY"`
	S3UsePathStyle  bool   `mapstructure:"S3_USE_PATH_STYLE"`
	S3PublicBaseURL string `mapstructure:"S3_PUBLIC_BASE_URL"`

	// AI providers — chọn provider mặc định + credential từng nhà cung cấp.
	TTSProvider        string `mapstructure:"TTS_PROVIDER"`
	STTProvider        string `mapstructure:"STT_PROVIDER"`
	LLMProvider        string `mapstructure:"LLM_PROVIDER"`
	ThreeVoicesAPIKey  string `mapstructure:"THREEVOICES_API_KEY"`
	ThreeVoicesBaseURL string `mapstructure:"THREEVOICES_BASE_URL"`
	ThreeVoicesVoiceID string `mapstructure:"THREEVOICES_VOICE_ID"`
	OpenAIAPIKey       string `mapstructure:"OPENAI_API_KEY"`
	AnthropicAPIKey    string `mapstructure:"ANTHROPIC_API_KEY"`
	AnthropicModel     string `mapstructure:"ANTHROPIC_MODEL"`

	// Binary ngoài mà worker gọi. YouTube dùng yt-dlp nên không cần API key;
	// nền tảng mới sẽ tự khai credential của nó khi có adapter.
	YtDlpPath   string `mapstructure:"YTDLP_PATH"`
	FFmpegPath  string `mapstructure:"FFMPEG_PATH"`
	FFprobePath string `mapstructure:"FFPROBE_PATH"`

	// multime.ai — đích publish.
	//   MULTIME_BASE_URL      voice API   (vd https://voice-api.strongbody.ai)
	//   MULTIME_AUTH_BASE_URL auth API    (vd https://api-v2.strongbody.ai)
	//   MULTIME_SITE_URL      web công khai, dùng để dựng URL bài đăng
	MultimeBaseURL     string `mapstructure:"MULTIME_BASE_URL"`
	MultimeAuthBaseURL string `mapstructure:"MULTIME_AUTH_BASE_URL"`
	MultimeSiteURL     string `mapstructure:"MULTIME_SITE_URL"`
	// Mặc định cho bài đăng.
	MultimeVisibility        string `mapstructure:"MULTIME_VISIBILITY"`
	MultimeCategoryIDsRaw    string `mapstructure:"MULTIME_CATEGORY_IDS"`
	MultimeDefaultHashtagRaw string `mapstructure:"MULTIME_DEFAULT_HASHTAGS"`
	MultimePublicDownload    bool   `mapstructure:"MULTIME_PUBLIC_DOWNLOAD"`

	// ---- Tham số quét: mặc định là chỉ số tối ưu, chỉnh được qua .env ----
	// Chu kỳ vòng dispatch của Breaking. Kênh nào có scan_interval riêng thì
	// theo kênh; không có thì theo giá trị này.
	BreakingScanInterval time.Duration `mapstructure:"BREAKING_SCAN_INTERVAL"`
	// Số bài lấy về mỗi vòng quét khi kênh không cấu hình scan_limit riêng.
	ScanLimitDefault int `mapstructure:"SCAN_LIMIT_DEFAULT"`
	// Trần Bài Post tạo ra trong 1 vòng quét — chặn nổ chi phí AI khi kênh
	// đăng ồ ạt. 0 = không giới hạn.
	MaxPostsPerRunDefault int `mapstructure:"MAX_POSTS_PER_RUN_DEFAULT"`
	// Số kênh Breaking quét song song trong 1 vòng dispatch.
	BreakingScanParallelism int `mapstructure:"BREAKING_SCAN_PARALLELISM"`
	// Chu kỳ scheduler đọc lại cấu hình lịch của Danh sách Định kỳ từ DB.
	SchedulerSyncInterval time.Duration `mapstructure:"SCHEDULER_SYNC_INTERVAL"`
	// Giữ skipped_log bao lâu trước khi job dọn dẹp xoá.
	SkippedLogRetention time.Duration `mapstructure:"SKIPPED_LOG_RETENTION"`

	// Ngôn ngữ mặc định hệ thống — đáy của cascade (business rule #9).
	// "auto" = để nền tảng nguồn / multime.ai tự nhận diện.
	DefaultLanguage string `mapstructure:"DEFAULT_LANGUAGE"`

	// Hình thức thu thập đang bật. Mode B/C cần TTS/LLM thật nên mặc định tắt;
	// UI hiển thị mờ và API từ chối cho tới khi bật ở đây.
	EnabledCollectModesRaw string `mapstructure:"ENABLED_COLLECT_MODES"`

	// Email này luôn được cấp quyền admin khi đăng nhập (không có mật khẩu
	// riêng — vẫn đăng nhập bằng tài khoản strongbody của họ).
	BootstrapAdminEmail string `mapstructure:"BOOTSTRAP_ADMIN_EMAIL"`
	// Role mặc định cho tài khoản đăng nhập lần đầu.
	DefaultUserRole string `mapstructure:"DEFAULT_USER_ROLE"`
	// Khoá AES-256 (base64) mã hoá token multime lưu trong DB.
	TokenEncryptionKey string `mapstructure:"TOKEN_ENCRYPTION_KEY"`
}

func Load(paths ...string) (*Config, error) {
	v := viper.New()
	v.SetConfigName(".env")
	v.SetConfigType("env")
	if len(paths) == 0 {
		paths = []string{".", "./backend", "/app"}
	}
	for _, p := range paths {
		v.AddConfigPath(p)
	}

	setDefaults(v)

	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := v.ReadInConfig(); err != nil {
		// Không có .env cũng chạy được — lấy hết từ biến môi trường.
		var nf viper.ConfigFileNotFoundError
		if !errors.As(err, &nf) {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("APP_ENV", "development")
	v.SetDefault("HTTP_PORT", 8080)
	v.SetDefault("LOG_LEVEL", "info")
	v.SetDefault("DB_MAX_CONNS", 10)
	v.SetDefault("REDIS_ADDR", "localhost:6379")
	v.SetDefault("REDIS_DB", 0)
	v.SetDefault("WORKER_CONCURRENCY", 10)
	v.SetDefault("JWT_ACCESS_TTL", "15m")
	v.SetDefault("JWT_REFRESH_TTL", "720h")
	v.SetDefault("S3_REGION", "us-east-1")
	v.SetDefault("S3_BUCKET", "voice-tool")
	v.SetDefault("S3_USE_PATH_STYLE", true)
	v.SetDefault("TTS_PROVIDER", "mock")
	v.SetDefault("STT_PROVIDER", "mock")
	v.SetDefault("LLM_PROVIDER", "mock")
	v.SetDefault("ANTHROPIC_MODEL", "claude-opus-5")
	v.SetDefault("THREEVOICES_BASE_URL", "https://3voices.win")
	v.SetDefault("YTDLP_PATH", "yt-dlp")
	v.SetDefault("FFMPEG_PATH", "ffmpeg")
	v.SetDefault("FFPROBE_PATH", "ffprobe")
	// Chỉ số quét tối ưu mặc định:
	//   60s  — đủ nhanh cho "breaking" mà không đụng rate-limit nền tảng.
	//   20   — số bài/vòng: bắt kịp kênh đăng dày, không tốn quota vô ích.
	//   50   — trần bài/vòng: chặn nổ chi phí AI khi kênh đăng ồ ạt.
	//   4    — quét 4 kênh song song: cân bằng throughput và rate-limit.
	v.SetDefault("BREAKING_SCAN_INTERVAL", "60s")
	v.SetDefault("SCAN_LIMIT_DEFAULT", 20)
	v.SetDefault("MAX_POSTS_PER_RUN_DEFAULT", 50)
	v.SetDefault("BREAKING_SCAN_PARALLELISM", 4)
	v.SetDefault("SCHEDULER_SYNC_INTERVAL", "30s")
	v.SetDefault("SKIPPED_LOG_RETENTION", "168h") // 7 ngày
	v.SetDefault("DEFAULT_LANGUAGE", "auto")
	// B/C đã chạy được (TTS 3voices + LLM) nên bật sẵn cả 3 hình thức.
	v.SetDefault("ENABLED_COLLECT_MODES", "A,B,C")

	v.SetDefault("MULTIME_BASE_URL", "https://voice-api.strongbody.ai")
	v.SetDefault("MULTIME_AUTH_BASE_URL", "https://api-v2.strongbody.ai")
	v.SetDefault("MULTIME_SITE_URL", "https://multime.ai")
	v.SetDefault("MULTIME_VISIBILITY", "public")
	v.SetDefault("MULTIME_PUBLIC_DOWNLOAD", false)
	v.SetDefault("DEFAULT_USER_ROLE", "user")

	// AutomaticEnv chỉ thấy key đã biết khi dùng Unmarshal -> bind tường minh.
	for _, k := range allKeys {
		_ = v.BindEnv(k)
	}
}

var allKeys = []string{
	"APP_ENV", "HTTP_PORT", "LOG_LEVEL",
	"DATABASE_URL", "DB_MAX_CONNS", "REDIS_ADDR", "REDIS_PASSWORD", "REDIS_DB", "WORKER_CONCURRENCY",
	"JWT_SECRET", "JWT_ACCESS_TTL", "JWT_REFRESH_TTL",
	"S3_ENDPOINT", "S3_REGION", "S3_BUCKET", "S3_ACCESS_KEY", "S3_SECRET_KEY",
	"S3_USE_PATH_STYLE", "S3_PUBLIC_BASE_URL",
	"TTS_PROVIDER", "STT_PROVIDER", "LLM_PROVIDER",
	"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "ANTHROPIC_MODEL",
	"YTDLP_PATH", "FFMPEG_PATH", "FFPROBE_PATH",
	"MULTIME_BASE_URL", "MULTIME_AUTH_BASE_URL", "MULTIME_SITE_URL",
	"MULTIME_VISIBILITY", "MULTIME_CATEGORY_IDS", "MULTIME_DEFAULT_HASHTAGS",
	"MULTIME_PUBLIC_DOWNLOAD",
	"BREAKING_SCAN_INTERVAL", "SCAN_LIMIT_DEFAULT", "MAX_POSTS_PER_RUN_DEFAULT",
	"BREAKING_SCAN_PARALLELISM", "SCHEDULER_SYNC_INTERVAL", "SKIPPED_LOG_RETENTION",
	"DEFAULT_LANGUAGE", "ENABLED_COLLECT_MODES", "BOOTSTRAP_ADMIN_EMAIL", "DEFAULT_USER_ROLE",
	"TOKEN_ENCRYPTION_KEY",
	"THREEVOICES_API_KEY", "THREEVOICES_BASE_URL", "THREEVOICES_VOICE_ID",
}

func (c *Config) validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL là bắt buộc")
	}
	if c.JWTSecret == "" {
		return fmt.Errorf("JWT_SECRET là bắt buộc")
	}
	// Token multime của user được lưu trong DB -> luôn phải mã hoá.
	if c.TokenEncryptionKey == "" {
		return fmt.Errorf("TOKEN_ENCRYPTION_KEY là bắt buộc (sinh bằng `make gen-key`)")
	}
	if role := Role(c.DefaultUserRole); role != "admin" && role != "editor" && role != "user" {
		return fmt.Errorf("DEFAULT_USER_ROLE phải là admin, editor hoặc user")
	}
	return nil
}

// Role là alias tránh import vòng giữa config và domain.
type Role string

func (c *Config) IsProduction() bool { return c.Env == "production" }

// EnabledCollectModes trả về các hình thức thu thập đang bật, mặc định chỉ A.
func (c *Config) EnabledCollectModes() []domain.CollectMode {
	out := make([]domain.CollectMode, 0, 3)
	for _, part := range strings.Split(c.EnabledCollectModesRaw, ",") {
		mode := domain.CollectMode(strings.ToUpper(strings.TrimSpace(part)))
		if mode.Valid() {
			out = append(out, mode)
		}
	}
	if len(out) == 0 {
		out = append(out, domain.ModeExtract)
	}
	return out
}

// MultimeCategoryIDs parse "12,34" thành danh sách category id.
func (c *Config) MultimeCategoryIDs() []int64 {
	out := make([]int64, 0, 4)
	for _, part := range strings.Split(c.MultimeCategoryIDsRaw, ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err == nil && id > 0 {
			out = append(out, id)
		}
	}
	return out
}

// MultimeDefaultHashtags là hashtag dùng khi Voice chưa có hashtag riêng —
// multime yêu cầu ít nhất 1 hashtag hoặc category cho mỗi bài đăng.
func (c *Config) MultimeDefaultHashtags() []string {
	return SplitHashtags(c.MultimeDefaultHashtagRaw)
}

// SplitHashtags tách chuỗi hashtag tự do thành từng thẻ đã chuẩn hoá.
// Nhận cả "#a #b", "a, b" và "#a,#b"; bỏ dấu # vì API nhận thẻ trần.
func SplitHashtags(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case ',', ';', '#', ' ', '\t', '\n', '\r':
			return true
		}
		return false
	})

	seen := make(map[string]struct{}, len(fields))
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		tag := strings.TrimSpace(f)
		if tag == "" {
			continue
		}
		if _, dup := seen[tag]; dup {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}
