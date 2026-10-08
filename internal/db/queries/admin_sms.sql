-- Admin read models. Each list query keeps the index-driving predicate as a
-- plain equality or a literal partial-index predicate. Do not fold these into
-- one statement with OR-null filters on those columns.

-- name: AdminListOutboundMessages :many
SELECT
    m.id,
    m.client_id,
    cl.name AS client_name,
    m.direction,
    m.from_msisdn,
    m.to_msisdn,
    (CASE WHEN char_length(m.text) > 80 THEN left(m.text, 80) || '…' ELSE m.text END)::text AS text_preview,
    m.status,
    m.provider_status,
    m.pdu_count,
    m.billed_segments,
    m.unit_sell_price,
    m.currency,
    m.billing_action,
    m.campaign_id,
    m.created_at
FROM sms_messages m
LEFT JOIN clients cl ON cl.id = m.client_id
WHERE cl.status IS DISTINCT FROM 'deleted'
  AND m.direction = 'outbound'
ORDER BY m.created_at DESC, m.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);


-- name: AdminListInboundMessages :many
SELECT
    m.id,
    m.client_id,
    cl.name AS client_name,
    m.direction,
    m.from_msisdn,
    m.to_msisdn,
    (CASE WHEN char_length(m.text) > 80 THEN left(m.text, 80) || '…' ELSE m.text END)::text AS text_preview,
    m.status,
    m.provider_status,
    m.pdu_count,
    m.billed_segments,
    m.unit_sell_price,
    m.currency,
    m.billing_action,
    m.campaign_id,
    m.created_at
FROM sms_messages m
LEFT JOIN clients cl ON cl.id = m.client_id
WHERE cl.status IS DISTINCT FROM 'deleted'
  AND m.direction = 'inbound'
ORDER BY m.created_at DESC, m.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);


-- name: AdminListInboundMessagesByStatus :many
SELECT
    m.id,
    m.client_id,
    cl.name AS client_name,
    m.direction,
    m.from_msisdn,
    m.to_msisdn,
    (CASE WHEN char_length(m.text) > 80 THEN left(m.text, 80) || '…' ELSE m.text END)::text AS text_preview,
    m.status,
    m.provider_status,
    m.pdu_count,
    m.billed_segments,
    m.unit_sell_price,
    m.currency,
    m.billing_action,
    m.campaign_id,
    m.created_at
FROM sms_messages m
LEFT JOIN clients cl ON cl.id = m.client_id
WHERE cl.status IS DISTINCT FROM 'deleted'
  AND m.direction = 'inbound'
  AND m.status = sqlc.arg(status)::sms_status
ORDER BY m.created_at DESC, m.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);


-- name: AdminListOutboundInflight :many
SELECT
    m.id,
    m.client_id,
    cl.name AS client_name,
    m.direction,
    m.from_msisdn,
    m.to_msisdn,
    (CASE WHEN char_length(m.text) > 80 THEN left(m.text, 80) || '…' ELSE m.text END)::text AS text_preview,
    m.status,
    m.provider_status,
    m.pdu_count,
    m.billed_segments,
    m.unit_sell_price,
    m.currency,
    m.billing_action,
    m.campaign_id,
    m.created_at
FROM sms_messages m
LEFT JOIN clients cl ON cl.id = m.client_id
WHERE cl.status IS DISTINCT FROM 'deleted'
  AND m.direction = 'outbound'
  AND m.status IN ('queued', 'accepted', 'sent')
  AND m.status = sqlc.arg(status)::sms_status
ORDER BY m.created_at DESC, m.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);


-- name: AdminListFailedMessages :many
SELECT
    m.id,
    m.client_id,
    cl.name AS client_name,
    m.direction,
    m.from_msisdn,
    m.to_msisdn,
    (CASE WHEN char_length(m.text) > 80 THEN left(m.text, 80) || '…' ELSE m.text END)::text AS text_preview,
    m.status,
    m.provider_status,
    m.pdu_count,
    m.billed_segments,
    m.unit_sell_price,
    m.currency,
    m.billing_action,
    m.campaign_id,
    m.created_at
