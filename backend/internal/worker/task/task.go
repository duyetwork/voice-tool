// Package task định nghĩa tên task và payload của Asynq — dùng chung giữa
// producer (API) và consumer (worker).
package task

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// Tên task.
const (
	TypeVoiceProcess = "voice:process"
	// TypePostMetadata lấy metadata gốc của Bài Post vừa tạo (không tạo voice).
	TypePostMetadata = "post:metadata"
	TypeVoicePublish = "voice:publish"
	// TypeBreakingDispatch là vòng lặp liên tục: mỗi lần chạy sẽ phát
	// breaking:scan cho từng kênh active rồi tự enqueue lại chính nó
	// (business rule #4 — không dùng cron).
	TypeBreakingDispatch = "breaking:dispatch"
	TypeBreakingScan     = "breaking:scan"
	TypeScheduledScan    = "scheduled:scan"
	// TypeMaintenanceCleanup dọn skipped_log quá hạn (chạy theo lịch ngày).
	TypeMaintenanceCleanup = "maintenance:cleanup"
)

// Tên queue — ưu tiên cao cho breaking (specs: F2 ưu tiên tốc độ).
const (
	QueueCritical = "critical" // breaking:scan
	QueueDefault  = "default"  // voice:process, voice:publish
	QueueLow      = "low"      // scheduled:scan
)

// Retry chung: 3 lần, backoff exponential (prompt mục 7).
const MaxRetry = 3

// RetryDelay là hàm backoff exponential dùng cho asynq.Config.
func RetryDelay(n int, _ error, _ *asynq.Task) time.Duration {
	base := 30 * time.Second
	d := base << n // 30s, 60s, 120s
	if max := 15 * time.Minute; d > max {
		return max
	}
	return d
}

type VoiceProcessPayload struct {
	SourcePostID string `json:"source_post_id"`
	ActorID      string `json:"actor_id"`
	// VoiceID là record voice `processing` đã được tạo sẵn lúc enqueue —
	// worker điền kết quả vào đúng record này. Rỗng nghĩa là task cũ enqueue
	// trước khi có trạng thái processing; worker tự tạo record như trước.
	VoiceID string `json:"voice_id,omitempty"`
}

type PostMetadataPayload struct {
	SourcePostID string `json:"source_post_id"`
}

type VoicePublishPayload struct {
	VoiceID string `json:"voice_id"`
	ActorID string `json:"actor_id"`
}

type BreakingScanPayload struct {
	ListID string `json:"list_id"`
}

type BreakingDispatchPayload struct{}

type ScheduledScanPayload struct {
	ListID string `json:"list_id"`
}

type MaintenanceCleanupPayload struct{}

func NewVoiceProcess(p VoiceProcessPayload) (*asynq.Task, error) {
	return newTask(TypeVoiceProcess, p, QueueDefault)
}

func NewPostMetadata(p PostMetadataPayload) (*asynq.Task, error) {
	return newTask(TypePostMetadata, p, QueueDefault)
}

func NewVoicePublish(p VoicePublishPayload) (*asynq.Task, error) {
	return newTask(TypeVoicePublish, p, QueueDefault)
}

func NewBreakingScan(p BreakingScanPayload) (*asynq.Task, error) {
	return newTask(TypeBreakingScan, p, QueueCritical)
}

func NewBreakingDispatch() (*asynq.Task, error) {
	return newTask(TypeBreakingDispatch, BreakingDispatchPayload{}, QueueCritical)
}

func NewScheduledScan(p ScheduledScanPayload) (*asynq.Task, error) {
	return newTask(TypeScheduledScan, p, QueueLow)
}

func NewMaintenanceCleanup() (*asynq.Task, error) {
	return newTask(TypeMaintenanceCleanup, MaintenanceCleanupPayload{}, QueueLow)
}

func newTask(name string, payload any, queue string) (*asynq.Task, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload %s: %w", name, err)
	}
	return asynq.NewTask(name, raw,
		asynq.Queue(queue),
		asynq.MaxRetry(MaxRetry),
		asynq.Timeout(30*time.Minute),
	), nil
}

// Decode parse payload của task về struct tương ứng.
func Decode[T any](t *asynq.Task) (T, error) {
	var out T
	if err := json.Unmarshal(t.Payload(), &out); err != nil {
		// Payload sai định dạng thì retry vô nghĩa.
		return out, fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}
	return out, nil
}
