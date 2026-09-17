package service

import (
	"context"
	"hash/fnv"
	"log/slog"
	"sync"
	"time"

	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/repository"
)

// defaultPlatformGap — khoảng nghỉ tối thiểu giữa 2 lần gọi yt-dlp TỚI CÙNG
// MỘT NỀN TẢNG.
//
// Vì sao cần: watermark đã giới hạn SỐ BÀI phải lấy, nhưng không giới hạn gì về
// nhịp. N kênh YouTube cùng tần suất sẽ bắn cùng một giây, và nền tảng nhìn vào
// chỉ thấy một IP máy chủ gọi N request liên tiếp — đúng cái kích hoạt bot-check
// của YouTube và 429 của Facebook. Đây là bậc rẻ nhất của thang xử lý, làm trước
// khi nghĩ tới proxy.
const defaultPlatformGap = 3 * time.Second

// KeyGate cho phép TỐI ĐA `slots` lần gọi đồng thời TRÊN MỖI KHOÁ, và ép một
// khoảng nghỉ giữa hai lần liên tiếp của cùng khoá.
//
// Hai chỗ dùng, cùng một bài toán "đừng dồn request vào một hạn mức":
//
//	nền tảng  khoá = tên nền tảng, 1 slot, nghỉ 3s  — xem NewPlatformGate.
//	TTS       khoá = API key, 2 slot, nghỉ 6s       — xem NewTTSGate.
//
// Giới hạn theo TỪNG KHOÁ chứ không phải toàn cục: chặn toàn cục thì một kênh
// YouTube chậm sẽ chặn luôn việc quét TikTok, và một người chạy hàng loạt sẽ
// chặn luôn key của người khác — trong khi hai bên không chia sẻ hạn mức nào.
//
// Phạm vi là 1 process. Chạy nhiều worker thì mỗi process tự giữ nhịp của mình
// — không hoàn hảo, nhưng nó biến "N việc cùng bắn một giây" thành "N/số worker
// rải đều", tức là phần lớn vấn đề, mà không cần thêm khoá phân tán.
type KeyGate struct {
	gap   time.Duration
	slots int

	// overrides: nhịp riêng cho một vài khoá. Khai lúc dựng, không đổi sau đó,
	// nên đọc không cần khoá.
	//
	// Có mặt vì "mỗi nền tảng một hạn mức" không có nghĩa là các hạn mức đó
	// bằng nhau: Instagram siết chặt hơn hẳn Facebook và X (xem app.go). Hạ
	// nhịp chung xuống mức của nền tảng khắt khe nhất thì hai nền tảng còn lại
	// chậm đi mà chẳng vì lý do gì.
	overrides map[string]gateSetting

	mu    sync.Mutex
	lanes map[string]*gateLane
}

type gateSetting struct {
	gap   time.Duration
	slots int
}

type gateLane struct {
	// token sức chứa = số lần gọi đồng thời cho phép trên khoá này.
	token chan struct{}
	// gap của riêng làn này — mặc định là gap của gate, trừ khi có override.
	gap    time.Duration
	mu     sync.Mutex
	lastAt time.Time
}

// NewKeyGate dựng gate với số slot và khoảng nghỉ tuỳ ý.
func NewKeyGate(gap time.Duration, slots int) *KeyGate {
	if gap < 0 {
		gap = 0
	}
	if slots <= 0 {
		slots = 1
	}
	return &KeyGate{gap: gap, slots: slots, lanes: map[string]*gateLane{}}
}

// NewPlatformGate — gate cho yt-dlp: 1 lần gọi mỗi nền tảng tại một thời điểm.
//
// Nối tiếp chứ không song song: hai tiến trình yt-dlp cùng đánh vào một nền
// tảng là cách nhanh nhất để ăn bot-check, và vòng quét không gấp tới mức đó.
func NewPlatformGate(gap time.Duration) *KeyGate {
	if gap <= 0 {
		gap = defaultPlatformGap
	}
	return NewKeyGate(gap, 1)
}

// NewTTSGate — gate cho nhà cung cấp TTS, khoá theo TỪNG API KEY.
//
// 3voices giới hạn 10 request/phút và 2 job đồng thời TRÊN MỖI KEY. Key khai
// theo từng người nên tải đã chia sẵn, nhưng WORKER_CONCURRENCY mặc định 10 × 2
// worker = 20 task song song vẫn có thể dồn hết vào một key khi một người chạy
// hàng loạt — và lúc đó 429 rơi vào đúng người đang vội.
//
// Vượt hạn mức không làm mất voice (Asynq retry với backoff), nên gate này mua
// sự ổn định chứ không cứu dữ liệu: xếp hàng 6 giây rẻ hơn một vòng retry.
func NewTTSGate(gap time.Duration, slots int) *KeyGate {
	if gap <= 0 {
		gap = 6 * time.Second
	}
	if slots <= 0 {
		slots = 2
	}
	return NewKeyGate(gap, slots)
}

