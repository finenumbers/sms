-- name: InsertCallbackEvent :one
INSERT INTO provider_callback_events (
    kind, idempotency_key, method, path, query, headers, raw_body, content_type
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
ON CONFLICT (idempotency_key) DO NOTHING
RETURNING *;

-- name: GetCallbackEventByID :one
SELECT * FROM provider_callback_events WHERE id = sqlc.arg(id);

-- name: ListCallbackEvents :many
SELECT
    id, kind, idempotency_key, method, path, query, content_type,
    octet_length(raw_body) AS body_bytes, created_at, processed_at
FROM provider_callback_events
WHERE (sqlc.narg('kind')::callback_kind IS NULL OR kind = sqlc.narg('kind'))
ORDER BY created_at DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: ListUnprocessedCallbackEvents :many
SELECT *
FROM provider_callback_events
WHERE processed_at IS NULL
ORDER BY created_at
LIMIT sqlc.arg(page_limit);

-- name: MarkCallbackProcessed :execrows
UPDATE provider_callback_events
SET processed_at = now(),
    parsed = sqlc.arg(parsed),
    sms_message_id = sqlc.narg('sms_message_id')
WHERE id = sqlc.arg(id)
  AND processed_at IS NULL;
