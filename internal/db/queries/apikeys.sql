-- name: InsertAPICredential :one
INSERT INTO api_credentials (
    client_id, name, key_prefix, secret_hash, scopes, allowed_cidrs, created_by
) VALUES (
    sqlc.arg(client_id),
    sqlc.arg(name),
    sqlc.arg(key_prefix),
    sqlc.arg(secret_hash),
    sqlc.arg(scopes),
    COALESCE(sqlc.arg(allowed_cidrs)::text[]::inet[], ARRAY[]::inet[]),
    sqlc.arg(created_by)
)
RETURNING
    id, client_id, name, key_prefix, secret_hash, scopes, status,
    allowed_cidrs::text[] AS allowed_cidrs,
    last_used_at, created_by, created_at;

-- name: GetAPICredentialByPrefix :one
SELECT
    id, client_id, name, key_prefix, secret_hash, scopes, status,
    allowed_cidrs::text[] AS allowed_cidrs,
    last_used_at, created_by, created_at
FROM api_credentials
WHERE key_prefix = sqlc.arg(key_prefix);

-- name: GetAPICredentialByIDForClient :one
SELECT
    id, client_id, name, key_prefix, secret_hash, scopes, status,
    allowed_cidrs::text[] AS allowed_cidrs,
    last_used_at, created_by, created_at
FROM api_credentials
WHERE id = sqlc.arg(id) AND client_id = sqlc.arg(client_id);

-- name: ListAPICredentialsForClient :many
SELECT
    id, client_id, name, key_prefix, secret_hash, scopes, status,
    allowed_cidrs::text[] AS allowed_cidrs,
    last_used_at, created_by, created_at
FROM api_credentials
WHERE client_id = sqlc.arg(client_id)
ORDER BY created_at DESC;

-- name: RevokeAPICredential :one
UPDATE api_credentials
SET status = 'revoked'
WHERE id = sqlc.arg(id)
  AND client_id = sqlc.arg(client_id)
  AND status = 'active'
RETURNING
    id, client_id, name, key_prefix, secret_hash, scopes, status,
    allowed_cidrs::text[] AS allowed_cidrs,
    last_used_at, created_by, created_at;

-- name: RevokeAPICredentialsForClient :exec
UPDATE api_credentials
SET status = 'revoked'
WHERE client_id = sqlc.arg(client_id) AND status = 'active';

-- name: TouchAPICredential :exec
UPDATE api_credentials
SET last_used_at = now()
WHERE id = sqlc.arg(id)
  AND status = 'active'
  AND (last_used_at IS NULL OR last_used_at < now() - interval '1 minute');
