package llm

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// geminiBaseURL — endpoint Generative Language API.
const geminiBaseURL = "https://generativelanguage.googleapis.com/v1beta/models"

// Gemini gọi thẳng REST API thay vì qua SDK.
//
// Vì sao không dùng SDK: request ở đây chỉ có 1 hình dạng (1 system + 1 user +
// generationConfig) và nó đã ổn định nhiều phiên bản. Đổi lại, tránh kéo thêm
// một cây phụ thuộc lớn vào một binary mà phần AI chỉ chiếm vài trăm dòng.
type Gemini struct {
	apiKey string
	model  string
}

var _ domain.LLMProvider = (*Gemini)(nil)

func NewGemini(apiKey, model string) *Gemini {
	return &Gemini{apiKey: apiKey, model: model}
}

func (g *Gemini) Name() string { return "gemini:" + g.model }

// ---- hình dạng request/response ------------------------------------------

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiGenerationConfig struct {
	ResponseMIMEType string         `json:"responseMimeType,omitempty"`
	ResponseSchema   map[string]any `json:"responseSchema,omitempty"`
}

type geminiRequest struct {
	SystemInstruction *geminiContent          `json:"system_instruction,omitempty"`
	Contents          []geminiContent         `json:"contents"`
	GenerationConfig  *geminiGenerationConfig `json:"generationConfig,omitempty"`
}

type geminiResponse struct {
	Candidates []struct {
		Content      geminiContent `json:"content"`
		FinishReason string        `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
}

// ---- gọi -----------------------------------------------------------------

func (g *Gemini) Generate(ctx context.Context, promptContent, sourceText string) (string, error) {
	resp, err := g.call(ctx, userMessage(promptContent, sourceText), systemPrompt, nil)
	if err != nil {
		return "", err
	}
	return g.textOf(resp)
}

func (g *Gemini) GenerateBatch(ctx context.Context, promptContent string, items []string) ([]string, error) {
	resp, err := g.call(ctx,
		batchUserMessage(promptContent, items),
		batchSystemPrompt,
		// Structured output NATIVE thay vì chỉ nhắc trong prompt: nhắc trong
		// prompt thì model vẫn có thể trả văn xuôi kèm ```json, và đó là lỗi
		// parse ngẫu nhiên rất khó tái hiện.
		&geminiGenerationConfig{
			ResponseMIMEType: "application/json",
			ResponseSchema:   batchSchema(false),
		})
	if err != nil {
		return nil, err
	}
	raw, err := g.textOf(resp)
	if err != nil {
		return nil, err
	}
	out, err := parseBatch(raw, len(items))
	if err != nil {
		return nil, domain.LLMFail(domain.LLMFailTransient, fmt.Errorf("gemini batch: %w", err))
	}
	return out, nil
}

func (g *Gemini) call(
	ctx context.Context,
	user, system string,
	genConfig *geminiGenerationConfig,
) (geminiResponse, error) {
	var out geminiResponse
	url := fmt.Sprintf("%s/%s:generateContent", geminiBaseURL, g.model)
	err := postJSON(ctx, url,
		// Key đi qua header chứ không qua query string: query string lọt vào
		// access log của mọi proxy trên đường đi.
		map[string]string{"x-goog-api-key": g.apiKey},
		geminiRequest{
			SystemInstruction: &geminiContent{Parts: []geminiPart{{Text: system}}},
			Contents:          []geminiContent{{Role: "user", Parts: []geminiPart{{Text: user}}}},
			GenerationConfig:  genConfig,
		},
		&out, geminiClassify)
	return out, err
}

func (g *Gemini) textOf(resp geminiResponse) (string, error) {
	// Prompt bị chặn ngay từ đầu: không có candidate nào, lý do nằm ở
	// promptFeedback. Đây là lỗi NỘI DUNG — xoay key không đổi được gì.
	if reason := resp.PromptFeedback.BlockReason; reason != "" {
		return "", domain.LLMFail(domain.LLMFailContent,
			fmt.Errorf("gemini chặn prompt (%s)", reason))
	}
	if len(resp.Candidates) == 0 {
		return "", emptyResult(g.Name(), "không có candidate nào")
	}

	c := resp.Candidates[0]
	switch c.FinishReason {
	case "SAFETY", "PROHIBITED_CONTENT", "BLOCKLIST", "SPII":
		return "", domain.LLMFail(domain.LLMFailContent,
			fmt.Errorf("gemini từ chối nội dung (%s)", c.FinishReason))
	}

	var b strings.Builder
	for _, p := range c.Content.Parts {
		b.WriteString(p.Text)
	}
	text := strings.TrimSpace(b.String())
	if text == "" {
		// MAX_TOKENS ở đây nghĩa là bài quá dài so với trần đầu ra -> đổi key
		// cũng vậy thôi, chỉ model khác mới có cửa.
		if c.FinishReason == "MAX_TOKENS" {
			return "", domain.LLMFail(domain.LLMFailContent,
				fmt.Errorf("gemini cắt đầu ra vì quá dài (MAX_TOKENS)"))
		}
		return "", emptyResult(g.Name(), "finish_reason="+c.FinishReason)
	}
	return text, nil
}

// geminiClassify đọc lỗi của Gemini.
//
// Gemini dùng 400 cho cả "key sai" (API_KEY_INVALID) lẫn "input quá dài", và
// 429 cho cả rate-limit phút lẫn hạn mức ngày — nên phải đọc body, không thể
// chỉ nhìn status.
func geminiClassify(status int, raw []byte, retryAfter time.Duration) error {
	body := parseAPIError(raw)
	msg := body.text(raw)
	kind := kindForStatus(status)

	switch {
	case status == http.StatusBadRequest && looksLikeAuth(msg):
		kind = domain.LLMFailAuth
	case status == http.StatusBadRequest && looksLikeContent(msg):
		kind = domain.LLMFailContent
	case looksLikeQuota(msg) || body.Error.Status == "RESOURCE_EXHAUSTED":
		kind = domain.LLMFailQuota
	}

	return domain.LLMFailAfter(kind, retryAfter,
		fmt.Errorf("gemini HTTP %d: %s", status, msg))
}
