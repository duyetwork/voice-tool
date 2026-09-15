package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/pkg/secret"
	"github.com/strongbody/voice-tool/backend/internal/repository"
)

// ---------------------------------------------------------------------------
// Tham số của router
// ---------------------------------------------------------------------------

const (
	// transientRetries — số lần thử LẠI CHÍNH key đó khi gặp 5xx/đứt mạng,
	// trước khi bỏ sang key kế. Lỗi loại này thường tự khỏi trong vài giây;
	// bỏ key ngay là đẩy tải sang nhà đắt hơn vì một sự cố thoáng qua.
	transientRetries = 2
	// retryBaseDelay — nghỉ giữa 2 lần thử lại, có jitter. Nhiều voice chạy
	// song song mà nghỉ đúng bằng nhau thì chúng cùng đập lại nhà cung cấp
	// trong cùng một khoảnh khắc.
	retryBaseDelay = 800 * time.Millisecond

	// Khoảng nghỉ khi nhà cung cấp báo hết hạn mức mà KHÔNG nói nghỉ bao lâu.
	// Tăng dần theo số lần hỏng liên tiếp của chính key đó: 429 lẻ tẻ thì nghỉ
	// ngắn, còn key đã cạn hạn mức ngày thì tự nó leo lên mức dài.
	quotaCooldownMin = time.Minute
	quotaCooldownMax = 6 * time.Hour

	// maxContentSwitch — lỗi do NỘI DUNG thì chỉ được đổi sang model kế tiếp
	// đúng 1 lần. Xem giải thích ở tryCandidates.
	maxContentSwitch = 1
)

// LLMResult là kết quả 1 lần gọi, kèm model THẬT đã chạy.
//
// Trả model ra ngoài vì nó được lưu lên voice (voice.llm_model_used): cùng một
// bộ API, hôm nay chạy gemini-2.5-flash-lite, mai hết quota thì chạy
// gpt-5.6-luna — không ghi lại thì không đối chiếu được chất lượng hay chi phí
// của một voice cụ thể.
type LLMResult struct {
	Text  string
	Model string
}

// LLMGenerator là thứ Engine cần: viết lại text bằng bộ API nào đó.
//
// Tách thành interface để Engine test được mà không cần DB lẫn mạng.
type LLMGenerator interface {
	Generate(ctx context.Context, setID *uuid.UUID, owner uuid.UUID,
		promptContent, sourceText string) (LLMResult, error)
	GenerateBatch(ctx context.Context, setID *uuid.UUID, owner uuid.UUID,
		promptContent string, items []string) ([]string, string, error)
}

// LLMRouter chọn key + model theo bộ API, tự chuyển dự phòng khi hết quota.
//
// KHÔNG XOAY VÒNG NGẪU NHIÊN. Thứ tự ứng viên là tất định — (vị trí trong chuỗi
// dự phòng, priority, id) — nên chừng nào key đầu chuỗi còn khoẻ thì mọi request
// đều rơi vào đúng nó. "Còn khoẻ hay không" đọc từ DB (disabled_at,
// cooldown_until) chứ không từ bộ nhớ của process: nhờ vậy mọi worker cùng thấy
// một trạng thái, và một key vừa bị 429 ở worker A thì worker B cũng biết đường
// tránh — thứ mà sticky-cache trong RAM không làm được.
type LLMRouter struct {
	q        *repository.Queries
	box      *secret.Box
	factory  domain.LLMFactory
	settings *Settings
	// fallback là provider cấu hình trong .env. Chỉ dùng khi KHÔNG chọn bộ API
	// nào — thực tế là dev (LLM_PROVIDER=mock).
	fallback domain.LLMProvider
	// usage ghi số token của mỗi lần gọi. Ghi Ở ĐÂY chứ không ở adapter: chỉ
	// router mới biết lần gọi này thuộc về ai và chạy bằng model nào sau khi
	// đã chuyển dự phòng.
	usage *AIUsage
	log   *slog.Logger
	// batcher gom những lần gọi đang cùng chờ vào 1 request (xem llmbatcher.go).
	batcher *llmBatcher
}

var _ LLMGenerator = (*LLMRouter)(nil)

type LLMRouterDeps struct {
	Queries  *repository.Queries
	Secret   *secret.Box
	Factory  domain.LLMFactory
	Settings *Settings
	Fallback domain.LLMProvider
	Usage    *AIUsage
	Logger   *slog.Logger
}

