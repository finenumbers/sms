-- name: InsertOpsEvent :exec
INSERT INTO ops_events (
    category, level, request_id, actor_type, actor_id, client_id,
    action, resource_type, resource_id,
    http_method, http_path, http_status, latency_ms,
    summary, detail, error, ip
) VALUES (
    sqlc.arg(category),
    sqlc.arg(level),
    sqlc.narg('request_id'),
    sqlc.narg('actor_type'),
    sqlc.narg('actor_id'),
    sqlc.narg('client_id'),
    sqlc.arg(action),
    sqlc.narg('resource_type'),
    sqlc.narg('resource_id'),
    sqlc.narg('http_method'),
    sqlc.narg('http_path'),
    sqlc.narg('http_status'),
    sqlc.narg('latency_ms'),
    sqlc.narg('summary'),
    sqlc.arg(detail),
    sqlc.narg('error'),
    sqlc.narg('ip')
);

-- name: GetOpsEventByID :one
SELECT * FROM ops_events WHERE id = sqlc.arg(id);

-- name: ListOpsEvents :many
SELECT
    id, created_at, category, level, request_id,
    actor_type, actor_id, client_id, action,
    resource_type, resource_id,
    http_method, http_path, http_status, latency_ms,
    summary, error, ip
FROM ops_events
WHERE created_at >= sqlc.arg(from_ts)
  AND created_at < sqlc.arg(to_ts)
  AND (sqlc.narg('category')::text IS NULL OR category = sqlc.narg('category'))
  AND (sqlc.narg('level')::text IS NULL OR level = sqlc.narg('level'))
  AND (sqlc.narg('request_id')::text IS NULL OR request_id = sqlc.narg('request_id'))
  AND (sqlc.narg('client_id')::uuid IS NULL OR client_id = sqlc.narg('client_id'))
  AND (
        sqlc.narg('q')::text IS NULL
        OR summary ILIKE sqlc.narg('q')
        OR action ILIKE sqlc.narg('q')
        OR COALESCE(error, '') ILIKE sqlc.narg('q')
      )
ORDER BY created_at DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);
