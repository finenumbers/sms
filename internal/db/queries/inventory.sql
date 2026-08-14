-- name: InsertDefNumber :one
INSERT INTO def_numbers (msisdn, region, notes)
VALUES ($1, $2, $3)
ON CONFLICT (msisdn) DO NOTHING
RETURNING *;

-- name: UpsertDefNumberFromSync :one
INSERT INTO def_numbers (msisdn, region, supports_sms, runexis_snapshot)
VALUES (sqlc.arg(msisdn), sqlc.narg(region), sqlc.arg(supports_sms), sqlc.narg(runexis_snapshot))
ON CONFLICT (msisdn) DO UPDATE SET
    supports_sms = EXCLUDED.supports_sms,
    runexis_snapshot = COALESCE(EXCLUDED.runexis_snapshot, def_numbers.runexis_snapshot),
    region = COALESCE(def_numbers.region, EXCLUDED.region),
    updated_at = now()
RETURNING *;

-- name: SetDefNumberSupportsSMSByMSISDN :execrows
UPDATE def_numbers
SET supports_sms = sqlc.arg(supports_sms),
    runexis_snapshot = COALESCE(sqlc.narg(runexis_snapshot), runexis_snapshot),
    updated_at = now()
WHERE msisdn = sqlc.arg(msisdn);

-- name: GetDefNumberByID :one
SELECT * FROM def_numbers WHERE id = sqlc.arg(id);

-- name: GetDefNumberByIDForUpdate :one
SELECT * FROM def_numbers WHERE id = sqlc.arg(id) FOR UPDATE;

-- name: GetDefNumberByMSISDN :one
SELECT * FROM def_numbers WHERE msisdn = sqlc.arg(msisdn);

-- name: UpdateDefNumberStatus :one
UPDATE def_numbers
SET status = sqlc.arg(status), updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: UpdateDefNumberMeta :one
UPDATE def_numbers
SET region = sqlc.arg(region), notes = sqlc.arg(notes), updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: ListDefNumbers :many
SELECT
    n.id,
    n.msisdn,
    n.status,
    n.region,
    n.notes,
    n.supports_sms,
    n.created_at,
    n.updated_at,
    a.id AS assignment_id,
    a.client_id,
    a.assigned_at,
    c.name AS client_name
FROM def_numbers n
LEFT JOIN number_assignments a ON a.def_number_id = n.id AND a.unassigned_at IS NULL
LEFT JOIN clients c ON c.id = a.client_id
WHERE (sqlc.narg('status')::def_number_status IS NULL OR n.status = sqlc.narg('status'))
  AND (sqlc.narg('client_id')::uuid IS NULL OR a.client_id = sqlc.narg('client_id'))
  AND (sqlc.narg('q')::text IS NULL OR n.msisdn LIKE '%' || sqlc.narg('q') || '%')
ORDER BY n.created_at DESC
LIMIT sqlc.arg('page_limit') OFFSET sqlc.arg('page_offset');

-- name: GetDefNumberView :one
SELECT
    n.id,
    n.msisdn,
    n.status,
    n.region,
    n.notes,
    n.supports_sms,
    n.created_at,
    n.updated_at,
    a.id AS assignment_id,
    a.client_id,
    a.assigned_at,
    c.name AS client_name
FROM def_numbers n
LEFT JOIN number_assignments a ON a.def_number_id = n.id AND a.unassigned_at IS NULL
LEFT JOIN clients c ON c.id = a.client_id
WHERE n.id = sqlc.arg(id);

-- name: InsertAssignment :one
INSERT INTO number_assignments (def_number_id, client_id, assigned_by)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetOpenAssignmentByNumber :one
SELECT * FROM number_assignments
WHERE def_number_id = sqlc.arg(def_number_id) AND unassigned_at IS NULL;

-- name: ListOpenAssignmentsByClient :many
SELECT * FROM number_assignments
WHERE client_id = sqlc.arg(client_id) AND unassigned_at IS NULL
FOR UPDATE;

-- name: CloseAssignment :one
UPDATE number_assignments
SET unassigned_at = now()
WHERE id = sqlc.arg(id) AND unassigned_at IS NULL
RETURNING *;

-- name: InsertDirectionJob :one
INSERT INTO number_direction_jobs (
    def_number_id, assignment_id, msisdn,
    dir_in, dir_dom_out, dir_int_out, dir_in_mass
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
RETURNING *;

-- name: ClaimDirectionJobs :many
UPDATE number_direction_jobs
SET status = 'processing',
    locked_at = now(),
    locked_by = sqlc.arg(worker_id),
    updated_at = now()
WHERE id IN (
    SELECT id FROM number_direction_jobs
    WHERE status IN ('pending', 'retry')
      AND available_at <= now()
    ORDER BY available_at
    LIMIT sqlc.arg(batch_limit)
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: CompleteDirectionJob :exec
UPDATE number_direction_jobs
SET status = 'done',
    locked_at = NULL,
    locked_by = NULL,
    last_error = NULL,
    updated_at = now()
WHERE id = sqlc.arg(id);

-- name: RetryDirectionJob :exec
UPDATE number_direction_jobs
SET status = 'retry',
    attempt = sqlc.arg(attempt),
    available_at = sqlc.arg(available_at),
    locked_at = NULL,
    locked_by = NULL,
    last_error = sqlc.arg(last_error),
    updated_at = now()
WHERE id = sqlc.arg(id);

-- name: DeadDirectionJob :exec
UPDATE number_direction_jobs
SET status = 'dead',
    attempt = sqlc.arg(attempt),
    locked_at = NULL,
    locked_by = NULL,
    last_error = sqlc.arg(last_error),
    updated_at = now()
WHERE id = sqlc.arg(id);

-- name: ReclaimStaleDirectionJobs :execrows
UPDATE number_direction_jobs
SET status = 'pending',
    locked_at = NULL,
    locked_by = NULL,
    updated_at = now()
WHERE status = 'processing'
  AND locked_at < sqlc.arg(stale_before);
