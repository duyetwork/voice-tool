package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/anthropics/anthropic-sdk-go/shared"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// anthropicMaxTokens — trần đầu ra. Batch 5 bài dài cần nhiều hơn 1 bài, nên
// nhân theo số mẩu thay vì để một hằng số phải vừa cho cả hai.
const (
	anthropicMaxTokens      = 16000
	anthropicBatchMaxTokens = 32000
	// anthropicBillingCooldown — hết tiền thì thử lại mỗi phút chỉ tạo ra rác
	// trong log; cho key nghỉ hẳn một tiếng rồi hãy hỏi lại.
	anthropicBillingCooldown = time.Hour
)

// batchToolName — tên "công cụ" mà model bắt buộc phải gọi để trả kết quả
// batch. Đây là cơ chế structured output native của Anthropic: ép tool_choice
// về đúng tool này thì đầu ra luôn khớp input_schema, không còn cửa trả văn
// xuôi kèm ```json.
const batchToolName = "tra_ket_qua"

// Anthropic — adapter cho 1 model Claude cụ thể, đã gắn sẵn API key.
type Anthropic struct {
	client anthropic.Client
	model  string
}

var _ domain.LLMProvider = (*Anthropic)(nil)

func NewAnthropic(apiKey, model string) *Anthropic {
	if model == "" {
		model = "claude-haiku-4-5-20251001"
	}
	return &Anthropic{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
		model:  model,
	}
}

func (a *Anthropic) Name() string { return "anthropic:" + a.model }

func (a *Anthropic) Generate(ctx context.Context, promptContent, sourceText string) (
	string, domain.LLMUsage, error,
) {
	resp, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(a.model),
		MaxTokens: anthropicMaxTokens,
		System:    a.system(systemPrompt),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewTextBlock(userMessage(promptContent, sourceText))),
		},
	})
	if err != nil {
		return "", domain.LLMUsage{}, anthropicClassify(err)
	}
	usage := anthropicUsage(resp)

	if resp.StopReason == anthropic.StopReasonRefusal {
		return "", usage, domain.LLMFail(domain.LLMFailContent,
			fmt.Errorf("anthropic từ chối xử lý nội dung: %s", resp.StopDetails.Explanation))
	}

	var b strings.Builder
	for _, block := range resp.Content {
		if text, ok := block.AsAny().(anthropic.TextBlock); ok {
			b.WriteString(text.Text)
		}
	}

	out := strings.TrimSpace(b.String())
	if out == "" {
		if resp.StopReason == anthropic.StopReasonMaxTokens {
			return "", usage, domain.LLMFail(domain.LLMFailContent,
				fmt.Errorf("anthropic cắt đầu ra vì quá dài (max_tokens)"))
		}
		return "", usage, emptyResult(a.Name(), "stop_reason="+string(resp.StopReason))
	}
	return out, usage, nil
}

func (a *Anthropic) GenerateBatch(ctx context.Context, promptContent string, items []string) (
	[]string, domain.LLMUsage, error,
) {
	schema := batchSchema(false)
	props, _ := schema["properties"].(map[string]any)

	resp, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(a.model),
		MaxTokens: anthropicBatchMaxTokens,
		System:    a.system(batchSystemPrompt),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewTextBlock(batchUserMessage(promptContent, items))),
		},
		Tools: []anthropic.ToolUnionParam{{OfTool: &anthropic.ToolParam{
			Name:        batchToolName,
			Description: param.NewOpt("Nộp kịch bản đọc đã viết lại cho từng văn bản nguồn."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: props,
				Required:   []string{"results"},
			},
		}}},
		// Ép gọi đúng tool đó, và đúng 1 lần.
		ToolChoice: anthropic.ToolChoiceUnionParam{OfTool: &anthropic.ToolChoiceToolParam{
			Name:                   batchToolName,
			DisableParallelToolUse: param.NewOpt(true),
		}},
	})
	if err != nil {
		return nil, domain.LLMUsage{}, anthropicClassify(err)
	}
	usage := anthropicUsage(resp)

	if resp.StopReason == anthropic.StopReasonRefusal {
		return nil, usage, domain.LLMFail(domain.LLMFailContent,
			fmt.Errorf("anthropic từ chối xử lý nội dung: %s", resp.StopDetails.Explanation))
	}

	for _, block := range resp.Content {
		use, ok := block.AsAny().(anthropic.ToolUseBlock)
		if !ok || use.Name != batchToolName {
			continue
		}
		out, err := parseBatch(string(use.Input), len(items))
		if err != nil {
			return nil, usage, domain.LLMFail(domain.LLMFailTransient,
				fmt.Errorf("anthropic batch: %w", err))
		}
		return out, usage, nil
	}

	return nil, usage, emptyResult(a.Name(),
		"không có tool_use nào, stop_reason="+string(resp.StopReason))
}

