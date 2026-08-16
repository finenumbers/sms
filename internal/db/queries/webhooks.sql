-- name: InsertWebhookEndpoint :one
INSERT INTO webhook_endpoints (
    client_id,
    url,
    secret_ciphertext,
    dek_key_id,
    description,
    enabled,
    events
) VALUES (
    sqlc.arg(client_id),
    sqlc.arg(url),
    sqlc.arg(secret_ciphertext),
    sqlc.arg(dek_key_id),
    sqlc.narg(description),
    sqlc.arg(enabled),
    sqlc.arg(events)
)
RETURNING *;

-- name: GetWebhookEndpointForClient :one
SELECT *
FROM webhook_endpoints
WHERE id = sqlc.arg(id)
  AND client_id = sqlc.arg(client_id);

-- name: GetWebhookEndpoint :one
SELECT *
FROM webhook_endpoints
WHERE id = sqlc.arg(id);

-- name: ListWebhookEndpointsForClient :many
SELECT *
FROM webhook_endpoints
WHERE client_id = sqlc.arg(client_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountWebhookEndpointsForClient :one
SELECT count(*)::bigint AS n
FROM webhook_endpoints
WHERE client_id = sqlc.arg(client_id);

-- name: ListEnabledWebhookEndpoints :many
SELECT *
FROM webhook_endpoints
WHERE client_id = sqlc.arg(client_id)
  AND enabled = true;

-- name: UpdateWebhookEndpoint :one
UPDATE webhook_endpoints
SET url = COALESCE(sqlc.narg(url), url),
    description = COALESCE(sqlc.narg(description), description),
    enabled = COALESCE(sqlc.narg(enabled), enabled),
    events = COALESCE(sqlc.narg(events), events),
    consecutive_failures = COALESCE(sqlc.narg(consecutive_failures), consecutive_failures),
    secret_ciphertext = COALESCE(sqlc.narg(secret_ciphertext), secret_ciphertext),
    dek_key_id = COALESCE(sqlc.narg(dek_key_id), dek_key_id),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND client_id = sqlc.arg(client_id)
RETURNING *;

-- name: DeleteWebhookEndpoint :execrows
DELETE FROM webhook_endpoints
WHERE id = sqlc.arg(id)
  AND client_id = sqlc.arg(client_id);

-- name: InsertWebhookDelivery :one
INSERT INTO webhook_deliveries (
    id,
    client_id,
    endpoint_id,
    job_id,
    job_item_id,
    event_type,
    payload,
    status,
    max_attempts,
    next_attempt_at
) VALUES (
    sqlc.arg(id),
    sqlc.arg(client_id),
    sqlc.arg(endpoint_id),
    sqlc.narg(job_id),
    sqlc.narg(job_item_id),
    sqlc.arg(event_type),
    sqlc.arg(payload),
    sqlc.arg(status),
    sqlc.arg(max_attempts),
    sqlc.arg(next_attempt_at)
)
RETURNING *;

-- name: ListWebhookDeliveriesForClient :many
SELECT *
FROM webhook_deliveries
WHERE client_id = sqlc.arg(client_id)
  AND (sqlc.narg(endpoint_id)::uuid IS NULL OR endpoint_id = sqlc.narg(endpoint_id))
  AND (sqlc.narg(filter_status)::webhook_delivery_status IS NULL OR status = sqlc.narg(filter_status))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountWebhookDeliveriesForClient :one
SELECT count(*)::bigint AS n
FROM webhook_deliveries
WHERE client_id = sqlc.arg(client_id)
  AND (sqlc.narg(endpoint_id)::uuid IS NULL OR endpoint_id = sqlc.narg(endpoint_id))
  AND (sqlc.narg(filter_status)::webhook_delivery_status IS NULL OR status = sqlc.narg(filter_status));

-- name: ClaimWebhookDeliveries :many
UPDATE webhook_deliveries AS d
SET next_attempt_at = now() + interval '120 seconds',
    updated_at = now()
WHERE d.id IN (
    SELECT d.id
    FROM webhook_deliveries d
    JOIN clients c ON c.id = d.client_id AND c.status = 'active'
    WHERE d.status IN ('pending', 'failed')
      AND d.next_attempt_at IS NOT NULL
      AND d.next_attempt_at <= now()
    ORDER BY d.next_attempt_at, d.id
    FOR UPDATE OF d SKIP LOCKED
    LIMIT sqlc.arg(page_limit)
)
RETURNING *;

-- name: MarkWebhookDeliveryDelivered :one
UPDATE webhook_deliveries
SET status = 'delivered',
    attempt_count = sqlc.arg(attempt_count),
    last_response_code = sqlc.narg(last_response_code),
    last_error = NULL,
    delivered_at = now(),
    next_attempt_at = NULL,
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: MarkWebhookDeliveryAttempt :one
UPDATE webhook_deliveries
SET status = sqlc.arg(status),
    attempt_count = sqlc.arg(attempt_count),
    last_response_code = sqlc.narg(last_response_code),
    last_error = sqlc.narg(last_error),
    next_attempt_at = sqlc.narg(next_attempt_at),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: IncrementWebhookEndpointFailures :one
UPDATE webhook_endpoints
SET consecutive_failures = consecutive_failures + 1,
    enabled = CASE
        WHEN consecutive_failures + 1 >= sqlc.arg(disable_after) THEN false
        ELSE enabled
    END,
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: ResetWebhookEndpointFailures :exec
UPDATE webhook_endpoints
SET consecutive_failures = 0,
    updated_at = now()
WHERE id = sqlc.arg(id);

-- name: CountWebhookDeliveriesSince :one
SELECT count(*)::bigint AS n
FROM webhook_deliveries
WHERE created_at >= sqlc.arg(since);
