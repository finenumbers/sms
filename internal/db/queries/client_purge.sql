-- name: TryAdvisoryLockClientPurge :one
SELECT pg_try_advisory_lock(sqlc.arg(key1)::integer, sqlc.arg(key2)::integer) AS locked;

-- name: AdvisoryUnlockClientPurge :one
SELECT pg_advisory_unlock(sqlc.arg(key1)::integer, sqlc.arg(key2)::integer) AS unlocked;

-- name: MarkClientDeleted :one
UPDATE clients
SET status = 'deleted',
    name = 'deleted',
    purged_at = NULL,
    updated_at = now()
WHERE id = sqlc.arg(id) AND status <> 'deleted'
RETURNING *;

-- name: MarkClientPurged :exec
UPDATE clients
SET purged_at = now(), updated_at = now()
WHERE id = sqlc.arg(id) AND status = 'deleted' AND purged_at IS NULL;

-- name: ClearClientPurgedAt :exec
UPDATE clients
SET purged_at = NULL, updated_at = now()
WHERE id = sqlc.arg(id) AND status = 'deleted' AND purged_at IS NOT NULL;

-- name: ListClientsPendingPurge :many
SELECT id
FROM clients
WHERE status = 'deleted' AND purged_at IS NULL
ORDER BY updated_at
LIMIT sqlc.arg(page_limit);

-- name: ListPurgedDeletedClientIDs :many
SELECT id
FROM clients
WHERE status = 'deleted' AND purged_at IS NOT NULL
ORDER BY updated_at
LIMIT sqlc.arg(page_limit);

-- name: ClientHasPurgeLeftover :one
SELECT (
    EXISTS (SELECT 1 FROM webhook_deliveries d WHERE d.client_id = sqlc.arg(cid))
    OR EXISTS (SELECT 1 FROM webhook_endpoints e WHERE e.client_id = sqlc.arg(cid))
    OR EXISTS (SELECT 1 FROM provider_lookup_callbacks c WHERE c.client_id = sqlc.arg(cid))
    OR EXISTS (
        SELECT 1 FROM provider_lookup_callbacks c
        JOIN lookup_items i ON i.id = c.job_item_id
        WHERE i.client_id = sqlc.arg(cid)
    )
    OR EXISTS (SELECT 1 FROM provider_lookup_requests r WHERE r.client_id = sqlc.arg(cid))
    OR EXISTS (
        SELECT 1 FROM provider_lookup_requests r
        JOIN lookup_items i ON i.id = r.job_item_id
        WHERE i.client_id = sqlc.arg(cid)
    )
    OR EXISTS (SELECT 1 FROM lookup_csv_previews p WHERE p.client_id = sqlc.arg(cid))
    OR EXISTS (SELECT 1 FROM wallet_transactions t WHERE t.client_id = sqlc.arg(cid))
    OR EXISTS (SELECT 1 FROM lookup_items i WHERE i.client_id = sqlc.arg(cid))
    OR EXISTS (SELECT 1 FROM lookup_jobs j WHERE j.client_id = sqlc.arg(cid))
    OR EXISTS (SELECT 1 FROM send_jobs j WHERE j.client_id = sqlc.arg(cid))
    OR EXISTS (
        SELECT 1 FROM send_jobs j
        JOIN sms_messages m ON m.id = j.sms_message_id
        WHERE m.client_id = sqlc.arg(cid)
    )
    OR EXISTS (
        SELECT 1 FROM provider_callback_events e
        JOIN sms_messages m ON m.id = e.sms_message_id
        WHERE m.client_id = sqlc.arg(cid)
    )
    OR EXISTS (SELECT 1 FROM sms_messages m WHERE m.client_id = sqlc.arg(cid))
    OR EXISTS (SELECT 1 FROM sms_campaigns c WHERE c.client_id = sqlc.arg(cid))
    OR EXISTS (SELECT 1 FROM client_tariffs t WHERE t.client_id = sqlc.arg(cid))
    OR EXISTS (SELECT 1 FROM wallets w WHERE w.client_id = sqlc.arg(cid))
    OR EXISTS (SELECT 1 FROM number_assignments a WHERE a.client_id = sqlc.arg(cid))
    OR EXISTS (SELECT 1 FROM api_credentials k WHERE k.client_id = sqlc.arg(cid))
    OR EXISTS (SELECT 1 FROM client_users u WHERE u.client_id = sqlc.arg(cid))
    OR EXISTS (
        SELECT 1 FROM audit_log a
        WHERE (
                a.client_id = sqlc.arg(cid)
                OR a.actor_id IN (SELECT u.id FROM client_users u WHERE u.client_id = sqlc.arg(cid))
            )
          AND NOT (a.action = 'client.delete' AND a.client_id = sqlc.arg(cid))
    )
    OR EXISTS (
        SELECT 1 FROM ops_events e
        WHERE e.client_id = sqlc.arg(cid)
           OR e.actor_id IN (SELECT u.id FROM client_users u WHERE u.client_id = sqlc.arg(cid))
    )
    OR EXISTS (
        SELECT 1 FROM sessions s
        JOIN client_users u ON u.id = s.client_user_id
        WHERE u.client_id = sqlc.arg(cid)
    )
    OR EXISTS (
        SELECT 1 FROM idempotency_keys k
        WHERE k.principal_id IN (
            SELECT u.id FROM client_users u WHERE u.client_id = sqlc.arg(cid)
            UNION ALL
            SELECT cr.id FROM api_credentials cr WHERE cr.client_id = sqlc.arg(cid)
        )
    )
)::bool AS leftover;