FROM sms_messages m
LEFT JOIN clients cl ON cl.id = m.client_id
WHERE cl.status IS DISTINCT FROM 'deleted'
  AND m.status = 'failed'
  AND m.direction = sqlc.arg(direction)::sms_direction
ORDER BY m.created_at DESC, m.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);


-- name: AdminListDeliveredMessages :many
SELECT
    m.id,
    m.client_id,
    cl.name AS client_name,
    m.direction,
    m.from_msisdn,
    m.to_msisdn,
    (CASE WHEN char_length(m.text) > 80 THEN left(m.text, 80) || '…' ELSE m.text END)::text AS text_preview,
    m.status,
    m.provider_status,
    m.pdu_count,
    m.billed_segments,
    m.unit_sell_price,
    m.currency,
    m.billing_action,
    m.campaign_id,
    m.created_at
FROM sms_messages m
LEFT JOIN clients cl ON cl.id = m.client_id
WHERE cl.status IS DISTINCT FROM 'deleted'
  AND m.status = 'delivered'
  AND m.direction = sqlc.arg(direction)::sms_direction
ORDER BY m.created_at DESC, m.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);


-- name: AdminListDirectOutbound :many
SELECT
    m.id,
    m.client_id,
    cl.name AS client_name,
    m.direction,
    m.from_msisdn,
    m.to_msisdn,
    (CASE WHEN char_length(m.text) > 80 THEN left(m.text, 80) || '…' ELSE m.text END)::text AS text_preview,
    m.status,
    m.provider_status,
    m.pdu_count,
    m.billed_segments,
    m.unit_sell_price,
    m.currency,
    m.billing_action,
    m.campaign_id,
    m.created_at
FROM sms_messages m
LEFT JOIN clients cl ON cl.id = m.client_id
WHERE cl.status IS DISTINCT FROM 'deleted'
  AND m.direction = 'outbound'
  AND m.campaign_id IS NULL
ORDER BY m.created_at DESC, m.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);


-- name: AdminListDirectOutboundByStatus :many
SELECT
    m.id,
    m.client_id,
    cl.name AS client_name,
    m.direction,
    m.from_msisdn,
    m.to_msisdn,
    (CASE WHEN char_length(m.text) > 80 THEN left(m.text, 80) || '…' ELSE m.text END)::text AS text_preview,
    m.status,
    m.provider_status,
    m.pdu_count,
    m.billed_segments,
    m.unit_sell_price,
    m.currency,
    m.billing_action,
    m.campaign_id,
    m.created_at
FROM sms_messages m
LEFT JOIN clients cl ON cl.id = m.client_id
WHERE cl.status IS DISTINCT FROM 'deleted'
  AND m.direction = 'outbound'
  AND m.campaign_id IS NULL
  AND m.status = sqlc.arg(status)::sms_status
ORDER BY m.created_at DESC, m.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);


-- name: AdminListCampaignOutbound :many
SELECT
    m.id,
    m.client_id,
    cl.name AS client_name,
    m.direction,
    m.from_msisdn,
    m.to_msisdn,
    (CASE WHEN char_length(m.text) > 80 THEN left(m.text, 80) || '…' ELSE m.text END)::text AS text_preview,
    m.status,
    m.provider_status,
    m.pdu_count,
    m.billed_segments,
    m.unit_sell_price,
    m.currency,
    m.billing_action,
    m.campaign_id,
    m.created_at
FROM sms_messages m
LEFT JOIN clients cl ON cl.id = m.client_id
WHERE cl.status IS DISTINCT FROM 'deleted'
  AND m.direction = 'outbound'
  AND m.campaign_id IS NOT NULL
ORDER BY m.created_at DESC, m.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);


-- name: AdminListCampaignOutboundInflight :many
SELECT
    m.id,
    m.client_id,
    cl.name AS client_name,
    m.direction,
    m.from_msisdn,
    m.to_msisdn,
    (CASE WHEN char_length(m.text) > 80 THEN left(m.text, 80) || '…' ELSE m.text END)::text AS text_preview,
    m.status,
    m.provider_status,
    m.pdu_count,
    m.billed_segments,
    m.unit_sell_price,
    m.currency,
    m.billing_action,
    m.campaign_id,
    m.created_at
