-- name: InsertLookupJob :one
INSERT INTO lookup_jobs (
    client_id,
    check_type,
    source,
    status,
    item_count,
    unit_sell_price,
    tariff_plan_id,
    tariff_plan_code,
    currency,
    estimated_cost,
    original_filename,
    idempotency_key,
    created_by,
    api_credential_id,
    metadata
) VALUES (
    sqlc.arg(client_id),
    sqlc.arg(check_type),
    sqlc.arg(source),
    sqlc.arg(status),
    sqlc.arg(item_count),
    sqlc.narg(unit_sell_price),
    sqlc.narg(tariff_plan_id),
    sqlc.narg(tariff_plan_code),
    sqlc.arg(currency),
    sqlc.narg(estimated_cost),
    sqlc.narg(original_filename),
    sqlc.narg(idempotency_key),
    sqlc.narg(created_by),
    sqlc.narg(api_credential_id),
    sqlc.arg(metadata)
)
RETURNING *;

-- name: GetLookupJobByIdempotency :one
SELECT *
FROM lookup_jobs
WHERE client_id = sqlc.arg(client_id)
  AND idempotency_key = sqlc.arg(idempotency_key);

-- name: InsertLookupItems :copyfrom
INSERT INTO lookup_items (
    job_id,
    client_id,
    check_type,
    phone_e164,
    phone_digits,
    unit_sell_price,
    tariff_plan_id,
    tariff_plan_code,
    currency,
    estimated_cost
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
);

-- name: ListLookupItemsByJob :many
SELECT *
FROM lookup_items
WHERE job_id = sqlc.arg(job_id)
ORDER BY created_at, id;

-- name: ListQueuedLookupItemIDs :many
SELECT id
FROM lookup_items
WHERE job_id = sqlc.arg(job_id)
  AND status = 'queued'
ORDER BY created_at, id;

-- name: ClaimQueuedLookupItemsFair :many
UPDATE lookup_items AS i
SET status = 'reserved', updated_at = now()
WHERE i.id IN (
    SELECT j.id
    FROM (
        SELECT client_id
        FROM lookup_items
        WHERE status = 'queued'
        GROUP BY client_id
        ORDER BY MIN(created_at)
        LIMIT sqlc.arg(client_limit)
    ) c
    CROSS JOIN LATERAL (
        SELECT id
        FROM lookup_items
        WHERE client_id IS NOT DISTINCT FROM c.client_id
          AND status = 'queued'
        ORDER BY created_at, id
        LIMIT sqlc.arg(per_client)
        FOR UPDATE SKIP LOCKED
    ) j
)
RETURNING *;

-- name: StampLookupItemClientSendID :exec
UPDATE lookup_items
SET provider_code = sqlc.arg(provider_code),
    provider_message_id = sqlc.arg(provider_message_id),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND (provider_message_id IS NULL OR provider_message_id = '');

-- name: ClaimReservedLookupItemsFair :many
UPDATE lookup_items AS i
SET status = status
WHERE i.id IN (
    SELECT j.id
    FROM (
        SELECT client_id
        FROM lookup_items
        WHERE status = 'reserved'
          AND updated_at <= now() - interval '5 seconds'
        GROUP BY client_id
        ORDER BY MIN(updated_at)
        LIMIT sqlc.arg(client_limit)
    ) c
    CROSS JOIN LATERAL (
        SELECT id
        FROM lookup_items
        WHERE client_id IS NOT DISTINCT FROM c.client_id
          AND status = 'reserved'
          AND updated_at <= now() - interval '5 seconds'
        ORDER BY updated_at, id
        LIMIT sqlc.arg(per_client)
        FOR UPDATE SKIP LOCKED
    ) j
)
RETURNING *;

-- name: ClaimItemForSubmit :one
UPDATE lookup_items
SET status = 'reserved', updated_at = now()
WHERE id = sqlc.arg(id)
  AND status IN ('queued', 'reserved')
RETURNING *;

