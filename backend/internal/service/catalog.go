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
		return wrapDB(err, "xoá prompt")
	}
	if rows == 0 {
		return fmt.Errorf("%w: prompt %s", domain.ErrNotFound, id)
	}
	return nil
}
