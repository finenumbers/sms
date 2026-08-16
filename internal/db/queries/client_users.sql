-- name: InsertClientUser :one
INSERT INTO client_users (client_id, email, password_hash, name, role, status)
VALUES ($1, $2, $3, $4, 'owner', 'active')
RETURNING *;

-- name: GetClientUserByEmail :one
SELECT * FROM client_users WHERE LOWER(email) = LOWER(sqlc.arg(email)) LIMIT 1;

-- name: GetClientUserByID :one
SELECT * FROM client_users WHERE id = sqlc.arg(id);

-- name: ListClientUsersByClientID :many
SELECT * FROM client_users
WHERE client_id = sqlc.arg(client_id)
ORDER BY created_at;

-- name: GetOwnerByClientID :one
SELECT * FROM client_users
WHERE client_id = sqlc.arg(client_id) AND role = 'owner'
ORDER BY created_at
LIMIT 1;

-- name: CountClientUsersByClientID :one
SELECT count(*)::bigint FROM client_users WHERE client_id = sqlc.arg(client_id);

-- name: CountActiveOwnersByClientID :one
SELECT count(*)::bigint FROM client_users
WHERE client_id = sqlc.arg(client_id)
  AND role = 'owner'
  AND status = 'active';

-- name: UpdateClientUserPassword :exec
UPDATE client_users
SET password_hash = sqlc.arg(password_hash), updated_at = now()
WHERE id = sqlc.arg(id);

-- name: UpdateClientUserStatus :one
UPDATE client_users
SET status = sqlc.arg(status), updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;