-- name: ClaimPendingLookupItemsFair :many
UPDATE lookup_items AS i
SET next_poll_at = now() + interval '120 seconds',
    updated_at = now()
WHERE i.id IN (
    SELECT j.id
    FROM (
        SELECT client_id
        FROM lookup_items
        WHERE status = 'pending'
          AND next_poll_at IS NOT NULL
          AND next_poll_at <= now()
        GROUP BY client_id
        ORDER BY MIN(next_poll_at)
        LIMIT sqlc.arg(client_limit)
    ) c
    CROSS JOIN LATERAL (
        SELECT id
        FROM lookup_items
        WHERE client_id IS NOT DISTINCT FROM c.client_id
          AND status = 'pending'
          AND next_poll_at IS NOT NULL
          AND next_poll_at <= now()
        ORDER BY next_poll_at, id
        LIMIT sqlc.arg(per_client)
        FOR UPDATE SKIP LOCKED
    ) j
)
RETURNING *;

-- name: MarkLookupJobProcessing :one
UPDATE lookup_jobs
SET status = 'processing',
    started_at = COALESCE(started_at, now()),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND status IN ('queued', 'processing')
RETURNING *;

-- name: RefreshLookupJobCounters :one
UPDATE lookup_jobs j
SET success_count = (
        SELECT count(*)::int FROM lookup_items WHERE job_id = j.id AND status = 'completed'
    ),
    failure_count = (
        SELECT count(*)::int FROM lookup_items WHERE job_id = j.id AND status = 'failed'
    ),
    actual_cost = (
        SELECT COALESCE(sum(i.actual_cost), 0)::billing_money
        FROM lookup_items i
        WHERE i.job_id = j.id
          AND i.billing_action = 'capture'
    ),
    updated_at = now()
WHERE j.id = sqlc.arg(id)
RETURNING *;

-- name: FinalizeLookupJob :one
UPDATE lookup_jobs
SET status = sqlc.arg(status),
    error_code = sqlc.narg(error_code),
    error_message = sqlc.narg(error_message),
    completed_at = now(),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND status IN ('queued', 'processing')
RETURNING *;

-- name: PatchLookupJobMetadata :one
UPDATE lookup_jobs
SET metadata = sqlc.arg(metadata),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: TransitionLookupItem :one
-- billing_action is written only by SetLookupItemBillingAction inside settle.
UPDATE lookup_items
SET status = sqlc.arg(to_status),
    provider_code = COALESCE(sqlc.narg(provider_code), provider_code),
    provider_message_id = COALESCE(sqlc.narg(provider_message_id), provider_message_id),
    result_status = COALESCE(sqlc.narg(result_status), result_status),
    is_reachable = COALESCE(sqlc.narg(is_reachable), is_reachable),
    imsi = COALESCE(sqlc.narg(imsi), imsi),
    mcc = COALESCE(sqlc.narg(mcc), mcc),
    mnc = COALESCE(sqlc.narg(mnc), mnc),
    operator_name = COALESCE(sqlc.narg(operator_name), operator_name),
    country_code = COALESCE(sqlc.narg(country_code), country_code),
    ported = COALESCE(sqlc.narg(ported), ported),
    roaming = COALESCE(sqlc.narg(roaming), roaming),
    normalized_result = COALESCE(sqlc.narg(normalized_result), normalized_result),
    error_code = COALESCE(sqlc.narg(error_code), error_code),
    error_message = COALESCE(sqlc.narg(error_message), error_message),
    next_poll_at = COALESCE(sqlc.narg(next_poll_at), next_poll_at),
    poll_attempts = COALESCE(sqlc.narg(poll_attempts), poll_attempts),
    sent_at = COALESCE(sqlc.narg(sent_at), sent_at),
    completed_at = COALESCE(sqlc.narg(completed_at), completed_at),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND status = ANY(sqlc.arg(from_statuses)::lookup_item_status[])
RETURNING *;

