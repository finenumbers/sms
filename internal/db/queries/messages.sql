-- name: InsertSmsMessage :one
INSERT INTO sms_messages (
    client_id, direction, from_msisdn, to_msisdn, text, status, campaign_id, idempotency_key,
    unit_sell_price, billed_segments, tariff_plan_id, tariff_plan_code, currency
) VALUES (
    $1, $2, $3, $4, $5, 'queued', sqlc.narg('campaign_id'), sqlc.narg('idempotency_key'),
    sqlc.narg('unit_sell_price'), sqlc.narg('billed_segments'), sqlc.narg('tariff_plan_id'),
    sqlc.narg('tariff_plan_code'), sqlc.narg('currency')
)
RETURNING *;

-- name: GetSmsMessageByIdempotency :one
SELECT * FROM sms_messages
WHERE client_id = sqlc.arg(client_id)::uuid
  AND idempotency_key = sqlc.arg(idempotency_key);

-- name: GetSmsMessageByID :one
SELECT * FROM sms_messages WHERE id = sqlc.arg(id);

-- name: GetSmsMessageForClient :one
SELECT * FROM sms_messages
WHERE id = sqlc.arg(id) AND client_id = sqlc.arg(client_id)::uuid;

-- name: ListSmsMessagesForClient :many
SELECT * FROM sms_messages
WHERE client_id = sqlc.arg(client_id)::uuid
  AND (sqlc.narg('direction')::sms_direction IS NULL OR direction = sqlc.narg('direction'))
ORDER BY created_at DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: GetSmsMessageByProviderID :one
SELECT * FROM sms_messages
WHERE provider_sms_id = sqlc.arg(provider_sms_id);

-- name: UpdateSmsMessageAccepted :one
UPDATE sms_messages
SET status = 'accepted',
    provider_sms_id = COALESCE(sqlc.narg('provider_sms_id'), provider_sms_id),
    pdu_count = COALESCE(sqlc.narg('pdu_count'), pdu_count),
    accepted_at = COALESCE(accepted_at, now())
WHERE id = sqlc.arg(id)
  AND status IN ('queued', 'accepted')
RETURNING *;

-- name: UpdateSmsMessageFailed :one
UPDATE sms_messages
SET status = 'failed',
    provider_status = sqlc.narg('provider_status'),
    failed_at = COALESCE(failed_at, now())
WHERE id = sqlc.arg(id)
  AND status IN ('queued', 'accepted', 'sent')
RETURNING *;

-- name: UpdateSmsMessageFromStatistic :one
UPDATE sms_messages
SET
    provider_sms_id = COALESCE(sms_messages.provider_sms_id, sqlc.narg('provider_sms_id')),
    provider_status = COALESCE(sqlc.narg('provider_status'), provider_status),
    pdu_count = COALESCE(sqlc.narg('pdu_count'), pdu_count),
    status = CASE
        WHEN sqlc.arg(delivered)::boolean THEN 'delivered'::sms_status
        WHEN sqlc.arg(sent)::boolean THEN 'sent'::sms_status
        ELSE 'accepted'::sms_status
    END,
    accepted_at = COALESCE(accepted_at, now()),
    sent_at = CASE
        WHEN sqlc.arg(sent)::boolean OR sqlc.arg(delivered)::boolean THEN COALESCE(sent_at, now())
        ELSE sent_at
    END,
    delivered_at = CASE
        WHEN sqlc.arg(delivered)::boolean THEN COALESCE(delivered_at, now())
        ELSE delivered_at
    END
WHERE id = sqlc.arg(id)
  AND status IN ('queued', 'accepted', 'sent')
RETURNING *;

-- name: InsertInboundMessage :one
INSERT INTO sms_messages (
    client_id, direction, from_msisdn, to_msisdn, text,
    provider_sms_id, pdu_count, status, accepted_at, delivered_at
) VALUES (
    $1, 'inbound', $2, $3, $4, $5, $6, 'delivered', now(), now()
)
ON CONFLICT (provider_sms_id) WHERE provider_sms_id IS NOT NULL DO NOTHING
RETURNING *;

-- name: ListStaleOutbound :many
SELECT * FROM sms_messages
WHERE direction = 'outbound'
  AND status IN ('accepted', 'sent')
  AND created_at < sqlc.arg(before)
ORDER BY created_at
LIMIT sqlc.arg(page_limit);

-- name: GetAssignedNumberForClient :one
SELECT n.id, n.msisdn, n.status
FROM def_numbers n
JOIN number_assignments a ON a.def_number_id = n.id AND a.unassigned_at IS NULL
WHERE n.msisdn = sqlc.arg(msisdn)
  AND a.client_id = sqlc.arg(client_id)
  AND n.status = 'assigned';

-- name: GetOpenAssignmentByMSISDN :one
SELECT a.id, a.def_number_id, a.client_id, a.assigned_at, n.msisdn
FROM def_numbers n
JOIN number_assignments a ON a.def_number_id = n.id AND a.unassigned_at IS NULL
WHERE n.msisdn = sqlc.arg(msisdn);