FROM sms_messages m
LEFT JOIN clients cl ON cl.id = m.client_id
WHERE cl.status IS DISTINCT FROM 'deleted'
  AND m.direction = 'outbound'
  AND m.campaign_id IS NOT NULL
  AND m.status IN ('queued', 'accepted', 'sent')
  AND m.status = sqlc.arg(status)::sms_status
ORDER BY m.created_at DESC, m.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);


-- name: AdminListCampaignOutboundFailed :many
SELECT
    m.id,
    m.client_id,
    cl.name AS client_name,
    m.direction,
    m.from_msisdn,
    m.to_msisdn,
    (CASE WHEN char_length(m.text) > 80 THEN left(m.text, 80) || '…' ELSE m.text END)::text AS text_preview,
    m.status,
    m.provider_status,
    m.pdu_count,
    m.billed_segments,
    m.unit_sell_price,
    m.currency,
    m.billing_action,
    m.campaign_id,
    m.created_at
FROM sms_messages m
LEFT JOIN clients cl ON cl.id = m.client_id
WHERE cl.status IS DISTINCT FROM 'deleted'
  AND m.direction = 'outbound'
  AND m.campaign_id IS NOT NULL
  AND m.status = 'failed'
ORDER BY m.created_at DESC, m.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);


-- name: AdminListCampaignOutboundByStatus :many
SELECT
    m.id,
    m.client_id,
    cl.name AS client_name,
    m.direction,
    m.from_msisdn,
    m.to_msisdn,
    (CASE WHEN char_length(m.text) > 80 THEN left(m.text, 80) || '…' ELSE m.text END)::text AS text_preview,
    m.status,
    m.provider_status,
    m.pdu_count,
    m.billed_segments,
    m.unit_sell_price,
    m.currency,
    m.billing_action,
    m.campaign_id,
    m.created_at
FROM sms_messages m
LEFT JOIN clients cl ON cl.id = m.client_id
WHERE cl.status IS DISTINCT FROM 'deleted'
  AND m.direction = 'outbound'
  AND m.campaign_id IS NOT NULL
  AND m.status = sqlc.arg(status)::sms_status
ORDER BY m.created_at DESC, m.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);


-- name: AdminListClientMessages :many
SELECT
    m.id,
    m.client_id,
    cl.name AS client_name,
    m.direction,
    m.from_msisdn,
    m.to_msisdn,
    (CASE WHEN char_length(m.text) > 80 THEN left(m.text, 80) || '…' ELSE m.text END)::text AS text_preview,
    m.status,
    m.provider_status,
    m.pdu_count,
    m.billed_segments,
    m.unit_sell_price,
    m.currency,
    m.billing_action,
    m.campaign_id,
    m.created_at
FROM sms_messages m
LEFT JOIN clients cl ON cl.id = m.client_id
WHERE cl.status IS DISTINCT FROM 'deleted'
  AND m.client_id = sqlc.arg(client_id)
  AND m.direction = sqlc.arg(direction)::sms_direction
ORDER BY m.created_at DESC, m.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);


-- name: AdminListClientMessagesByStatus :many
SELECT
    m.id,
    m.client_id,
    cl.name AS client_name,
    m.direction,
    m.from_msisdn,
    m.to_msisdn,
    (CASE WHEN char_length(m.text) > 80 THEN left(m.text, 80) || '…' ELSE m.text END)::text AS text_preview,
    m.status,
    m.provider_status,
    m.pdu_count,
    m.billed_segments,
    m.unit_sell_price,
    m.currency,
    m.billing_action,
    m.campaign_id,
    m.created_at
FROM sms_messages m
LEFT JOIN clients cl ON cl.id = m.client_id
WHERE cl.status IS DISTINCT FROM 'deleted'
  AND m.client_id = sqlc.arg(client_id)
  AND m.direction = sqlc.arg(direction)::sms_direction
  AND m.status = sqlc.arg(status)::sms_status
ORDER BY m.created_at DESC, m.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);


-- name: AdminListClientDirect :many
SELECT
    m.id,
    m.client_id,
    cl.name AS client_name,
    m.direction,
    m.from_msisdn,
    m.to_msisdn,
    (CASE WHEN char_length(m.text) > 80 THEN left(m.text, 80) || '…' ELSE m.text END)::text AS text_preview,
    m.status,
    m.provider_status,
    m.pdu_count,
    m.billed_segments,
    m.unit_sell_price,
    m.currency,
    m.billing_action,
    m.campaign_id,
    m.created_at