// anthropicUsage gom số token của một response.
//
// Token đọc từ cache (CacheReadInputTokens) tính vào đầu vào dù nhà cung cấp
// tính tiền chúng rẻ hơn token thường: bảng giá ở đây chỉ có MỘT đơn giá đầu
// vào, nên con số quy ra tiền là TRẦN TRÊN chứ không phải hoá đơn chính xác.
// Gộp như vậy vẫn tốt hơn bỏ qua — bỏ qua thì phần cache biến mất khỏi thống kê
// và system prompt trông như miễn phí.
func anthropicUsage(resp *anthropic.Message) domain.LLMUsage {
	if resp == nil {
		return domain.LLMUsage{}
	}
	return domain.LLMUsage{
		InputTokens: resp.Usage.InputTokens +
			resp.Usage.CacheReadInputTokens + resp.Usage.CacheCreationInputTokens,
		OutputTokens: resp.Usage.OutputTokens,
	}
}

// system dựng khối system kèm cache: phần này cố định giữa mọi request nên
// cache lại cắt được phần lớn chi phí input.
func (a *Anthropic) system(text string) []anthropic.TextBlockParam {
	return []anthropic.TextBlockParam{{
		Text:         text,
		CacheControl: anthropic.NewCacheControlEphemeralParam(),
	}}
}

// anthropicClassify đổi lỗi của SDK sang phân loại chung của router.
//
// SDK trả *anthropic.Error mang cả status code lẫn error type, nên ở đây không
// phải đoán từ chuỗi như hai nhà kia — trừ `invalid_request_error`, vốn gộp cả
// "key sai" lẫn "prompt quá dài" vào một mã.
func anthropicClassify(err error) error {
	if err == nil {
		return nil
	}

	var apiErr *anthropic.Error
	if !errors.As(err, &apiErr) {
		// Đứt mạng / timeout / context bị huỷ: key không liên quan.
		return domain.LLMFail(domain.LLMFailTransient, err)
	}

	msg := err.Error()
	switch apiErr.Type() {
	case shared.ErrorTypeRateLimitError:
		return domain.LLMFail(domain.LLMFailQuota, err)
	case shared.ErrorTypeBillingError:
		// Hết tiền: key vẫn đúng, nhưng chờ vài phút không giải quyết được gì.
		return domain.LLMFailAfter(domain.LLMFailQuota, anthropicBillingCooldown, err)
	case shared.ErrorTypeAuthenticationError, shared.ErrorTypePermissionError:
		return domain.LLMFail(domain.LLMFailAuth, err)
	case shared.ErrorTypeOverloadedError, shared.ErrorTypeTimeoutError, shared.ErrorTypeAPIError:
		return domain.LLMFail(domain.LLMFailTransient, err)
	case shared.ErrorTypeInvalidRequestError:
		if looksLikeAuth(msg) {
			return domain.LLMFail(domain.LLMFailAuth, err)
		}
		if looksLikeQuota(msg) {
			return domain.LLMFail(domain.LLMFailQuota, err)
		}
		// Còn lại của 400 là lỗi về chính request/nội dung: prompt quá dài,
		// schema sai. Xoay key chỉ lặp lại đúng lỗi đó trên mọi key.
		return domain.LLMFail(domain.LLMFailContent, err)
	}

	if apiErr.StatusCode == http.StatusTooManyRequests {
		return domain.LLMFail(domain.LLMFailQuota, err)
	}
	return domain.LLMFail(kindForStatus(apiErr.StatusCode), err)
}
