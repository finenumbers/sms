-- name: CountSendJobsByStatus :many
SELECT status, count(*)::bigint AS n
FROM send_jobs
GROUP BY status;

-- name: CountSmsMessagesByStatus :many
SELECT status, count(*)::bigint AS n
FROM sms_messages
GROUP BY status;

-- name: CountUnprocessedCallbacks :one
SELECT count(*)::bigint AS n
FROM provider_callback_events
WHERE processed_at IS NULL;

-- name: CountLookupItemsByStatus :many
SELECT status, count(*)::bigint AS n
FROM lookup_items
GROUP BY status;

-- name: CountLookupJobsByStatus :many
SELECT status, count(*)::bigint AS n
FROM lookup_jobs
GROUP BY status;

-- name: OldestUnprocessedLookupCallbackAt :one
SELECT min(created_at) AS created_at
FROM provider_lookup_callbacks
WHERE processed_at IS NULL;