-- name: GetLookupItemByProviderMessage :one
SELECT *
FROM lookup_items
WHERE provider_code = sqlc.arg(provider_code)
  AND provider_message_id = sqlc.arg(provider_message_id);

-- name: ListLookupItemsByProviderMessage :many
SELECT *
FROM lookup_items
WHERE provider_code = sqlc.arg(provider_code)
  AND provider_message_id = sqlc.arg(provider_message_id)
ORDER BY created_at, id;

-- name: ListOpenLookupItemsForCallbackPhone :many
SELECT *
FROM lookup_items
WHERE phone_digits = sqlc.arg(phone_digits)
  AND status IN ('queued', 'reserved', 'pending')
  AND created_at >= sqlc.arg(created_after)
  AND (
    provider_message_id IS NULL
    OR provider_message_id = ''
    OR provider_message_id = sqlc.arg(provider_message_id)
  )
ORDER BY created_at, id;

-- name: ListOpenLookupItemsForCallbackPhoneTail :many
SELECT *
FROM lookup_items
WHERE right(phone_digits, 10) = sqlc.arg(phone_tail)
  AND status IN ('queued', 'reserved', 'pending')
  AND created_at >= sqlc.arg(created_after)
  AND (
    provider_message_id IS NULL
    OR provider_message_id = ''
    OR provider_message_id = sqlc.arg(provider_message_id)
  )
ORDER BY created_at, id;

-- name: ListOpenLookupItemsForCallbackSendID :many
SELECT i.*
FROM provider_lookup_requests r
JOIN lookup_items i ON i.id = r.job_item_id
WHERE r.kind = 'send'
  AND r.created_at >= sqlc.arg(created_after)
  AND i.status IN ('queued', 'reserved', 'pending')
  AND (
    r.provider_message_id = sqlc.arg(callback_id)::text
    OR r.request_payload->>'id' = sqlc.arg(callback_id)::text
  )
ORDER BY i.created_at, i.id;

-- name: ListUnprocessedProviderLookupCallbacks :many
SELECT *
FROM provider_lookup_callbacks
WHERE processed_at IS NULL
ORDER BY created_at, id
LIMIT sqlc.arg(page_limit);

-- name: MarkProviderLookupCallbackProcessed :exec
UPDATE provider_lookup_callbacks
SET processed_at = now(),
    process_error = sqlc.narg(process_error),
    job_item_id = COALESCE(sqlc.narg(job_item_id), job_item_id),
    client_id = COALESCE(sqlc.narg(client_id), client_id)
WHERE id = sqlc.arg(id);

-- name: ReopenUnappliedLookupCallbacks :execrows
UPDATE provider_lookup_callbacks
SET processed_at = NULL,
    process_error = NULL
WHERE processed_at IS NOT NULL
  AND created_at >= sqlc.arg(created_after)
  AND COALESCE(process_error, '') IN ('', 'not_found', 'item_not_found', 'ambiguous', 'phone_mismatch');

-- name: ListStalePendingLookupItems :many
SELECT *
FROM lookup_items
WHERE status = 'pending'
  AND updated_at <= sqlc.arg(older_than)
ORDER BY updated_at, id
LIMIT sqlc.arg(page_limit);

-- name: ListStaleReservedLookupItems :many
SELECT *
FROM lookup_items
WHERE status = 'reserved'
  AND updated_at <= sqlc.arg(older_than)
ORDER BY updated_at, id
LIMIT sqlc.arg(page_limit);

-- name: ListJobsNeedingFinalize :many
SELECT *
FROM lookup_jobs
WHERE status = 'processing'
  AND item_count > 0
  AND item_count = success_count + failure_count
ORDER BY updated_at
LIMIT sqlc.arg(page_limit);

-- name: ListJobsNeedingSubmitResume :many
SELECT j.*
FROM lookup_jobs j
WHERE j.status IN ('queued', 'processing')
  AND j.item_count > 0
  AND EXISTS (
      SELECT 1 FROM lookup_items i
      WHERE i.job_id = j.id AND i.status = 'queued'
  )
  AND j.updated_at <= sqlc.arg(older_than)
