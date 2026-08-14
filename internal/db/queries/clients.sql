-- name: InsertClient :one
INSERT INTO clients (name, status)
VALUES ($1, 'active')
RETURNING *;

-- name: GetClientByID :one
SELECT * FROM clients WHERE id = sqlc.arg(id);

-- name: GetClientByIDForUpdate :one
SELECT * FROM clients WHERE id = sqlc.arg(id) FOR UPDATE;

-- name: ListClients :many
SELECT
    c.id,
    c.name,
    c.status,
    c.created_at,
    c.updated_at,
    COALESCE(ou.email, '') AS owner_email,
    (
        SELECT count(*)::bigint
        FROM number_assignments a
        WHERE a.client_id = c.id AND a.unassigned_at IS NULL
    ) AS assigned_count,
    w.available_balance,
    w.held_balance,
    w.currency AS wallet_currency
FROM clients c
LEFT JOIN wallets w ON w.client_id = c.id
LEFT JOIN LATERAL (
    SELECT email
    FROM client_users
    WHERE client_id = c.id AND role = 'owner'
    ORDER BY created_at
    LIMIT 1
) ou ON true
WHERE c.status <> 'deleted'
  AND (sqlc.narg('status')::client_status IS NULL OR c.status = sqlc.narg('status'))
ORDER BY c.created_at DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: UpdateClientName :one
UPDATE clients
SET name = sqlc.arg(name), updated_at = now()
WHERE id = sqlc.arg(id) AND status <> 'deleted'
RETURNING *;

-- name: SetClientStatus :one
UPDATE clients
SET status = sqlc.arg(status), updated_at = now()
WHERE id = sqlc.arg(id) AND status <> 'deleted'
RETURNING *;
