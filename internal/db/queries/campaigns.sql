-- name: InsertCampaign :one
INSERT INTO sms_campaigns (client_id, from_msisdn, text, created_by)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetCampaignByID :one
SELECT * FROM sms_campaigns WHERE id = sqlc.arg(id);

-- name: GetCampaignForClient :one
SELECT * FROM sms_campaigns
WHERE id = sqlc.arg(id) AND client_id = sqlc.arg(client_id)::uuid;

-- name: GetCampaignForClientForUpdate :one
SELECT * FROM sms_campaigns
WHERE id = sqlc.arg(id) AND client_id = sqlc.arg(client_id)::uuid
FOR UPDATE;

-- name: ListCampaignsForClient :many
SELECT * FROM sms_campaigns
WHERE client_id = sqlc.arg(client_id)::uuid
  AND (sqlc.narg('status')::campaign_status IS NULL OR status = sqlc.narg('status'))
ORDER BY created_at DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: UpdateCampaignDraft :one
UPDATE sms_campaigns
SET
    from_msisdn = COALESCE(sqlc.narg('from_msisdn'), from_msisdn),
    text = COALESCE(sqlc.narg('text'), text),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND client_id = sqlc.arg(client_id)::uuid
  AND status = 'draft'
RETURNING *;

-- name: DeleteCampaignDraft :execrows
DELETE FROM sms_campaigns
WHERE id = sqlc.arg(id)
  AND client_id = sqlc.arg(client_id)::uuid
  AND status = 'draft';

-- name: QueueCampaign :one
UPDATE sms_campaigns
SET status = 'queued', updated_at = now()
WHERE id = sqlc.arg(id)
  AND client_id = sqlc.arg(client_id)::uuid
  AND status = 'draft'
  AND total_count > 0
RETURNING *;

-- name: CancelCampaign :one
UPDATE sms_campaigns
SET status = 'cancelled', updated_at = now()
WHERE id = sqlc.arg(id)
  AND client_id = sqlc.arg(client_id)::uuid
  AND status IN ('queued', 'running')
RETURNING *;

-- name: SetCampaignTotal :exec
UPDATE sms_campaigns
SET total_count = sqlc.arg(total_count), updated_at = now()
WHERE id = sqlc.arg(id);