ORDER BY j.updated_at
LIMIT sqlc.arg(page_limit);

-- name: ListEmptyCsvLookupShells :many
SELECT *
FROM lookup_jobs
WHERE item_count = 0
  AND status = 'queued'
  AND created_at <= sqlc.arg(older_than)
  AND metadata ? 'csv_file_path'
ORDER BY created_at
LIMIT sqlc.arg(page_limit);

-- name: InsertProviderLookupRequest :one
INSERT INTO provider_lookup_requests (
    client_id,
    job_item_id,
    provider_code,
    kind,
    status,
    provider_message_id,
    request_payload,
    idempotency_key,
    started_at
) VALUES (
    sqlc.narg(client_id),
    sqlc.narg(job_item_id),
    sqlc.arg(provider_code),
    sqlc.arg(kind),
    sqlc.arg(status),
    sqlc.narg(provider_message_id),
    sqlc.arg(request_payload),
    sqlc.narg(idempotency_key),
    sqlc.narg(started_at)
)
RETURNING *;

-- name: UpdateProviderLookupRequest :exec
UPDATE provider_lookup_requests
SET status = sqlc.arg(status),
    provider_message_id = COALESCE(sqlc.narg(provider_message_id), provider_message_id),
    http_status = sqlc.narg(http_status),
    error_code = sqlc.narg(error_code),
    error_message = sqlc.narg(error_message),
    response_payload = sqlc.narg(response_payload),
    normalized_result = sqlc.narg(normalized_result),
    finished_at = sqlc.narg(finished_at)
WHERE id = sqlc.arg(id);

-- name: GetLatestProviderSendByIdempotency :one
SELECT *
FROM provider_lookup_requests
WHERE provider_code = sqlc.arg(provider_code)
  AND idempotency_key = sqlc.arg(idempotency_key)
  AND kind = 'send'
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: GetSucceededProviderSendByIdempotency :one
SELECT *
FROM provider_lookup_requests
WHERE provider_code = sqlc.arg(provider_code)
  AND idempotency_key = sqlc.arg(idempotency_key)
  AND kind = 'send'
  AND status = 'succeeded'
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: InsertProviderLookupCallback :one
INSERT INTO provider_lookup_callbacks (
    client_id,
    job_item_id,
    provider_code,
    provider_message_id,
    raw_payload,
    normalized_result,
    dedupe_key,
    signature_valid,
    processed_at,
    process_error
) VALUES (
    sqlc.narg(client_id),
    sqlc.narg(job_item_id),
    sqlc.arg(provider_code),
    sqlc.narg(provider_message_id),
    sqlc.arg(raw_payload),
    sqlc.narg(normalized_result),
    sqlc.narg(dedupe_key),
    sqlc.narg(signature_valid),
    sqlc.narg(processed_at),
    sqlc.narg(process_error)
)
RETURNING *;

-- name: GetProviderLookupCallbackByDedupe :one
SELECT *
FROM provider_lookup_callbacks
WHERE provider_code = sqlc.arg(provider_code)
  AND dedupe_key = sqlc.arg(dedupe_key);

-- name: GetLookupJobForClient :one
SELECT *
FROM lookup_jobs
WHERE id = sqlc.arg(id)
  AND client_id = sqlc.arg(client_id);

-- name: GetLookupItemForClient :one
SELECT *
FROM lookup_items
WHERE id = sqlc.arg(id)
  AND client_id = sqlc.arg(client_id);

-- name: ListLookupJobs :many
SELECT *
FROM lookup_jobs
WHERE (sqlc.narg(client_id)::uuid IS NULL OR client_id = sqlc.narg(client_id))
  AND (sqlc.narg(filter_status)::lookup_job_status IS NULL OR status = sqlc.narg(filter_status))
  AND (sqlc.narg(filter_check_type)::lookup_check_type IS NULL OR check_type = sqlc.narg(filter_check_type))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountLookupJobs :one
