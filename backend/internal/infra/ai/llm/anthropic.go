package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// systemPrompt cố định phần "khung" của Mode C; nội dung Prompt mẫu do user
// cấu hình được đưa vào như chỉ dẫn biên tập.
const systemPrompt = `Bạn là biên tập viên viết kịch bản đọc (voice-over) cho nội dung mạng xã hội.
Nhiệm vụ: viết lại văn bản nguồn theo đúng chỉ dẫn biên tập được cung cấp.

Yêu cầu đầu ra:
- Chỉ trả về nội dung kịch bản để đọc, không thêm lời dẫn, tiêu đề hay giải thích.
- Không dùng markdown, emoji, hay ký tự đặc biệt gây khó cho hệ thống đọc.
- Viết số, ngày tháng, viết tắt dưới dạng chữ để đọc tự nhiên.
- Giữ nguyên ngôn ngữ của văn bản nguồn, trừ khi chỉ dẫn yêu cầu khác.`

// Anthropic — LLM provider cho Mode C.
type Anthropic struct {
	client anthropic.Client
	model  string
}

var _ domain.LLMProvider = (*Anthropic)(nil)

func NewAnthropic(apiKey, model string) *Anthropic {
	if model == "" {
		model = "claude-opus-5"
	}
	return &Anthropic{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
		model:  model,
	}
}

func (a *Anthropic) Name() string { return "anthropic:" + a.model }

func (a *Anthropic) Generate(ctx context.Context, promptContent, sourceText string) (string, error) {
	userMessage := fmt.Sprintf("## Chỉ dẫn biên tập\n%s\n\n## Văn bản nguồn\n%s",
		strings.TrimSpace(promptContent), strings.TrimSpace(sourceText))

	resp, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(a.model),
		MaxTokens: 16000,
		System: []anthropic.TextBlockParam{{
			Text: systemPrompt,
			// Phần system cố định giữa mọi request -> cache để giảm chi phí.
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userMessage)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("gọi anthropic: %w", err)
	}

	if resp.StopReason == anthropic.StopReasonRefusal {
		return "", domain.Permanent(fmt.Errorf("anthropic từ chối xử lý nội dung: %s",
			resp.StopDetails.Explanation))
	}

	var b strings.Builder
	for _, block := range resp.Content {
		if text, ok := block.AsAny().(anthropic.TextBlock); ok {
			b.WriteString(text.Text)
		}
	}

	out := strings.TrimSpace(b.String())
	if out == "" {
		return "", fmt.Errorf("anthropic trả về nội dung rỗng (stop_reason=%s)", resp.StopReason)
	}
	return out, nil
}