-- name: CancelOpenCampaignsForClient :execrows
UPDATE sms_campaigns
SET status = 'cancelled', updated_at = now()
WHERE client_id = sqlc.arg(cid)
  AND status IN ('queued', 'running');

-- name: DisableWebhookEndpointsForClient :execrows
UPDATE webhook_endpoints
SET enabled = false, updated_at = now()
WHERE client_id = sqlc.arg(cid) AND enabled;

-- name: ScrambleClientUsersForDelete :execrows
UPDATE client_users
SET email = 'deleted-' || id::text || '@invalid',
    name = '',
    status = 'disabled',
    updated_at = now()
WHERE client_id = sqlc.arg(cid)
  AND email NOT LIKE 'deleted-%@invalid';

-- name: PurgeWebhookDeliveriesForClient :execrows
DELETE FROM webhook_deliveries
WHERE id IN (
    SELECT d.id FROM webhook_deliveries d
    WHERE d.client_id = sqlc.arg(cid)
    LIMIT 500
);

-- name: PurgeWebhookEndpointsForClient :execrows
DELETE FROM webhook_endpoints
WHERE id IN (
    SELECT e.id FROM webhook_endpoints e
    WHERE e.client_id = sqlc.arg(cid)
    LIMIT 100
);

-- name: PurgeProviderLookupCallbacksForClient :execrows
DELETE FROM provider_lookup_callbacks
WHERE id IN (
    SELECT c.id
    FROM provider_lookup_callbacks c
    WHERE c.client_id = sqlc.arg(cid)
       OR c.job_item_id IN (SELECT id FROM lookup_items WHERE client_id = sqlc.arg(cid))
    LIMIT 500
);

-- name: PurgeProviderLookupRequestsForClient :execrows
DELETE FROM provider_lookup_requests
WHERE id IN (
    SELECT r.id
    FROM provider_lookup_requests r
    WHERE r.client_id = sqlc.arg(cid)
       OR r.job_item_id IN (SELECT id FROM lookup_items WHERE client_id = sqlc.arg(cid))
    LIMIT 500
);

-- name: PurgeLookupCSVPreviewsForClient :execrows
DELETE FROM lookup_csv_previews
WHERE id IN (
    SELECT p.id FROM lookup_csv_previews p
    WHERE p.client_id = sqlc.arg(cid)
    LIMIT 200
);

-- name: PurgeWalletTransactionsForClient :execrows
DELETE FROM wallet_transactions
WHERE id IN (
    SELECT t.id FROM wallet_transactions t
    WHERE t.client_id = sqlc.arg(cid)
    LIMIT 500
);

-- name: PurgeLookupItemsForClient :execrows
DELETE FROM lookup_items
WHERE id IN (
    SELECT i.id FROM lookup_items i
    WHERE i.client_id = sqlc.arg(cid)
    LIMIT 500
);