SELECT count(*)::bigint AS n
FROM lookup_jobs
WHERE (sqlc.narg(client_id)::uuid IS NULL OR client_id = sqlc.narg(client_id))
  AND (sqlc.narg(filter_status)::lookup_job_status IS NULL OR status = sqlc.narg(filter_status))
  AND (sqlc.narg(filter_check_type)::lookup_check_type IS NULL OR check_type = sqlc.narg(filter_check_type));

-- name: ListLookupItemsByJobPage :many
SELECT *
FROM lookup_items
WHERE job_id = sqlc.arg(job_id)
ORDER BY created_at, id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountLookupItemsByJob :one
SELECT count(*)::bigint AS n
FROM lookup_items
WHERE job_id = sqlc.arg(job_id);

-- name: InsertLookupCSVPreview :one
INSERT INTO lookup_csv_previews (
    client_id,
    check_type,
    status,
    original_filename,
    phone_count,
    phones_json,
    job_id,
    expires_at
) VALUES (
    sqlc.arg(client_id),
    sqlc.arg(check_type),
    sqlc.arg(status),
    sqlc.narg(original_filename),
    sqlc.arg(phone_count),
    sqlc.arg(phones_json),
    sqlc.narg(job_id),
    sqlc.arg(expires_at)
)
RETURNING *;

-- name: GetLookupCSVPreviewForClient :one
SELECT *
FROM lookup_csv_previews
WHERE id = sqlc.arg(id)
  AND client_id = sqlc.arg(client_id);

-- name: UpdateLookupCSVPreview :one
UPDATE lookup_csv_previews
SET status = COALESCE(sqlc.narg(status), status),
    phone_count = COALESCE(sqlc.narg(phone_count), phone_count),
    phones_json = COALESCE(sqlc.narg(phones_json), phones_json),
    error_message = sqlc.narg(error_message),
    job_id = COALESCE(sqlc.narg(job_id), job_id),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: ClaimLookupCSVPreviewReady :one
UPDATE lookup_csv_previews
SET status = 'consuming',
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND client_id = sqlc.arg(client_id)
  AND status = 'ready'
  AND expires_at > now()
RETURNING *;

-- name: RollbackLookupCSVPreviewConsuming :one
UPDATE lookup_csv_previews
SET status = 'ready',
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND status = 'consuming'
RETURNING *;

-- name: MarkLookupCSVPreviewConsumed :one
UPDATE lookup_csv_previews
SET status = 'consumed',
    job_id = COALESCE(sqlc.narg(job_id), job_id),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND status = 'consuming'
RETURNING *;

-- name: GetLookupJobByCSVPreview :one
SELECT *
FROM lookup_jobs
WHERE client_id = sqlc.arg(client_id)
  AND metadata->>'csv_preview_id' = sqlc.arg(preview_id)::text
ORDER BY created_at
LIMIT 1;

-- name: ClaimLookupCSVPreviewsPendingParse :many
UPDATE lookup_csv_previews AS p
SET status = 'consuming',
    updated_at = now()