func NewLLMRouter(d LLMRouterDeps) *LLMRouter {
	r := &LLMRouter{
		q: d.Queries, box: d.Secret, factory: d.Factory,
		settings: d.Settings, fallback: d.Fallback, usage: d.Usage, log: d.Logger,
	}
	r.batcher = newLLMBatcher(r)
	return r
}

// ---------------------------------------------------------------------------
// Ứng viên
// ---------------------------------------------------------------------------

// candidate là 1 cặp (key, model) sẵn sàng để thử.
type candidate struct {
	key   repository.LlmApiKey
	step  domain.LLMChainStep
	label string
}

// skipped ghi lại vì sao một key bị loại khỏi danh sách ứng viên — để câu lỗi
// cuối cùng nói được "key nào, vì sao" thay vì chỉ "bộ API đã cạn".
type skipped struct {
	label  string
	reason string
}

func (r *LLMRouter) candidates(
	ctx context.Context,
	setID uuid.UUID,
) ([]candidate, []skipped, error) {
	keys, err := r.q.ListLLMAPIKeys(ctx, setID)
	if err != nil {
		return nil, nil, fmt.Errorf("đọc key của bộ API %s: %w", setID, err)
	}
	if len(keys) == 0 {
		return nil, nil, domain.Permanent(domain.Explain(
			"Bộ API đang chọn chưa có API key nào — thêm key ở mục AI Engine > LLM Model",
			fmt.Errorf("%w: llm_api_set %s rỗng", domain.ErrInvalidInput, setID)))
	}

	chain := r.settings.LLMChain(ctx)
	now := time.Now()

	var (
		out  []candidate
		skip []skipped
		// Một key bị loại có thể xuất hiện ở nhiều mắt xích của cùng 1 nhà;
		// chỉ nói về nó một lần.
		noted = map[uuid.UUID]bool{}
	)

	// Vòng ngoài là CHUỖI DỰ PHÒNG, vòng trong là key: thứ tự ưu tiên thuộc về
	// model (rẻ trước đắt sau), không thuộc về key. Đảo hai vòng này lại nghĩa
	// là key thứ nhất sẽ chạy hết mọi model đắt tiền trước khi key thứ hai được
	// thử ở model rẻ nhất.
	for _, step := range chain {
		for _, key := range keys {
			if domain.LLMProviderName(key.Provider) != step.Provider {
				continue
			}
			label := keyLabel(key, step.Model)

			switch {
			case key.DisabledAt != nil:
				if !noted[key.ID] {
					noted[key.ID] = true
					skip = append(skip, skipped{label,
						"đã tắt vì key sai hoặc bị thu hồi: " + deref(key.LastError)})
				}
				continue
			case key.CooldownUntil != nil && key.CooldownUntil.After(now):
				if !noted[key.ID] {
					noted[key.ID] = true
					skip = append(skip, skipped{label, fmt.Sprintf(
						"đang nghỉ tới %s (hết hạn mức)",
						key.CooldownUntil.Local().Format("15:04 02/01"))})
				}
				continue
			}

			out = append(out, candidate{key: key, step: step, label: label})
		}
	}
	return out, skip, nil
}

func keyLabel(key repository.LlmApiKey, model string) string {
	if label := strings.TrimSpace(deref(key.Label)); label != "" {
		return fmt.Sprintf("%s (%s) / %s", key.Provider, label, model)
	}
	return fmt.Sprintf("%s / %s", key.Provider, model)
}

// ---------------------------------------------------------------------------
// Gọi
// ---------------------------------------------------------------------------

// Generate viết lại 1 mẩu text.
//
// Đi qua bộ gom lô khi batch đang bật: nhiều task chạy song song sau một vòng
// quét sẽ gặp nhau ở đó và cùng đi trong 1 request (xem llmbatcher.go). Tắt
// batch, hoặc voice không gắn bộ API, thì gọi thẳng.
func (r *LLMRouter) Generate(
	ctx context.Context,
	setID *uuid.UUID,
	owner uuid.UUID,
	promptContent, sourceText string,
) (LLMResult, error) {
	if setID != nil && r.settings.LLMBatch(ctx).Enabled {
		return r.batcher.submit(ctx, *setID, owner, promptContent, sourceText)
	}
	return r.generateOne(ctx, setID, owner, promptContent, sourceText)
}