-- name: ClaimQueuedCampaigns :many
UPDATE sms_campaigns
SET status = 'running', updated_at = now()
WHERE id IN (
    SELECT id
    FROM sms_campaigns
    WHERE status = 'queued'
      AND EXISTS (
          SELECT 1 FROM clients cl
          WHERE cl.id = sms_campaigns.client_id AND cl.status = 'active'
      )
    ORDER BY created_at, id
    LIMIT sqlc.arg(batch_limit)
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: CompleteIdleCampaigns :execrows
UPDATE sms_campaigns c
SET status = 'completed', updated_at = now()
WHERE c.status = 'running'
  AND NOT EXISTS (
      SELECT 1 FROM campaign_recipients r
      WHERE r.campaign_id = c.id AND r.status = 'pending'
  )
  AND NOT EXISTS (
      SELECT 1
      FROM sms_messages m
      JOIN send_jobs j ON j.sms_message_id = m.id
      WHERE m.campaign_id = c.id
        AND j.status IN ('pending', 'processing', 'retry', 'uncertain')
  );

-- name: RefreshCampaignCounters :exec
UPDATE sms_campaigns c
SET
    accepted_count = COALESCE(s.accepted, 0),
    delivered_count = COALESCE(s.delivered, 0),
    failed_count = COALESCE(s.failed, 0),
    updated_at = now()
FROM (
    SELECT
        m.campaign_id,
        COUNT(*) FILTER (WHERE m.status IN ('accepted', 'sent', 'delivered'))::int AS accepted,
        COUNT(*) FILTER (WHERE m.status = 'delivered')::int AS delivered,
        COUNT(*) FILTER (WHERE m.status = 'failed')::int AS failed
    FROM sms_messages m
    WHERE m.campaign_id IS NOT NULL
    GROUP BY m.campaign_id
) s
WHERE c.id = s.campaign_id
  AND (
      c.status IN ('running', 'cancelled')
      OR (c.status = 'completed' AND c.updated_at > sqlc.arg(since))
  );

-- name: ListCampaignRecipientMSISDNs :many
SELECT to_msisdn FROM campaign_recipients WHERE campaign_id = sqlc.arg(campaign_id);

-- name: CountCampaignRecipients :one
SELECT COUNT(*)::int AS n FROM campaign_recipients WHERE campaign_id = sqlc.arg(campaign_id);

-- name: CampaignRecipientStats :one
SELECT
    COUNT(*)::int AS total,
    COUNT(*) FILTER (WHERE status = 'pending')::int AS pending,
    COUNT(*) FILTER (WHERE status = 'enqueued')::int AS enqueued,
    COUNT(*) FILTER (WHERE status = 'skipped')::int AS skipped,
    COUNT(*) FILTER (WHERE status = 'failed')::int AS failed
FROM campaign_recipients
WHERE campaign_id = sqlc.arg(campaign_id);

-- name: ListCampaignRecipients :many
SELECT * FROM campaign_recipients
WHERE campaign_id = sqlc.arg(campaign_id)
ORDER BY created_at, id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: ListCampaignRecipientRows :many
SELECT
    r.id,
    r.campaign_id,
    r.to_msisdn,
    r.status,
    r.sms_message_id,
    r.created_at,
    m.status AS message_status
FROM campaign_recipients r
LEFT JOIN sms_messages m ON m.id = r.sms_message_id
WHERE r.campaign_id = sqlc.arg(campaign_id)
ORDER BY r.created_at, r.id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: InsertCampaignRecipients :execrows
INSERT INTO campaign_recipients (campaign_id, to_msisdn)
SELECT sqlc.arg(campaign_id), x
FROM unnest(sqlc.arg(msisdns)::text[]) AS x
ON CONFLICT (campaign_id, to_msisdn) DO NOTHING;

-- name: ListFanoutClientIDs :many
SELECT c.client_id
FROM campaign_recipients r
JOIN sms_campaigns c ON c.id = r.campaign_id
WHERE r.status = 'pending' AND c.status = 'running'
GROUP BY c.client_id
ORDER BY MIN(r.created_at)
LIMIT sqlc.arg(client_limit);

-- name: LockPendingRecipientsForClient :many
SELECT
    r.id,
    r.campaign_id,
    r.to_msisdn,
    r.status,
    r.sms_message_id,
    r.created_at,
    c.client_id,
    c.from_msisdn,
    c.text,
    c.status AS campaign_status
FROM campaign_recipients r
INNER JOIN sms_campaigns c ON c.id = r.campaign_id
WHERE c.client_id = sqlc.arg(client_id)
  AND r.status = 'pending'
  AND c.status = 'running'
ORDER BY r.created_at, r.id
LIMIT sqlc.arg(per_client)
FOR UPDATE OF r SKIP LOCKED;

-- name: MarkRecipientEnqueued :exec
UPDATE campaign_recipients
SET status = 'enqueued', sms_message_id = sqlc.arg(sms_message_id)
WHERE id = sqlc.arg(id) AND status = 'pending';

-- name: MarkRecipientSkipped :exec
UPDATE campaign_recipients
SET status = 'skipped'
WHERE id = sqlc.arg(id) AND status = 'pending';

-- name: MarkRecipientFailed :exec
UPDATE campaign_recipients
SET status = 'failed'
WHERE id = sqlc.arg(id) AND status = 'pending';

-- name: SkipPendingForCampaign :execrows
UPDATE campaign_recipients AS r
SET status = 'skipped'
FROM (
    SELECT cr.id
    FROM campaign_recipients cr
    WHERE cr.campaign_id = sqlc.arg(campaign_id) AND cr.status = 'pending'
    FOR UPDATE SKIP LOCKED
) AS s
WHERE r.id = s.id AND r.status = 'pending';

-- name: SkipPendingForCancelledCampaigns :execrows
UPDATE campaign_recipients AS r
SET status = 'skipped'
FROM (
    SELECT r2.id
    FROM campaign_recipients r2
    JOIN sms_campaigns c ON c.id = r2.campaign_id
    WHERE r2.status = 'pending' AND c.status = 'cancelled'
    FOR UPDATE OF r2 SKIP LOCKED
) AS s
WHERE r.id = s.id AND r.status = 'pending';

-- name: CampaignRunnableForFanout :one
SELECT
    (c.status = 'running')::bool AS running,
    EXISTS (
        SELECT 1
        FROM def_numbers n
        JOIN number_assignments a ON a.def_number_id = n.id AND a.unassigned_at IS NULL
        WHERE n.msisdn = c.from_msisdn
          AND a.client_id = c.client_id
          AND n.status = 'assigned'
    )::bool AS assigned
FROM sms_campaigns c
WHERE c.id = sqlc.arg(id);
