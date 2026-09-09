-- name: CreateListScheduled :one
INSERT INTO list_scheduled (
  source_url, platform, content_type, collect_mode, prompt_id, scan_frequency,
  language_default, auto_process, auto_publish, status, scan_limit,
  max_posts_per_run, created_by
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
)
RETURNING *;

-- name: GetListScheduled :one
SELECT * FROM list_scheduled WHERE id = $1;

-- name: ListListScheduleds :many
SELECT ls.*, u.email AS created_by_email
FROM list_scheduled ls
JOIN app_user u ON u.id = ls.created_by
WHERE (sqlc.narg('status')::varchar   IS NULL OR ls.status     = sqlc.narg('status'))
  AND (sqlc.narg('platform')::varchar IS NULL OR ls.platform   = sqlc.narg('platform'))
  AND (sqlc.narg('created_by')::uuid  IS NULL OR ls.created_by = sqlc.narg('created_by'))
  AND (sqlc.narg('search')::text      IS NULL OR ls.source_url ~* sqlc.narg('search'))
ORDER BY ls.created_at DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: ListActiveListScheduleds :many
SELECT * FROM list_scheduled WHERE status = 'active' ORDER BY created_at;

-- name: UpdateListScheduled :one
UPDATE list_scheduled
SET source_url        = COALESCE(sqlc.narg('source_url'), source_url),
    platform          = COALESCE(sqlc.narg('platform'), platform),
    content_type      = COALESCE(sqlc.narg('content_type'), content_type),
    collect_mode      = COALESCE(sqlc.narg('collect_mode'), collect_mode),
    prompt_id         = COALESCE(sqlc.narg('prompt_id'), prompt_id),
    scan_frequency    = COALESCE(sqlc.narg('scan_frequency'), scan_frequency),
    language_default  = COALESCE(sqlc.narg('language_default'), language_default),
    auto_process      = COALESCE(sqlc.narg('auto_process'), auto_process),
    auto_publish      = COALESCE(sqlc.narg('auto_publish'), auto_publish),
    status            = COALESCE(sqlc.narg('status'), status),
    scan_limit        = COALESCE(sqlc.narg('scan_limit'), scan_limit),
    max_posts_per_run = COALESCE(sqlc.narg('max_posts_per_run'), max_posts_per_run)
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: SetLastSyncedPostID :exec
UPDATE list_scheduled
SET last_synced_post_id = $2, last_scanned_at = now()
WHERE id = $1;

-- name: TouchListScheduledScanned :exec
UPDATE list_scheduled SET last_scanned_at = now() WHERE id = $1;

-- name: DeleteListScheduled :execrows
DELETE FROM list_scheduled WHERE id = $1;
