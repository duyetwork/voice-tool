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

// PlatformGate cho phép TỐI ĐA 1 lần gọi yt-dlp mỗi nền tảng tại một thời điểm,
// và ép một khoảng nghỉ ngắn giữa hai lần liên tiếp.
//
// Giới hạn theo TỪNG NỀN TẢNG chứ không phải toàn cục: chặn toàn cục thì một
// kênh YouTube chậm sẽ chặn luôn cả việc quét TikTok, trong khi hai nền tảng đó
// không hề chia sẻ hạn mức nào với nhau.
//
// Phạm vi là 1 process. Chạy nhiều worker thì mỗi process tự giữ nhịp của mình
// — không hoàn hảo, nhưng nó biến "N kênh cùng bắn một giây" thành "N/số worker
// kênh rải đều", tức là phần lớn vấn đề, mà không cần thêm khoá phân tán.
type PlatformGate struct {
	gap time.Duration

	mu    sync.Mutex
	lanes map[string]*platformLane
}

type platformLane struct {
	// token sức chứa 1 = mỗi nền tảng chỉ 1 yt-dlp chạy cùng lúc.
	token  chan struct{}
	mu     sync.Mutex
	lastAt time.Time
}

func NewPlatformGate(gap time.Duration) *PlatformGate {
	if gap <= 0 {
		gap = defaultPlatformGap
	}
	return &PlatformGate{gap: gap, lanes: map[string]*platformLane{}}
}

// Acquire chờ tới lượt của nền tảng này. Hàm trả về phải được gọi khi xong
// (kể cả khi lỗi), nếu không nền tảng đó kẹt vĩnh viễn.
func (g *PlatformGate) Acquire(ctx context.Context, platform string) (func(), error) {
	lane := g.lane(platform)

	select {
	case lane.token <- struct{}{}:
	case <-ctx.Done():
		return func() {}, ctx.Err()
	}

	// Giữ nhịp: chưa đủ khoảng nghỉ kể từ lần trước thì chờ nốt phần còn thiếu.
	lane.mu.Lock()
	wait := g.gap - time.Since(lane.lastAt)
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
			// giữa hai lần gọi, còn bản thân lần gọi kéo dài bao lâu thì tuỳ bài.
			lane.lastAt = time.Now()
			lane.mu.Unlock()
			<-lane.token
		})
	}, nil
}

func (g *PlatformGate) lane(platform string) *platformLane {
	g.mu.Lock()
	defer g.mu.Unlock()
	lane, ok := g.lanes[platform]
	if !ok {
		lane = &platformLane{token: make(chan struct{}, 1)}
		g.lanes[platform] = lane
	}
	return lane
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
