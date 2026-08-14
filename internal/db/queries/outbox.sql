-- name: InsertSendJob :one
INSERT INTO send_jobs (sms_message_id, client_id, status)
VALUES ($1, $2, 'pending')
RETURNING *;

-- name: GetSendJobByID :one
SELECT * FROM send_jobs WHERE id = sqlc.arg(id);

-- name: GetSendJobByMessageID :one
SELECT * FROM send_jobs WHERE sms_message_id = sqlc.arg(sms_message_id);

-- name: ClaimSendJobsFair :many
UPDATE send_jobs AS s
SET
    status = 'processing',
    locked_at = now(),
    locked_by = sqlc.arg(worker_id),
    updated_at = now()
WHERE s.id IN (
    SELECT j.id
    FROM (
        SELECT client_id
        FROM send_jobs
        WHERE status IN ('pending', 'retry', 'uncertain')
          AND available_at <= now()
        GROUP BY client_id
        ORDER BY MIN(available_at)
        LIMIT sqlc.arg(batch_limit)
    ) c
    CROSS JOIN LATERAL (
        SELECT id
        FROM send_jobs
        WHERE client_id IS NOT DISTINCT FROM c.client_id
          AND status IN ('pending', 'retry', 'uncertain')
          AND available_at <= now()
        ORDER BY
            CASE status
                WHEN 'uncertain' THEN 0
                WHEN 'retry' THEN 1
                ELSE 2
            END,
            available_at,
            id
        LIMIT 1
        FOR UPDATE SKIP LOCKED
    ) j
)
RETURNING *;

-- name: TouchSendJobLock :one
UPDATE send_jobs
SET locked_at = now(), updated_at = now()
WHERE id = sqlc.arg(id)
  AND status = 'processing'
  AND locked_by = sqlc.arg(worker_id)
RETURNING *;

-- name: CompleteSendJob :execrows
UPDATE send_jobs
SET status = 'done',
    locked_at = NULL,
    locked_by = NULL,
    last_error = NULL,
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND status = 'processing'
  AND locked_by = sqlc.arg(worker_id);

-- name: ParkSendJob :execrows
UPDATE send_jobs
SET status = sqlc.arg(status),
    attempt = sqlc.arg(attempt),
    available_at = sqlc.arg(available_at),
    locked_at = NULL,
    locked_by = NULL,
    last_error = sqlc.arg(last_error),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND status = 'processing'
  AND locked_by = sqlc.arg(worker_id);

-- name: ReclaimStaleSendJobs :execrows
UPDATE send_jobs
SET status = 'uncertain',
    last_error = 'uncertain:need_stat',
    attempt = GREATEST(attempt, 1),
    locked_at = NULL,
    locked_by = NULL,
    available_at = now(),
    updated_at = now()
WHERE status = 'processing'
  AND locked_at < sqlc.arg(stale_before);

-- name: InsertSendAttempt :exec
INSERT INTO provider_send_attempts (
    send_job_id, attempt, request_meta, http_status, response_body, latency_ms, error_kind
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
);

-- name: GetLatestSendAttempt :one
SELECT * FROM provider_send_attempts
WHERE send_job_id = sqlc.arg(send_job_id)
ORDER BY attempt DESC, created_at DESC
LIMIT 1;
