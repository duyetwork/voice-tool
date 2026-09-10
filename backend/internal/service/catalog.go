package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/repository"
)

// Catalog quản lý 2 danh mục: Prompt mẫu (A1) và AI Engine (A2).
// Không nằm trong 4 entity bắt buộc audit nên không ghi audit_log.
type Catalog struct {
	q *repository.Queries
}

func NewCatalog(q *repository.Queries) *Catalog { return &Catalog{q: q} }

// ---------------------------------------------------------------------------
// Prompt mẫu
// ---------------------------------------------------------------------------

func (c *Catalog) CreatePrompt(ctx context.Context, actor uuid.UUID, name, content string) (repository.Prompt, error) {
	name, content = strings.TrimSpace(name), strings.TrimSpace(content)
	if name == "" || content == "" {
		return repository.Prompt{}, fmt.Errorf("%w: name và content là bắt buộc", domain.ErrInvalidInput)
	}
	prompt, err := c.q.CreatePrompt(ctx, repository.CreatePromptParams{
		Name: name, Content: content, CreatedBy: actor,
	})
	if err != nil {
		return repository.Prompt{}, fmt.Errorf("tạo prompt: %w", err)
	}
	return prompt, nil
}

func (c *Catalog) GetPrompt(ctx context.Context, id uuid.UUID) (repository.Prompt, error) {
	prompt, err := c.q.GetPrompt(ctx, id)
	if err != nil {
		return repository.Prompt{}, wrapNotFound(err, "prompt "+id.String())
	}
	return prompt, nil
}

func (c *Catalog) ListPrompts(ctx context.Context, limit, offset int32, dirRaw string) ([]repository.Prompt, int64, error) {
	limit, offset = clampPage(limit, offset)
	_, dir := normalizeSort("", dirRaw)
	items, err := c.q.ListPrompts(ctx, repository.ListPromptsParams{Dir: dir, Lim: limit, Off: offset})
	if err != nil {
		return nil, 0, fmt.Errorf("list prompt: %w", err)
	}
	total, err := c.q.CountPrompts(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count prompt: %w", err)
	}
	return items, total, nil
}

func (c *Catalog) UpdatePrompt(ctx context.Context, id uuid.UUID, name, content *string) (repository.Prompt, error) {
	prompt, err := c.q.UpdatePrompt(ctx, repository.UpdatePromptParams{ID: id, Name: name, Content: content})
	if err != nil {
		return repository.Prompt{}, wrapNotFound(err, "prompt "+id.String())
	}
	return prompt, nil
}

func (c *Catalog) DeletePrompt(ctx context.Context, id uuid.UUID) error {
	rows, err := c.q.DeletePrompt(ctx, id)
	if err != nil {
		return fmt.Errorf("xoá prompt: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("%w: prompt %s", domain.ErrNotFound, id)
	}
	return nil
}

// ---------------------------------------------------------------------------
// AI Engine (TTS)
// ---------------------------------------------------------------------------

type AIEngineInput struct {
	Name               string
	Provider           string
	SupportedLanguages []string
	IsActive           *bool
}

func (c *Catalog) CreateAIEngine(ctx context.Context, in AIEngineInput) (repository.AiEngine, error) {
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Provider) == "" {
		return repository.AiEngine{}, fmt.Errorf("%w: name và provider là bắt buộc", domain.ErrInvalidInput)
	}
	langs := in.SupportedLanguages
	if langs == nil {
		langs = []string{}
	}
	engine, err := c.q.CreateAIEngine(ctx, repository.CreateAIEngineParams{
		Name:               strings.TrimSpace(in.Name),
		Provider:           strings.TrimSpace(in.Provider),
		SupportedLanguages: langs,
		IsActive:           boolOr(in.IsActive, true),
	})
	if err != nil {
		return repository.AiEngine{}, fmt.Errorf("tạo ai_engine: %w", err)
	}
	return engine, nil
}

func (c *Catalog) GetAIEngine(ctx context.Context, id uuid.UUID) (repository.AiEngine, error) {
	engine, err := c.q.GetAIEngine(ctx, id)
	if err != nil {
		return repository.AiEngine{}, wrapNotFound(err, "ai_engine "+id.String())
	}
	return engine, nil
}

func (c *Catalog) ListAIEngines(ctx context.Context, onlyActive *bool) ([]repository.AiEngine, error) {
	return c.q.ListAIEngines(ctx, onlyActive)
}

func (c *Catalog) UpdateAIEngine(ctx context.Context, id uuid.UUID, in AIEngineInput) (repository.AiEngine, error) {
	params := repository.UpdateAIEngineParams{
		ID:                 id,
		SupportedLanguages: in.SupportedLanguages,
		IsActive:           in.IsActive,
	}
	if in.Name != "" {
		params.Name = &in.Name
	}
	if in.Provider != "" {
		params.Provider = &in.Provider
	}

	engine, err := c.q.UpdateAIEngine(ctx, params)
	if err != nil {
		return repository.AiEngine{}, wrapNotFound(err, "ai_engine "+id.String())
	}
	return engine, nil
}

func (c *Catalog) DeleteAIEngine(ctx context.Context, id uuid.UUID) error {
	rows, err := c.q.DeleteAIEngine(ctx, id)
	if err != nil {
		return fmt.Errorf("xoá ai_engine: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("%w: ai_engine %s", domain.ErrNotFound, id)
	}
	return nil
}
