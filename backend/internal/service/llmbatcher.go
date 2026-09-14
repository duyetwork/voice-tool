package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/google/uuid"
)

// llmBatcher gom những lần gọi LLM ĐANG CÙNG CHỜ vào một request.
//
// Vì sao gom ở đây chứ không ở chỗ quét kênh: batch chỉ đúng khi N mẩu text là
// N mẩu mà hệ thống THẬT SỰ sẽ đọc. Text đó chỉ có sau khi worker đã tải bài và
// lấy phụ đề — ở thời điểm quét kênh mới chỉ có caption, gom sớm hơn nghĩa là
// viết lại một nội dung khác với nội dung sẽ được đọc.
//
// Gom ở tầng router thì không phải đụng gì tới pipeline: mỗi task voice:process
// vẫn chạy y như cũ, chỉ là khi tới bước gọi LLM thì nó đứng chờ vài giây xem có
// ai cùng bộ API + cùng Prompt mẫu đang chờ không. Một vòng quét sinh ra N task
// chạy song song, nên chúng gặp nhau ở đây; còn người tạo voice lẻ trên UI thì
// tự nhiên thành lô 1 mẩu, đúng như bản chất của nó.
type llmBatcher struct {
	router *LLMRouter

	mu     sync.Mutex
	groups map[string]*batchGroup
}

// batchGroup là một lô đang gom. Mọi mẩu trong lô dùng CHUNG bộ API, CHUNG chủ
// sở hữu và CHUNG Prompt mẫu — ba thứ đó nằm trong khoá của lô.
//
// Chủ sở hữu nằm trong khoá vì router kiểm tra quyền dùng bộ API theo từng
// người: gom hai người vào một request thì chỉ còn một lần kiểm tra, và người
// chưa được chia sẻ bộ vẫn chạy được nhờ đứng ké lô của người khác.
type batchGroup struct {
	key    string
	setID  uuid.UUID
	owner  uuid.UUID
	prompt string

	items []*batchItem
	chars int
	timer *time.Timer
}

type batchItem struct {
	text string
	done chan batchOutcome
}

type batchOutcome struct {
	res LLMResult
	err error
}

func newLLMBatcher(router *LLMRouter) *llmBatcher {
	return &llmBatcher{router: router, groups: map[string]*batchGroup{}}
}

// submit đưa 1 mẩu vào lô và chờ kết quả của chính nó.
//
// Lô được gửi đi khi đủ `size` mẩu, khi tổng ký tự vượt `max_chars`, hoặc khi
// hết thời gian chờ — tuỳ điều kiện nào tới trước.
func (b *llmBatcher) submit(
	ctx context.Context,
	setID, owner uuid.UUID,
	promptContent, sourceText string,
) (LLMResult, error) {
	cfg := b.router.settings.LLMBatch(ctx)

	item := &batchItem{text: sourceText, done: make(chan batchOutcome, 1)}
	key := batchKey(setID, owner, promptContent)

	b.mu.Lock()
	group, ok := b.groups[key]
	if !ok {
		group = &batchGroup{key: key, setID: setID, owner: owner, prompt: promptContent}
		b.groups[key] = group
		// Hẹn giờ chốt lô. Dùng context tách khỏi caller: người gọi đầu tiên có
		// thể bị huỷ trước khi lô đầy, nhưng những người vào sau vẫn đang chờ.
		group.timer = time.AfterFunc(cfg.Wait(), func() { b.flush(key) })
	}
	group.items = append(group.items, item)
	group.chars += len(sourceText)

	full := len(group.items) >= cfg.Size || group.chars >= cfg.MaxChars
	if full {
		b.detach(group)
	}
	b.mu.Unlock()

	if full {
		go b.send(group)
	}

	select {
	case out := <-item.done:
		return out.res, out.err
	case <-ctx.Done():
		// Mẩu này bỏ cuộc (task bị huỷ) nhưng lô vẫn chạy tiếp cho những mẩu
		// còn lại — kênh `done` có bộ đệm nên goroutine gửi kết quả không kẹt.
		return LLMResult{}, ctx.Err()
	}
}

// flush là đường chốt lô theo THỜI GIAN.
func (b *llmBatcher) flush(key string) {
	b.mu.Lock()
	group, ok := b.groups[key]
	if ok {
		b.detach(group)
	}
	b.mu.Unlock()

	if ok {
		b.send(group)
	}
}

// detach gỡ lô khỏi bản đồ để mẩu tới sau mở lô mới. Gọi khi ĐANG giữ khoá.
func (b *llmBatcher) detach(group *batchGroup) {
	if group.timer != nil {
		group.timer.Stop()
	}
	delete(b.groups, group.key)
}

// send gọi LLM cho cả lô rồi phát kết quả về từng mẩu.
func (b *llmBatcher) send(group *batchGroup) {
	// Context riêng: mọi caller có thể đã bị huỷ, nhưng lô đã gom thì vẫn phải
	// chạy xong cho những mẩu còn chờ.
	ctx, cancel := context.WithTimeout(context.Background(), batchCallTimeout)
	defer cancel()

	texts := make([]string, 0, len(group.items))
	for _, item := range group.items {
		texts = append(texts, item.text)
	}

	// Lô 1 mẩu đi thẳng: bọc thêm một lớp JSON schema chỉ tốn token mà không
	// tiết kiệm được request nào.
	if len(texts) == 1 {
		res, err := b.router.generateOne(ctx, &group.setID, group.owner, group.prompt, texts[0])
		group.items[0].done <- batchOutcome{res: res, err: err}
		return
	}

	out, model, err := b.router.GenerateBatch(ctx, &group.setID, group.owner, group.prompt, texts)
	for i, item := range group.items {
		switch {
		case err != nil:
			item.done <- batchOutcome{err: err}
		case i < len(out):
			item.done <- batchOutcome{res: LLMResult{Text: out[i], Model: model}}
		default:
			// GenerateBatch đã đảm bảo đủ phần tử; nhánh này chỉ để không có
			// mẩu nào chờ vĩnh viễn nếu điều đó vỡ.
			item.done <- batchOutcome{err: errBatchShortResult}
		}
	}
}

// batchCallTimeout — trần cho 1 lô. Rộng hơn 1 lần gọi lẻ vì lô có thể phải hạ
// về gọi lẻ từng mẩu, và vì nó không còn bám theo context của caller nào.
const batchCallTimeout = 10 * time.Minute

// batchKey: cùng bộ API + cùng người + cùng Prompt mẫu thì mới gom chung được.
// Băm nội dung prompt thay vì dùng nguyên văn để khoá không phình theo độ dài
// prompt (prompt mẫu có thể dài vài nghìn ký tự).
func batchKey(setID, owner uuid.UUID, promptContent string) string {
	sum := sha256.Sum256([]byte(promptContent))
	return setID.String() + "|" + owner.String() + "|" + hex.EncodeToString(sum[:8])
}