FROM sms_messages m
LEFT JOIN clients cl ON cl.id = m.client_id
WHERE cl.status IS DISTINCT FROM 'deleted'
  AND m.client_id = sqlc.arg(client_id)
  AND m.direction = 'outbound'
  AND m.campaign_id IS NULL
ORDER BY m.created_at DESC, m.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);


-- name: AdminListClientDirectByStatus :many
SELECT
    m.id,
    m.client_id,
    cl.name AS client_name,
    m.direction,
    m.from_msisdn,
    m.to_msisdn,
    (CASE WHEN char_length(m.text) > 80 THEN left(m.text, 80) || '…' ELSE m.text END)::text AS text_preview,
    m.status,
    m.provider_status,
    m.pdu_count,
    m.billed_segments,
    m.unit_sell_price,
    m.currency,
    m.billing_action,
    m.campaign_id,
    m.created_at
FROM sms_messages m
LEFT JOIN clients cl ON cl.id = m.client_id
WHERE cl.status IS DISTINCT FROM 'deleted'
  AND m.client_id = sqlc.arg(client_id)
  AND m.direction = 'outbound'
  AND m.campaign_id IS NULL
  AND m.status = sqlc.arg(status)::sms_status
ORDER BY m.created_at DESC, m.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);


-- name: AdminListClientCampaign :many
SELECT
    m.id,
    m.client_id,
    cl.name AS client_name,
    m.direction,
    m.from_msisdn,
    m.to_msisdn,
    (CASE WHEN char_length(m.text) > 80 THEN left(m.text, 80) || '…' ELSE m.text END)::text AS text_preview,
    m.status,
    m.provider_status,
    m.pdu_count,
    m.billed_segments,
    m.unit_sell_price,
    m.currency,
    m.billing_action,
    m.campaign_id,
    m.created_at
FROM sms_messages m
LEFT JOIN clients cl ON cl.id = m.client_id
WHERE cl.status IS DISTINCT FROM 'deleted'
  AND m.client_id = sqlc.arg(client_id)
  AND m.direction = 'outbound'
  AND m.campaign_id IS NOT NULL
ORDER BY m.created_at DESC, m.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);


-- name: AdminListClientCampaignByStatus :many
SELECT
    m.id,
    m.client_id,
    cl.name AS client_name,
    m.direction,
    m.from_msisdn,
    m.to_msisdn,
    (CASE WHEN char_length(m.text) > 80 THEN left(m.text, 80) || '…' ELSE m.text END)::text AS text_preview,
    m.status,
    m.provider_status,
    m.pdu_count,
    m.billed_segments,
    m.unit_sell_price,
    m.currency,
    m.billing_action,
    m.campaign_id,
    m.created_at
FROM sms_messages m
LEFT JOIN clients cl ON cl.id = m.client_id
WHERE cl.status IS DISTINCT FROM 'deleted'
  AND m.client_id = sqlc.arg(client_id)
  AND m.direction = 'outbound'
  AND m.campaign_id IS NOT NULL
  AND m.status = sqlc.arg(status)::sms_status
ORDER BY m.created_at DESC, m.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);


-- name: AdminListMessagesByMSISDN :many
SELECT
    id,
    client_id,
    client_name,
    direction,
    from_msisdn,
    to_msisdn,
    text_preview,
    status,
    provider_status,
    pdu_count,
    billed_segments,
    unit_sell_price,
    currency,
    billing_action,
    campaign_id,
    created_at