// generateOne là đường gọi TRỰC TIẾP, không qua bộ gom lô.
//
// Tách khỏi Generate để đường hạ-về-gọi-lẻ của GenerateBatch không quay ngược
// vào bộ gom lô — làm thế là đệ quy vô tận giữa hai hàm.
func (r *LLMRouter) generateOne(
	ctx context.Context,
	setID *uuid.UUID,
	owner uuid.UUID,
	promptContent, sourceText string,
) (LLMResult, error) {
	if setID == nil {
		text, name, err := r.viaFallback(ctx, owner,
			func(p domain.LLMProvider) (string, domain.LLMUsage, error) {
				return p.Generate(ctx, promptContent, sourceText)
			})
		return LLMResult{Text: text, Model: name}, err
	}

	var out LLMResult
	err := r.run(ctx, *setID, owner, func(p domain.LLMProvider) (domain.LLMUsage, error) {
		text, used, err := p.Generate(ctx, promptContent, sourceText)
		if err != nil {
			return used, err
		}
		if strings.TrimSpace(text) == "" {
			return used, domain.LLMFail(domain.LLMFailTransient,
				fmt.Errorf("%s trả về nội dung rỗng", p.Name()))
		}
		out = LLMResult{Text: strings.TrimSpace(text), Model: p.Name()}
		return used, nil
	})
	return out, err
}

// GenerateBatch viết lại nhiều mẩu trong 1 request, và TỰ HẠ VỀ GỌI LẺ khi lô
// hỏng.
//
// Một mẩu xấu (quá dài, bị chặn vì chính sách) không được làm chết N-1 mẩu còn
// lại: nếu cả lô thất bại vì lý do không phải hạn mức, mỗi mẩu được gọi riêng và
// chỉ mẩu thật sự có vấn đề mới trả lỗi.
func (r *LLMRouter) GenerateBatch(
	ctx context.Context,
	setID *uuid.UUID,
	owner uuid.UUID,
	promptContent string,
	items []string,
) ([]string, string, error) {
	if len(items) == 0 {
		return nil, "", nil
	}
	// Batch 1 mẩu chỉ là gọi lẻ đội thêm một lớp JSON schema — đi thẳng.
	if len(items) == 1 {
		res, err := r.generateOne(ctx, setID, owner, promptContent, items[0])
		if err != nil {
			return nil, "", err
		}
		return []string{res.Text}, res.Model, nil
	}

	if setID == nil {
		out, name, err := r.batchViaFallback(ctx, owner, promptContent, items)
		return out, name, err
	}

	var (
		out   []string
		model string
	)
	err := r.run(ctx, *setID, owner, func(p domain.LLMProvider) (domain.LLMUsage, error) {
		texts, used, err := p.GenerateBatch(ctx, promptContent, items)
		if err != nil {
			return used, err
		}
		out, model = texts, p.Name()
		return used, nil
	})
	if err == nil {
		return out, model, nil
	}

	// Hết hạn mức thì gọi lẻ cũng hết hạn mức — hạ về gọi lẻ ở đây chỉ nhân số
	// request lên N lần rồi thất bại y như cũ.
	if domain.LLMFailureOf(err).Kind == domain.LLMFailQuota {
		return nil, "", err
	}

	r.log.WarnContext(ctx, "batch LLM thất bại, hạ về gọi lẻ từng mẩu",
		"error", err, "items", len(items), "set_id", *setID)

	out = make([]string, len(items))
	for i, item := range items {
		res, err := r.generateOne(ctx, setID, owner, promptContent, item)
		if err != nil {
			return nil, "", fmt.Errorf("mẩu %d/%d: %w", i+1, len(items), err)
		}
		out[i] = res.Text
		model = res.Model
	}
	return out, model, nil
}

