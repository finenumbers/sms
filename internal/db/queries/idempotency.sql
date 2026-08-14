-- name: InsertIdempotencyKey :one
INSERT INTO idempotency_keys (
    principal_type, principal_id, key, request_hash, expires_at
) VALUES (
    sqlc.arg(principal_type),
    sqlc.arg(principal_id),
    sqlc.arg(key),
    sqlc.arg(request_hash),
    sqlc.arg(expires_at)
)
ON CONFLICT (principal_type, principal_id, key) DO NOTHING
RETURNING *;

-- name: GetIdempotencyKeyForUpdate :one
SELECT * FROM idempotency_keys
WHERE principal_type = sqlc.arg(principal_type)
  AND principal_id = sqlc.arg(principal_id)
  AND key = sqlc.arg(key)
FOR UPDATE;

-- name: CompleteIdempotencyKey :one
UPDATE idempotency_keys
SET response_status = sqlc.arg(response_status),
    response_body = sqlc.arg(response_body)
WHERE id = sqlc.arg(id)
  AND response_status IS NULL
RETURNING *;

-- name: DeleteExpiredIdempotencyKeys :execrows
DELETE FROM idempotency_keys WHERE expires_at < now();
