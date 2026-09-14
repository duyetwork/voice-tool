package llm

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

const openAIURL = "https://api.openai.com/v1/chat/completions"

// OpenAI gọi Chat Completions bằng net/http — cùng lý do với Gemini: request ở
// đây chỉ có một hình dạng, không đáng để kéo cả SDK vào.
type OpenAI struct {
	apiKey string
	model  string
}

var _ domain.LLMProvider = (*OpenAI)(nil)

func NewOpenAI(apiKey, model string) *OpenAI {
	return &OpenAI{apiKey: apiKey, model: model}
}

func (o *OpenAI) Name() string { return "openai:" + o.model }

// ---- hình dạng request/response ------------------------------------------

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIJSONSchema struct {
	Name   string         `json:"name"`
	Strict bool           `json:"strict"`
	Schema map[string]any `json:"schema"`
}

type openAIResponseFormat struct {
	Type       string            `json:"type"`
	JSONSchema *openAIJSONSchema `json:"json_schema,omitempty"`
}

type openAIRequest struct {
	Model          string                `json:"model"`
	Messages       []openAIMessage       `json:"messages"`
	ResponseFormat *openAIResponseFormat `json:"response_format,omitempty"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
			// Refusal là cách OpenAI nói "tôi không làm nội dung này" mà vẫn
			// trả HTTP 200 — bỏ qua nó thì lỗi hiện ra dưới dạng "nội dung
			// rỗng" và router đi xoay key vô ích.
			Refusal string `json:"refusal"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// ---- gọi -----------------------------------------------------------------

func (o *OpenAI) Generate(ctx context.Context, promptContent, sourceText string) (string, error) {
	resp, err := o.call(ctx, systemPrompt, userMessage(promptContent, sourceText), nil)
	if err != nil {
		return "", err
	}
	return o.textOf(resp)
}

func (o *OpenAI) GenerateBatch(ctx context.Context, promptContent string, items []string) ([]string, error) {
	resp, err := o.call(ctx, batchSystemPrompt, batchUserMessage(promptContent, items),
		&openAIResponseFormat{
			Type: "json_schema",
			JSONSchema: &openAIJSONSchema{
				Name: "batch_rewrite",
				// strict = OpenAI tự ép đầu ra khớp schema ở tầng decode, chứ
				// không chỉ "cố gắng". Đổi lại schema phải đóng
				// (additionalProperties:false) — xem batchSchema(true).
				Strict: true,
				Schema: batchSchema(true),
			},
		})
	if err != nil {
		return nil, err
	}
	raw, err := o.textOf(resp)
	if err != nil {
		return nil, err
	}
	out, err := parseBatch(raw, len(items))
	if err != nil {
		return nil, domain.LLMFail(domain.LLMFailTransient, fmt.Errorf("openai batch: %w", err))
	}
	return out, nil
}

func (o *OpenAI) call(
	ctx context.Context,
	system, user string,
	format *openAIResponseFormat,
) (openAIResponse, error) {
	var out openAIResponse
	err := postJSON(ctx, openAIURL,
		map[string]string{"Authorization": "Bearer " + o.apiKey},
		openAIRequest{
			Model: o.model,
			Messages: []openAIMessage{
				{Role: "system", Content: system},
				{Role: "user", Content: user},
			},
			ResponseFormat: format,
		},
		&out, openAIClassify)
	return out, err
}

func (o *OpenAI) textOf(resp openAIResponse) (string, error) {
	if len(resp.Choices) == 0 {
		return "", emptyResult(o.Name(), "không có choice nào")
	}
	choice := resp.Choices[0]

	if refusal := strings.TrimSpace(choice.Message.Refusal); refusal != "" {
		return "", domain.LLMFail(domain.LLMFailContent,
			fmt.Errorf("openai từ chối xử lý nội dung: %s", refusal))
	}
	if choice.FinishReason == "content_filter" {
		return "", domain.LLMFail(domain.LLMFailContent,
			fmt.Errorf("openai chặn nội dung (content_filter)"))
	}

	text := strings.TrimSpace(choice.Message.Content)
	if text == "" {
		if choice.FinishReason == "length" {
			return "", domain.LLMFail(domain.LLMFailContent,
				fmt.Errorf("openai cắt đầu ra vì quá dài (finish_reason=length)"))
		}
		return "", emptyResult(o.Name(), "finish_reason="+choice.FinishReason)
	}
	return text, nil
}

// openAIClassify đọc lỗi của OpenAI.
//
// Chỗ dễ nhầm: `insufficient_quota` tới dưới dạng HTTP 429 nhưng KHÔNG tự hồi
// theo thời gian như rate-limit — tài khoản hết tiền thì chờ mấy cũng vậy. Vẫn
// xếp vào quota (key không sai, không nên tắt), chỉ là khoảng nghỉ dài hơn để
// router đừng thử lại mỗi phút.
func openAIClassify(status int, raw []byte, retryAfter time.Duration) error {
	body := parseAPIError(raw)
	msg := body.text(raw)
	kind := kindForStatus(status)

	switch {
	case body.Error.Type == "insufficient_quota":
		kind = domain.LLMFailQuota
		if retryAfter == 0 {
			retryAfter = time.Hour
		}
	case status == http.StatusBadRequest && looksLikeContent(msg):
		kind = domain.LLMFailContent
	case status == http.StatusBadRequest && looksLikeAuth(msg):
		kind = domain.LLMFailAuth
	case looksLikeQuota(msg):
		kind = domain.LLMFailQuota
	}

	return domain.LLMFailAfter(kind, retryAfter,
		fmt.Errorf("openai HTTP %d: %s", status, msg))
}
