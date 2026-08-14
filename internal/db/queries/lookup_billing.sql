-- name: GetLookupJob :one
SELECT * FROM lookup_jobs WHERE id = sqlc.arg(id);

-- name: GetLookupItem :one
SELECT * FROM lookup_items WHERE id = sqlc.arg(id);

-- name: SetLookupItemBillingAction :exec
UPDATE lookup_items
SET billing_action = sqlc.arg(billing_action),
    actual_cost = sqlc.narg(actual_cost),
    updated_at = now()
WHERE id = sqlc.arg(id);

-- name: GetLookupJobHold :one
SELECT *
FROM wallet_transactions
WHERE lookup_job_id = sqlc.arg(lookup_job_id)
  AND type = 'HOLD'
ORDER BY created_at
LIMIT 1;

-- name: SumSettledAgainstHold :one
SELECT COALESCE(sum(amount), 0)::billing_money AS settled
FROM wallet_transactions
WHERE related_hold_id = sqlc.arg(hold_id)
  AND type IN ('DEBIT', 'RELEASE');

-- name: ListUnsettledTerminalLookupItems :many
SELECT *
FROM lookup_items
WHERE job_id = sqlc.arg(job_id)
  AND status IN ('completed', 'failed')
  AND billing_action IS NULL
ORDER BY created_at, id;

-- name: CountLookupItemsBlockingRemainder :one
-- Remainder must not run while any item is still open or terminal-but-unposted.
SELECT count(*)::bigint AS n
FROM lookup_items
WHERE job_id = sqlc.arg(job_id)
  AND (
    status NOT IN ('completed', 'failed')
    OR billing_action IS NULL
  );

-- name: ListOpenLookupJobHolds :many
SELECT
    h.id AS hold_id,
    h.client_id,
    h.lookup_job_id,
    h.amount AS hold_amount,
    (h.amount - COALESCE((
        SELECT sum(s.amount)
        FROM wallet_transactions s
        WHERE s.related_hold_id = h.id
          AND s.type IN ('DEBIT', 'RELEASE')
    ), 0))::billing_money AS remaining,
    j.status AS job_status
FROM wallet_transactions h
JOIN lookup_jobs j ON j.id = h.lookup_job_id
WHERE h.type = 'HOLD'
  AND h.lookup_job_id IS NOT NULL
  AND (h.amount - COALESCE((
        SELECT sum(s.amount)
        FROM wallet_transactions s
        WHERE s.related_hold_id = h.id
          AND s.type IN ('DEBIT', 'RELEASE')
    ), 0)) > 0
ORDER BY h.created_at
LIMIT sqlc.arg(page_limit);

-- name: CountOpenLookupHolds :one
SELECT count(*)::bigint AS n
FROM wallet_transactions h
WHERE h.type = 'HOLD'
  AND h.lookup_job_id IS NOT NULL
  AND (h.amount - COALESCE((
        SELECT sum(s.amount)
        FROM wallet_transactions s
        WHERE s.related_hold_id = h.id
          AND s.type IN ('DEBIT', 'RELEASE')
    ), 0)) > 0;