// run là vòng lặp chính: dựng ứng viên, thử lần lượt, cập nhật sức khoẻ key.
func (r *LLMRouter) run(
	ctx context.Context,
	setID uuid.UUID,
	owner uuid.UUID,
	call func(domain.LLMProvider) (domain.LLMUsage, error),
) error {
	// Người dùng chỉ được chạy bằng bộ của mình / được chia sẻ / admin đã bật
	// hiển thị. Chặn ở đây chứ không chỉ lúc tạo voice: bộ có thể bị gỡ chia sẻ
	// sau khi voice đã nằm trong hàng đợi.
	if _, err := r.q.CanUseLLMAPISet(ctx, repository.CanUseLLMAPISetParams{
		ID: setID, UserID: owner,
	}); err != nil {
		return domain.Permanent(domain.Explain(
			"Bộ API đang chọn không còn dùng được cho tài khoản này — chọn bộ khác rồi chạy lại",
			fmt.Errorf("kiểm tra quyền dùng llm_api_set %s của user %s: %w", setID, owner, err)))
	}

	cands, skip, err := r.candidates(ctx, setID)
	if err != nil {
		return err
	}
	if len(cands) == 0 {
		return r.exhausted(setID, skip, nil)
	}

	var (
		failures       []skipped
		contentSwitch  int
		lastContentErr error
		skipModel      string
	)

	for _, c := range cands {
		// Sau một lỗi NỘI DUNG, mọi ứng viên còn lại dùng ĐÚNG model đó đều vô
		// nghĩa: chính model ấy vừa từ chối nội dung này, đổi key không đổi
		// được câu trả lời.
		if skipModel != "" && c.step.Model == skipModel {
			continue
		}

		provider, err := r.providerFor(c)
		if err != nil {
			failures = append(failures, skipped{c.label, domain.UserMessage(err)})
			continue
		}

		err = r.attempt(ctx, owner, c, provider, call)
		if err == nil {
			r.markOK(ctx, setID, c)
			return nil
		}

		fail := domain.LLMFailureOf(err)
		r.log.WarnContext(ctx, "mắt xích LLM thất bại",
			"error", err, "kind", fail.Kind.String(), "candidate", c.label, "set_id", setID)
		failures = append(failures, skipped{c.label, domain.UserMessage(err)})

		switch fail.Kind {
		case domain.LLMFailQuota:
			r.markCooldown(ctx, c, fail)

		case domain.LLMFailAuth:
			r.markDisabled(ctx, c, fail)

		case domain.LLMFailContent:
			// Lỗi do chính nội dung này, không phải do key. Xoay hết key của
			// mọi nhà trên một input chắc chắn thất bại sẽ đốt sạch hạn mức rồi
			// báo "hết quota" — trong khi quota vẫn còn nguyên, và người dùng
			// đi sửa sai chỗ. Chỉ cho đổi sang MODEL kế tiếp, đúng 1 lần.
			lastContentErr = err
			skipModel = c.step.Model
			contentSwitch++
			if contentSwitch > maxContentSwitch {
				return domain.Permanent(domain.Explain(
					"Nhà cung cấp LLM từ chối nội dung này (bị chặn hoặc quá dài) — sửa nội dung nguồn rồi chạy lại",
					err))
			}

		default:
			r.markFailure(ctx, c, fail)
		}
	}

	// Chạy hết ứng viên mà lần cuối cùng là lỗi nội dung: nói đúng điều đó thay
	// vì "bộ API đã cạn" — quota không liên quan gì ở đây.
	if lastContentErr != nil {
		return domain.Permanent(domain.Explain(
			"Nhà cung cấp LLM từ chối nội dung này (bị chặn hoặc quá dài) — sửa nội dung nguồn rồi chạy lại",
			lastContentErr))
	}
	return r.exhausted(setID, skip, failures)
}

// attempt gọi 1 provider, thử lại tại chỗ với lỗi tạm thời.
//
// Ghi usage sau MỖI lần thử, kể cả lần hỏng và kể cả các lần retry: ba lần thử
// trên cùng một key là ba lần nhà cung cấp tính token đầu vào, và một bảng chi
// phí chỉ đếm lần thành công sẽ thiếu đúng phần đắt nhất của những ngày tệ.
func (r *LLMRouter) attempt(
	ctx context.Context,
	owner uuid.UUID,
	c candidate,
	provider domain.LLMProvider,
	call func(domain.LLMProvider) (domain.LLMUsage, error),
) error {
	var err error
	for i := 0; i <= transientRetries; i++ {
		var used domain.LLMUsage
		used, err = call(provider)
		r.recordUsage(ctx, owner, string(c.step.Provider), c.step.Model, used, err)
		if err == nil {
			return nil
		}
		// Chỉ lỗi TẠM THỜI mới đáng thử lại trên cùng key. Hết quota mà thử
		// lại ngay thì vẫn hết quota, còn key sai thì vẫn sai.
		if domain.LLMFailureOf(err).Kind != domain.LLMFailTransient {
			return err
		}
		if i == transientRetries {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(jitter(retryBaseDelay << i)):
		}
	}
	return err
}

