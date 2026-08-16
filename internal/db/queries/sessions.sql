-- name: InsertSession :one
INSERT INTO sessions (audience, admin_user_id, client_user_id, expires_at, ip, user_agent)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetSessionByID :one
SELECT * FROM sessions WHERE id = sqlc.arg(id);

-- name: TouchSession :exec
UPDATE sessions
SET last_seen_at = now(), expires_at = sqlc.arg(expires_at)
WHERE id = sqlc.arg(id) AND revoked_at IS NULL AND expires_at > now();

-- name: RevokeSession :exec
UPDATE sessions
SET revoked_at = now()
WHERE id = sqlc.arg(id) AND revoked_at IS NULL;

-- name: RevokeSessionsForClient :exec
UPDATE sessions s
SET revoked_at = now()
FROM client_users u
WHERE s.client_user_id = u.id
  AND u.client_id = sqlc.arg(client_id)
  AND s.revoked_at IS NULL;

-- name: RevokeSessionsForClientUser :exec
UPDATE sessions
SET revoked_at = now()
WHERE client_user_id = sqlc.arg(client_user_id)
  AND revoked_at IS NULL;
