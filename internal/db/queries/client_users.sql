-- name: InsertClientUser :one
INSERT INTO client_users (client_id, email, password_hash, role, status)
VALUES ($1, $2, $3, 'owner', 'active')
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

-- name: UpdateClientUserPassword :exec
UPDATE client_users
SET password_hash = sqlc.arg(password_hash), updated_at = now()
WHERE id = sqlc.arg(id);