func (r *LLMRouter) providerFor(c candidate) (domain.LLMProvider, error) {
	apiKey, err := r.box.Decrypt(c.key.ApiKeyEncrypted)
	if err != nil {
		return nil, domain.Explain(
			"Không giải mã được API key — khai lại key ở mục AI Engine > LLM Model",
			fmt.Errorf("giải mã llm_api_key %s: %w", c.key.ID, err))
	}
	return r.factory.For(domain.LLMCredential{
		Provider: domain.LLMProviderName(c.key.Provider),
		APIKey:   apiKey,
		Model:    c.step.Model,
	})
}

// ---------------------------------------------------------------------------
// Cập nhật sức khoẻ key — ghi hỏng cũng KHÔNG làm hỏng voice
// ---------------------------------------------------------------------------

func (r *LLMRouter) markOK(ctx context.Context, setID uuid.UUID, c candidate) {
	if err := r.q.MarkLLMAPIKeyOK(ctx, c.key.ID); err != nil {
		r.log.WarnContext(ctx, "không ghi được trạng thái key LLM", "error", err, "key_id", c.key.ID)
	}
	if err := r.q.TouchLLMAPISetUsed(ctx, setID); err != nil {
		r.log.WarnContext(ctx, "không ghi được last_used_at của bộ API", "error", err, "set_id", setID)
	}
}

func (r *LLMRouter) markCooldown(ctx context.Context, c candidate, fail *domain.LLMError) {
	until := time.Now().Add(cooldownFor(fail, c.key.ConsecutiveFailures))
	msg := domain.UserMessage(fail)
	if err := r.q.MarkLLMAPIKeyCooldown(ctx, repository.MarkLLMAPIKeyCooldownParams{
		ID: c.key.ID, CooldownUntil: &until, LastError: &msg,
	}); err != nil {
		r.log.WarnContext(ctx, "không ghi được cooldown key LLM", "error", err, "key_id", c.key.ID)
	}
}

func (r *LLMRouter) markDisabled(ctx context.Context, c candidate, fail *domain.LLMError) {
	msg := domain.UserMessage(fail)
	if err := r.q.MarkLLMAPIKeyDisabled(ctx, repository.MarkLLMAPIKeyDisabledParams{
		ID: c.key.ID, LastError: &msg,
	}); err != nil {
		r.log.WarnContext(ctx, "không tắt được key LLM hỏng", "error", err, "key_id", c.key.ID)
	}
}

func (r *LLMRouter) markFailure(ctx context.Context, c candidate, fail *domain.LLMError) {
	msg := domain.UserMessage(fail)
	if err := r.q.MarkLLMAPIKeyFailure(ctx, repository.MarkLLMAPIKeyFailureParams{
		ID: c.key.ID, LastError: &msg,
	}); err != nil {
		r.log.WarnContext(ctx, "không ghi được lỗi key LLM", "error", err, "key_id", c.key.ID)
	}
}

// cooldownFor chốt khoảng nghỉ cho 1 key vừa báo hết hạn mức.
//
// Ưu tiên tuyệt đối con số của nhà cung cấp (Retry-After): họ biết chính xác
// khi nào hạn mức hồi, ta thì đoán. Không có thì tăng gấp đôi theo số lần hỏng
// liên tiếp — 429 lẻ tẻ chỉ nghỉ 1 phút, còn key đã cạn hạn mức NGÀY thì sau
// vài lần tự leo lên mức hàng giờ thay vì hỏi lại mỗi phút suốt cả ngày.
func cooldownFor(fail *domain.LLMError, consecutiveFailures int32) time.Duration {
	if fail.RetryAfter > 0 {
		return fail.RetryAfter
	}
	d := quotaCooldownMin
	for i := int32(0); i < consecutiveFailures && d < quotaCooldownMax; i++ {
		d *= 2
	}
	return min(d, quotaCooldownMax)
}

