// Package worker chứa producer (enqueue) và consumer (xử lý) của Asynq.
package worker

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"

	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/worker/task"
)

// Enqueuer là cài đặt domain.Enqueuer trên Asynq. HTTP handler chỉ enqueue,
// không gọi trực tiếp TTS/STT/LLM/Multime (business rule #10).
type Enqueuer struct {
	client *asynq.Client
}

var _ domain.Enqueuer = (*Enqueuer)(nil)

func NewEnqueuer(client *asynq.Client) *Enqueuer { return &Enqueuer{client: client} }

func (e *Enqueuer) EnqueueVoiceProcess(ctx context.Context, sourcePostID, actorID, voiceID string) error {
	t, err := task.NewVoiceProcess(task.VoiceProcessPayload{
		SourcePostID: sourcePostID,
		ActorID:      actorID,
		VoiceID:      voiceID,
	})
	if err != nil {
		return err
	}
	return e.enqueue(ctx, t)
}

func (e *Enqueuer) EnqueueVoiceText(ctx context.Context, voiceID, actorID string) error {
	t, err := task.NewVoiceText(task.VoiceTextPayload{VoiceID: voiceID, ActorID: actorID})
	if err != nil {
		return err
	}
	return e.enqueue(ctx, t)
}

func (e *Enqueuer) EnqueuePostMetadata(ctx context.Context, sourcePostID string) error {
	t, err := task.NewPostMetadata(task.PostMetadataPayload{SourcePostID: sourcePostID})
	if err != nil {
		return err
	}
	return e.enqueue(ctx, t)
}

func (e *Enqueuer) EnqueueVoicePublish(ctx context.Context, voiceID, actorID string) error {
	t, err := task.NewVoicePublish(task.VoicePublishPayload{VoiceID: voiceID, ActorID: actorID})
	if err != nil {
		return err
	}
	return e.enqueue(ctx, t)
}

func (e *Enqueuer) EnqueueBreakingScan(ctx context.Context, listID string) error {
	t, err := task.NewBreakingScan(task.BreakingScanPayload{ListID: listID})
	if err != nil {
		return err
	}
	return e.enqueue(ctx, t)
}

func (e *Enqueuer) enqueue(ctx context.Context, t *asynq.Task) error {
	if _, err := e.client.EnqueueContext(ctx, t); err != nil {
		return fmt.Errorf("enqueue %s: %w", t.Type(), err)
	}
	return nil
}
