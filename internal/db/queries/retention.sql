-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions
WHERE expires_at < now()
   OR (revoked_at IS NOT NULL AND revoked_at < now() - interval '7 days');

-- name: DeleteOldAuditLog :execrows
DELETE FROM audit_log
WHERE id IN (
    SELECT a.id FROM audit_log a
    WHERE a.created_at < sqlc.arg(before)
    ORDER BY a.created_at
    LIMIT 1000
);

-- name: DeleteOldOpsEvents :execrows
DELETE FROM ops_events
WHERE id IN (
    SELECT e.id FROM ops_events e
    WHERE e.created_at < sqlc.arg(before)
    ORDER BY e.created_at
    LIMIT 5000
);

-- name: DeleteOldCallbackEvents :execrows
DELETE FROM provider_callback_events
WHERE id IN (
    SELECT e.id FROM provider_callback_events e
    WHERE e.created_at < sqlc.arg(before)
    ORDER BY e.created_at
    LIMIT 1000
);

-- name: DeleteOldSmsMessages :execrows
WITH doomed AS (
    SELECT m.id
    FROM sms_messages m
    WHERE m.created_at < sqlc.arg(before)
      AND NOT (m.unit_sell_price IS NOT NULL AND m.billing_action IS NULL)
    ORDER BY m.created_at
    LIMIT 200
),
unlinked_rec AS (
    UPDATE campaign_recipients
    SET sms_message_id = NULL
    WHERE sms_message_id IN (SELECT id FROM doomed)
),
unlinked_cb AS (
    UPDATE provider_callback_events
    SET sms_message_id = NULL
    WHERE sms_message_id IN (SELECT id FROM doomed)
),
del_attempts AS (
    DELETE FROM provider_send_attempts
    WHERE send_job_id IN (
        SELECT id FROM send_jobs WHERE sms_message_id IN (SELECT id FROM doomed)
    )
),
del_jobs AS (
    DELETE FROM send_jobs
    WHERE sms_message_id IN (SELECT id FROM doomed)
)
DELETE FROM sms_messages
WHERE id IN (SELECT id FROM doomed);

-- name: DeleteOldCampaigns :execrows
DELETE FROM sms_campaigns
WHERE created_at < sqlc.arg(before)
  AND status IN ('completed', 'failed', 'cancelled', 'draft');