// exhausted dựng câu lỗi cuối cùng, kèm lý do của TỪNG key.
//
// Không trả mỗi chữ "bộ API đã cạn": người dùng cần biết nên đi nạp tiền cho
// nhà nào, hay nên dán lại key nào.
func (r *LLMRouter) exhausted(setID uuid.UUID, skip, failures []skipped) error {
	var lines []string
	for _, s := range append(append([]skipped{}, skip...), failures...) {
		lines = append(lines, s.label+": "+s.reason)
	}
	detail := strings.Join(lines, "; ")
	if detail == "" {
		detail = "không có key nào dùng được"
	}
	return domain.Explain(
		"Bộ API đã cạn — "+detail,
		fmt.Errorf("llm_api_set %s: hết ứng viên (%s)", setID, detail))
}

// ---------------------------------------------------------------------------
// Đường dự phòng khi voice không gắn bộ API nào
// ---------------------------------------------------------------------------

func (r *LLMRouter) viaFallback(
	ctx context.Context,
	owner uuid.UUID,
	call func(domain.LLMProvider) (string, domain.LLMUsage, error),
) (string, string, error) {
	if r.fallback == nil {
		return "", "", domain.Permanent(domain.Explain(
			"Voice này chưa chọn Bộ API — chọn một bộ ở mục AI Engine > LLM Model rồi chạy lại",
			fmt.Errorf("%w: không có llm_api_set_id và cũng không có LLM_PROVIDER trong .env",
				domain.ErrInvalidInput)))
	}
	text, used, err := call(r.fallback)
	provider, model := splitProviderName(r.fallback.Name())
	r.recordUsage(ctx, owner, provider, model, used, err)
	if err != nil {
		return "", r.fallback.Name(), err
	}
	return strings.TrimSpace(text), r.fallback.Name(), nil
}

func (r *LLMRouter) batchViaFallback(
	ctx context.Context,
	owner uuid.UUID,
	promptContent string,
	items []string,
) ([]string, string, error) {
	if r.fallback == nil {
		_, _, err := r.viaFallback(ctx, owner, nil)
		return nil, "", err
	}
	out, used, err := r.fallback.GenerateBatch(ctx, promptContent, items)
	provider, model := splitProviderName(r.fallback.Name())
	r.recordUsage(ctx, owner, provider, model, used, err)
	return out, r.fallback.Name(), err
}

// recordUsage ghi 1 lần gọi vào bảng chi phí.
//
// Bỏ qua lần gọi KHÔNG tốn token nào và cũng không lỗi: đó là mock (dev), và
// một dòng "mock, 0 token" mỗi lần chạy chỉ làm loãng bảng thống kê thật.
func (r *LLMRouter) recordUsage(
	ctx context.Context,
	owner uuid.UUID,
	provider, model string,
	used domain.LLMUsage,
	callErr error,
) {
	if used.InputTokens == 0 && used.OutputTokens == 0 && callErr == nil {
		return
	}
	r.usage.Record(ctx, owner, domain.AIUsageEvent{
		Kind:         domain.AIKindLLM,
		Provider:     provider,
		Model:        model,
		InputTokens:  used.InputTokens,
		OutputTokens: used.OutputTokens,
		OK:           callErr == nil,
	})
}

// splitProviderName tách "anthropic:claude-haiku-4-5" thành nhà và model —
// đường dự phòng chỉ có tên gộp, trong khi bảng giá tra theo từng phần.
func splitProviderName(name string) (provider, model string) {
	if i := strings.Index(name, ":"); i >= 0 {
		return name[:i], name[i+1:]
	}
	return name, ""
}

// errBatchShortResult chỉ xảy ra nếu bảo đảm "đủ phần tử" của GenerateBatch bị
// vỡ. Có tên riêng để mẩu chờ nhận được một lỗi thay vì treo vĩnh viễn.
var errBatchShortResult = errors.New("lô LLM trả về thiếu kết quả")

// jitter rải lệch khoảng nghỉ ±25%: nhiều voice chạy song song mà nghỉ đúng
// bằng nhau thì chúng cùng đập lại nhà cung cấp trong cùng một khoảnh khắc.
func jitter(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	spread := float64(d) * 0.25
	return d + time.Duration((rand.Float64()*2-1)*spread)
}