FROM (
    SELECT
    m.id,
    m.client_id,
    cl.name AS client_name,
    m.direction,
    m.from_msisdn,
    m.to_msisdn,
    (CASE WHEN char_length(m.text) > 80 THEN left(m.text, 80) || '…' ELSE m.text END)::text AS text_preview,
    m.status,
    m.provider_status,
    m.pdu_count,
    m.billed_segments,
    m.unit_sell_price,
    m.currency,
    m.billing_action,
    m.campaign_id,
    m.created_at
FROM sms_messages m
LEFT JOIN clients cl ON cl.id = m.client_id
    WHERE m.from_msisdn = sqlc.arg(msisdn)
      AND m.direction = sqlc.arg(direction)::sms_direction
      AND cl.status IS DISTINCT FROM 'deleted'
      AND (sqlc.narg(client_id)::uuid IS NULL OR m.client_id = sqlc.narg(client_id))
      AND (sqlc.narg(status)::sms_status IS NULL OR m.status = sqlc.narg(status))
      AND (
        sqlc.narg(source)::text IS NULL
        OR (sqlc.narg(source)::text = 'direct' AND m.campaign_id IS NULL)
        OR (sqlc.narg(source)::text = 'campaign' AND m.campaign_id IS NOT NULL)
      )
    UNION ALL
    SELECT
    m.id,
    m.client_id,
    cl.name AS client_name,
    m.direction,
    m.from_msisdn,
    m.to_msisdn,
    (CASE WHEN char_length(m.text) > 80 THEN left(m.text, 80) || '…' ELSE m.text END)::text AS text_preview,
    m.status,
    m.provider_status,
    m.pdu_count,
    m.billed_segments,
    m.unit_sell_price,
    m.currency,
    m.billing_action,
    m.campaign_id,
    m.created_at
FROM sms_messages m
LEFT JOIN clients cl ON cl.id = m.client_id
    WHERE m.to_msisdn = sqlc.arg(msisdn)
      AND m.direction = sqlc.arg(direction)::sms_direction
      AND cl.status IS DISTINCT FROM 'deleted'
      AND (sqlc.narg(client_id)::uuid IS NULL OR m.client_id = sqlc.narg(client_id))
      AND (sqlc.narg(status)::sms_status IS NULL OR m.status = sqlc.narg(status))
      AND (
        sqlc.narg(source)::text IS NULL
        OR (sqlc.narg(source)::text = 'direct' AND m.campaign_id IS NULL)
        OR (sqlc.narg(source)::text = 'campaign' AND m.campaign_id IS NOT NULL)
      )
      AND m.from_msisdn <> sqlc.arg(msisdn)
) AS matched
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);


-- name: AdminListCampaigns :many
SELECT
    c.id,
    c.client_id,
    cl.name AS client_name,
    c.from_msisdn,
    (CASE WHEN char_length(c.text) > 80 THEN left(c.text, 80) || '…' ELSE c.text END)::text AS text_preview,
    c.status,
    c.total_count,
    c.accepted_count,
    c.delivered_count,
    c.failed_count,
    c.created_at
FROM sms_campaigns c
JOIN clients cl ON cl.id = c.client_id AND cl.status <> 'deleted'
WHERE (sqlc.narg(client_id)::uuid IS NULL OR c.client_id = sqlc.narg(client_id))
  AND (sqlc.narg(status)::campaign_status IS NULL OR c.status = sqlc.narg(status))
  AND (sqlc.narg(from_msisdn)::text IS NULL OR c.from_msisdn = sqlc.narg(from_msisdn))
ORDER BY c.created_at DESC, c.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: AdminGetCampaign :one
SELECT
    c.id,
    c.client_id,
    cl.name AS client_name,
    c.from_msisdn,
    c.text,
    c.status,
    c.total_count,
    c.accepted_count,
    c.delivered_count,
    c.failed_count,
    c.created_by,
    cu.email AS created_by_email,
    c.created_at,
    c.updated_at
FROM sms_campaigns c
LEFT JOIN clients cl ON cl.id = c.client_id
LEFT JOIN client_users cu ON cu.id = c.created_by
WHERE c.id = sqlc.arg(id);

-- name: AdminCampaignBilling :one
SELECT
    COALESCE(SUM(unit_sell_price * billed_segments) FILTER (WHERE billing_action = 'capture'), 0)::text AS captured,
    COALESCE(SUM(unit_sell_price * billed_segments) FILTER (WHERE billing_action IS NULL AND unit_sell_price IS NOT NULL), 0)::text AS held,
    COALESCE(SUM(unit_sell_price * billed_segments) FILTER (WHERE billing_action = 'release'), 0)::text AS released,
    COALESCE(MAX(currency)::text, '') AS currency
