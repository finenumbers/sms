-- name: CountAdminUsers :one
SELECT count(*)::bigint AS count FROM admin_users;

-- name: InsertAdminUser :one
INSERT INTO admin_users (email, password_hash, name, status)
VALUES ($1, $2, $3, 'active')
RETURNING *;

-- name: GetAdminUserByEmail :one
SELECT * FROM admin_users WHERE LOWER(email) = LOWER(sqlc.arg(email)) LIMIT 1;

-- name: GetAdminUserByID :one
SELECT * FROM admin_users WHERE id = sqlc.arg(id);