-- name: PurgeLookupJobsForClient :execrows
DELETE FROM lookup_jobs
WHERE id IN (
    SELECT j.id FROM lookup_jobs j
    WHERE j.client_id = sqlc.arg(cid)
    LIMIT 100
);

-- name: PurgeSendJobsForClient :execrows
WITH doomed AS (
    SELECT j.id
    FROM send_jobs j
    WHERE j.client_id = sqlc.arg(cid)
       OR j.sms_message_id IN (SELECT id FROM sms_messages WHERE client_id = sqlc.arg(cid))
    LIMIT 200
),
del_attempts AS (
    DELETE FROM provider_send_attempts
    WHERE send_job_id IN (SELECT id FROM doomed)
)
DELETE FROM send_jobs
WHERE id IN (SELECT id FROM doomed);

-- name: PurgeCallbackEventsForClientMessages :execrows
DELETE FROM provider_callback_events
WHERE id IN (
    SELECT e.id
    FROM provider_callback_events e
    JOIN sms_messages m ON m.id = e.sms_message_id
    WHERE m.client_id = sqlc.arg(cid)
    LIMIT 500
);

-- name: UnlinkCampaignRecipientsForClient :execrows
UPDATE campaign_recipients
SET sms_message_id = NULL
WHERE sms_message_id IN (
    SELECT id FROM sms_messages WHERE client_id = sqlc.arg(cid)
);

-- name: PurgeSmsMessagesForClient :execrows
DELETE FROM sms_messages
WHERE id IN (
    SELECT m.id FROM sms_messages m
    WHERE m.client_id = sqlc.arg(cid)
    LIMIT 200
);

-- name: PurgeSmsCampaignsForClient :execrows
DELETE FROM sms_campaigns
WHERE id IN (
    SELECT c.id FROM sms_campaigns c
    WHERE c.client_id = sqlc.arg(cid)
    LIMIT 100
);

-- name: PurgeClientTariffsForClient :execrows
DELETE FROM client_tariffs WHERE client_id = sqlc.arg(cid);

-- name: PurgeWalletsForClient :execrows
DELETE FROM wallets WHERE client_id = sqlc.arg(cid);

-- name: UnlinkDirectionJobsForClient :execrows
UPDATE number_direction_jobs
SET assignment_id = NULL
WHERE assignment_id IN (
    SELECT id FROM number_assignments WHERE client_id = sqlc.arg(cid)
);

-- name: PurgeNumberAssignmentsForClient :execrows
DELETE FROM number_assignments WHERE client_id = sqlc.arg(cid);

-- name: PurgeIdempotencyForClient :execrows
DELETE FROM idempotency_keys
WHERE principal_id IN (
    SELECT u.id FROM client_users u WHERE u.client_id = sqlc.arg(cid)
    UNION ALL
    SELECT cr.id FROM api_credentials cr WHERE cr.client_id = sqlc.arg(cid)
);

-- name: PurgeSessionsForClient :execrows
DELETE FROM sessions
WHERE client_user_id IN (SELECT id FROM client_users WHERE client_id = sqlc.arg(cid));

-- name: PurgeAPICredentialsForClient :execrows
DELETE FROM api_credentials WHERE client_id = sqlc.arg(cid);

-- name: PurgeClientUsersForClient :execrows
DELETE FROM client_users WHERE client_id = sqlc.arg(cid);

-- name: PurgeAuditLogForClient :execrows
DELETE FROM audit_log
WHERE id IN (
    SELECT a.id
    FROM audit_log a
    WHERE (
            a.client_id = sqlc.arg(cid)
            OR a.actor_id IN (SELECT id FROM client_users WHERE client_id = sqlc.arg(cid))
        )
      AND NOT (a.action = 'client.delete' AND a.client_id = sqlc.arg(cid))
    LIMIT 1000
);

-- name: PurgeOpsEventsForClient :execrows
DELETE FROM ops_events
WHERE id IN (
    SELECT e.id
    FROM ops_events e
    WHERE e.client_id = sqlc.arg(cid)
       OR e.actor_id IN (SELECT id FROM client_users WHERE client_id = sqlc.arg(cid))
    LIMIT 1000
);
