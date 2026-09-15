-- name: RecordAIUsage :exec
INSERT INTO ai_usage (
  kind, provider, model, user_id, input_tokens, output_tokens, characters, audio_seconds, ok
) VALUES (
  $1, $2, $3, sqlc.narg('user_id'), $4, $5, $6, $7, $8
);

-- name: ListAIUsageDaily :many
-- Gộp theo ngày + model: đủ chi tiết để so hai model với nhau, đủ thô để bảng
-- không dài hơn một màn hình sau một tuần chạy thật.
SELECT
  date_trunc('day', at)::date                  AS day,
  kind,
  provider,
  model,
  COUNT(*)::bigint                             AS calls,
  COUNT(*) FILTER (WHERE NOT ok)::bigint       AS failed,
  COALESCE(SUM(input_tokens), 0)::bigint       AS input_tokens,
  COALESCE(SUM(output_tokens), 0)::bigint      AS output_tokens,
  COALESCE(SUM(characters), 0)::bigint         AS characters,
  COALESCE(SUM(audio_seconds), 0)::float8      AS audio_seconds
FROM ai_usage
WHERE at >= now() - make_interval(days => sqlc.arg('days')::int)
GROUP BY 1, 2, 3, 4
ORDER BY 1 DESC, 2, 3, 4;

-- name: DeleteAIUsageBefore :execrows
DELETE FROM ai_usage WHERE at < sqlc.arg('before');