FROM sms_messages
WHERE campaign_id = sqlc.arg(campaign_id)
  AND direction = 'outbound';

-- name: AdminListCampaignRecipients :many
SELECT
    r.id,
    r.to_msisdn,
    r.status,
    r.sms_message_id,
    r.created_at,
    m.status AS message_status,
    m.provider_sms_id,
    m.provider_status,
    m.billed_segments,
    m.unit_sell_price,
    m.currency,
    m.billing_action,
    j.last_error
FROM campaign_recipients r
LEFT JOIN sms_messages m ON m.id = r.sms_message_id
LEFT JOIN send_jobs j ON j.sms_message_id = m.id
WHERE r.campaign_id = sqlc.arg(campaign_id)
ORDER BY r.created_at, r.id
LIMIT sqlc.arg(page_limit);

-- name: AdminListCampaignRecipientsAfter :many
SELECT
    r.id,
    r.to_msisdn,
    r.status,
    r.sms_message_id,
    r.created_at,
    m.status AS message_status,
    m.provider_sms_id,
    m.provider_status,
    m.billed_segments,
    m.unit_sell_price,
    m.currency,
    m.billing_action,
    j.last_error
FROM campaign_recipients r
LEFT JOIN sms_messages m ON m.id = r.sms_message_id
LEFT JOIN send_jobs j ON j.sms_message_id = m.id
WHERE r.campaign_id = sqlc.arg(campaign_id)
  AND (r.created_at, r.id) > (sqlc.arg(cursor_created_at)::timestamptz, sqlc.arg(cursor_id)::uuid)
ORDER BY r.created_at, r.id
LIMIT sqlc.arg(page_limit);

-- name: AdminGetSmsMessage :one
SELECT
    m.id,
    m.client_id,
    cl.name AS client_name,
    m.direction,
    m.from_msisdn,
    m.to_msisdn,
    m.text,
    m.provider,
    m.provider_sms_id,
    m.provider_status,
    m.pdu_count,
    m.campaign_id,
    m.status,
    m.idempotency_key,
    m.created_at,
    m.accepted_at,
    m.sent_at,
    m.delivered_at,
    m.failed_at,
    m.unit_sell_price,
    m.billed_segments,
    m.tariff_plan_id,
    m.tariff_plan_code,
    m.currency,
    m.billing_action
FROM sms_messages m
LEFT JOIN clients cl ON cl.id = m.client_id
WHERE m.id = sqlc.arg(id);

-- name: AdminGetSmsMessageByProviderID :one
SELECT
    m.id,
    m.client_id,
    cl.name AS client_name,
    m.direction,
    m.from_msisdn,
    m.to_msisdn,
    (CASE WHEN char_length(m.text) > 80 THEN left(m.text, 80) || '…' ELSE m.text END)::text AS text_preview,
    m.status,
    m.provider_status,
    m.pdu_count,
    m.billed_segments,
    m.unit_sell_price,
    m.currency,
    m.billing_action,
    m.campaign_id,
    m.created_at
FROM sms_messages m
LEFT JOIN clients cl ON cl.id = m.client_id
WHERE m.provider_sms_id = sqlc.arg(provider_sms_id)
  AND cl.status IS DISTINCT FROM 'deleted';

-- name: AdminGetSendJobByMessage :one
SELECT
    id,
    sms_message_id,
    status,
    attempt,
    available_at,
    locked_at,
    locked_by,
    last_error,
    created_at,
    updated_at
FROM send_jobs
WHERE sms_message_id = sqlc.arg(sms_message_id);

-- name: AdminListSendAttempts :many
SELECT
    id,
    attempt,
    request_meta,
    http_status,
    response_body,
    latency_ms,
    error_kind,
    created_at
FROM provider_send_attempts
WHERE send_job_id = sqlc.arg(send_job_id)
ORDER BY attempt DESC, created_at DESC
LIMIT 20;

-- name: AdminListMessageCallbacks :many
SELECT
    id,
    kind,
    created_at,
    processed_at,
    parsed
FROM provider_callback_events
WHERE sms_message_id = sqlc.arg(sms_message_id)
ORDER BY created_at DESC
LIMIT 20;