WHERE p.id IN (
    SELECT id
    FROM lookup_csv_previews
    WHERE status = 'ready'
      AND job_id IS NOT NULL
      AND phones_json ? 'raw'
    ORDER BY created_at, id
    LIMIT sqlc.arg(page_limit)
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: HealStaleConsumingLookupCSVPreviews :execrows
UPDATE lookup_csv_previews
SET status = CASE
        WHEN job_id IS NOT NULL AND NOT (phones_json ? 'raw') THEN 'consumed'::lookup_csv_preview_status
        ELSE 'ready'::lookup_csv_preview_status
    END,
    updated_at = now()
WHERE status = 'consuming'
  AND updated_at <= sqlc.arg(older_than);

-- name: ClaimLookupItemsNeedingHLREnrich :many
UPDATE lookup_items AS i
SET updated_at = now()
WHERE i.id IN (
    SELECT e.id
    FROM lookup_items e
    WHERE e.check_type = 'hlr'
      AND e.status IN ('completed', 'failed')
      AND e.provider_message_id IS NOT NULL
      AND e.provider_message_id <> ''
      AND e.enrich_attempts < sqlc.arg(max_attempts)
      AND (
            e.imsi IS NULL OR btrim(e.imsi) = ''
            OR COALESCE(e.normalized_result#>>'{extras,msc}', '') = ''
          )
      AND e.updated_at <= now() - interval '15 seconds'
    ORDER BY e.updated_at, e.id
    LIMIT sqlc.arg(page_limit)
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: BumpLookupItemEnrichAttempt :exec
UPDATE lookup_items
SET enrich_attempts = enrich_attempts + 1,
    updated_at = now()
WHERE id = sqlc.arg(id);

-- name: PatchLookupJobAfterParse :one
UPDATE lookup_jobs
SET item_count = sqlc.arg(item_count),
    unit_sell_price = sqlc.narg(unit_sell_price),
    tariff_plan_id = sqlc.narg(tariff_plan_id),
    tariff_plan_code = sqlc.narg(tariff_plan_code),
    currency = sqlc.arg(currency),
    estimated_cost = sqlc.narg(estimated_cost),
    original_filename = COALESCE(sqlc.narg(original_filename), original_filename),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND status = 'queued'
  AND item_count = 0
RETURNING *;

-- name: CountProviderLookupRequestsSince :one
SELECT count(*)::bigint AS n
FROM provider_lookup_requests
WHERE created_at >= sqlc.arg(since);

-- name: CountProviderLookupCallbacksSince :one
SELECT count(*)::bigint AS n
FROM provider_lookup_callbacks
WHERE created_at >= sqlc.arg(since);

-- name: ListRecentProviderLookupRequests :many
SELECT id, provider_code, kind, status, provider_message_id, http_status, error_code, error_message, request_payload, response_payload, created_at
FROM provider_lookup_requests
ORDER BY created_at DESC
LIMIT sqlc.arg(page_limit);

-- name: ListRecentProviderLookupCallbacks :many
SELECT id, provider_code, provider_message_id, signature_valid, processed_at, process_error, raw_payload, created_at
FROM provider_lookup_callbacks
ORDER BY created_at DESC
LIMIT sqlc.arg(page_limit);

-- name: ListLookupItemsForClient :many
SELECT *
FROM lookup_items
WHERE client_id = sqlc.arg(client_id)
  AND (sqlc.narg(filter_status)::lookup_item_status IS NULL OR status = sqlc.narg(filter_status))
  AND (sqlc.narg(filter_check_type)::lookup_check_type IS NULL OR check_type = sqlc.narg(filter_check_type))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountLookupItemsForClient :one
SELECT count(*)::bigint AS n
FROM lookup_items
WHERE client_id = sqlc.arg(client_id)
  AND (sqlc.narg(filter_status)::lookup_item_status IS NULL OR status = sqlc.narg(filter_status))
  AND (sqlc.narg(filter_check_type)::lookup_check_type IS NULL OR check_type = sqlc.narg(filter_check_type));

-- name: CountLookupItemsSinceByStatusForClient :many
SELECT status, count(*)::bigint AS n
FROM lookup_items
WHERE client_id = sqlc.arg(client_id)
  AND created_at >= sqlc.arg(since)
GROUP BY status;

-- name: CountLookupItemsSinceByTypeForClient :many
SELECT check_type, count(*)::bigint AS n
FROM lookup_items
WHERE client_id = sqlc.arg(client_id)
  AND created_at >= sqlc.arg(since)
GROUP BY check_type;

-- name: DeleteLookupCSVPreview :execrows
DELETE FROM lookup_csv_previews
WHERE id = sqlc.arg(id)
  AND client_id = sqlc.arg(client_id)
  AND status IN ('ready', 'invalid', 'expired');