// Acquire chờ tới lượt của khoá này. Hàm trả về phải được gọi khi xong
// (kể cả khi lỗi), nếu không khoá đó kẹt vĩnh viễn.
func (g *KeyGate) Acquire(ctx context.Context, key string) (func(), error) {
	if g == nil {
		return func() {}, nil
	}
	lane := g.lane(key)

	select {
	case lane.token <- struct{}{}:
	case <-ctx.Done():
		return func() {}, ctx.Err()
	}

	// Giữ nhịp: chưa đủ khoảng nghỉ kể từ lần trước thì chờ nốt phần còn thiếu.
	lane.mu.Lock()
	wait := lane.gap - time.Since(lane.lastAt)
	lane.mu.Unlock()

	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			<-lane.token
			return func() {}, ctx.Err()
		}
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			lane.mu.Lock()
			// Đóng mốc lúc XONG chứ không lúc bắt đầu: khoảng nghỉ phải nằm
			// giữa hai lần gọi, còn bản thân lần gọi kéo dài bao lâu thì tuỳ việc.
			lane.lastAt = time.Now()
			lane.mu.Unlock()
			<-lane.token
		})
	}, nil
}

func (g *KeyGate) lane(key string) *gateLane {
	g.mu.Lock()
	defer g.mu.Unlock()
	lane, ok := g.lanes[key]
	if !ok {
		gap, slots := g.gap, g.slots
		if o, has := g.overrides[key]; has {
			gap, slots = o.gap, o.slots
		}
		lane = &gateLane{token: make(chan struct{}, slots), gap: gap}
		g.lanes[key] = lane
	}
	return lane
}

// SetKeyPace đặt nhịp riêng cho một khoá. Gọi TRƯỚC khi gate được dùng — làn
// đã dựng thì giữ nguyên nhịp cũ, vì đổi nhịp giữa chừng nghĩa là những request
// đang xếp hàng chờ theo một luật khác với những request vừa vào.
func (g *KeyGate) SetKeyPace(key string, gap time.Duration, slots int) {
	if g == nil {
		return
	}
	if gap < 0 {
		gap = 0
	}
	if slots <= 0 {
		slots = 1
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.overrides == nil {
		g.overrides = map[string]gateSetting{}
	}
	g.overrides[key] = gateSetting{gap: gap, slots: slots}
}

// ---------------------------------------------------------------------------
// Đếm lỗi bị chặn
// ---------------------------------------------------------------------------

// FetchStats đếm số lần từng nền tảng chặn ta, gộp theo ngày.
//
// Đây là BẬC 6 của thang xử lý rủi ro: đo trước, rồi mới quyết định có mua proxy
// hay không. Bộ phân loại lỗi yt-dlp đã gắn nhãn sẵn từng loại (xem
// domain.FetchBlockKind), ở đây chỉ cộng dồn.
type FetchStats struct {
	q   *repository.Queries
	log *slog.Logger
}

func NewFetchStats(q *repository.Queries, log *slog.Logger) *FetchStats {
	return &FetchStats{q: q, log: log}
}

// Record ghi nhận 1 lỗi lấy bài. Lỗi không phải do nền tảng chặn (thiếu ffmpeg,
// bug của ta) bị bỏ qua: đếm chúng vào đây chỉ làm nhiễu con số dùng để quyết
// định mua proxy.
//
// KHÔNG trả lỗi: đây là đo đạc, không phải nghiệp vụ. Ghi hỏng thì mất một số
// đếm, không được phép làm hỏng thêm một vòng quét vốn đã lỗi.
func (s *FetchStats) Record(ctx context.Context, platform string, err error) {
	if s == nil || err == nil {
		return
	}
	kind, ok := domain.FetchBlockKindOf(err)
	if !ok {
		return
	}
	if berr := s.q.BumpFetchErrorStat(ctx, repository.BumpFetchErrorStatParams{
		Platform: platform, Kind: string(kind),
	}); berr != nil {
		s.log.WarnContext(ctx, "không ghi được thống kê lỗi nền tảng",
			"error", berr, "platform", platform, "kind", kind)
	}
}

// FetchStatRow là 1 dòng thống kê cho màn Cài đặt.
type FetchStatRow struct {
	Day      string `json:"day"`
	Platform string `json:"platform"`
	Kind     string `json:"kind"`
	Count    int64  `json:"count"`
	LastAt   string `json:"last_at"`
}

// List trả thống kê `days` ngày gần nhất.
func (s *FetchStats) List(ctx context.Context, days int32) ([]FetchStatRow, error) {
	if days <= 0 {
		days = 7
	}
	rows, err := s.q.ListFetchErrorStats(ctx, days)
	if err != nil {
		return nil, err
	}
	out := make([]FetchStatRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, FetchStatRow{
			Day:      r.Day.Time.Format(time.DateOnly),
			Platform: r.Platform,
			Kind:     r.Kind,
			Count:    r.Count,
			LastAt:   r.LastAt.Format(time.RFC3339),
		})
	}
	return out, nil
}

// ---------------------------------------------------------------------------

// ScanJitter rải lệch thời điểm quét của từng kênh, TẤT ĐỊNH theo id kênh.
//
// Tất định chứ không ngẫu nhiên: ngẫu nhiên thì mỗi vòng dispatch lại xáo lại
// thứ tự, và hai kênh vẫn có lúc rơi trúng nhau. Băm theo id thì mỗi kênh có
// một chỗ đứng cố định trong cửa sổ, và chúng rải đều mãi mãi.
func ScanJitter(id string, window time.Duration) time.Duration {
	if window <= 0 {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(id))
	return time.Duration(uint64(h.Sum32()) % uint64(window))
}
