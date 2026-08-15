ALTER TABLE lookup_jobs
    ADD COLUMN reachable_count int NOT NULL DEFAULT 0 CHECK (reachable_count >= 0),
    ADD COLUMN unreachable_count int NOT NULL DEFAULT 0 CHECK (unreachable_count >= 0);

UPDATE lookup_jobs j
SET reachable_count = (
        SELECT count(*)::int FROM lookup_items i
        WHERE i.job_id = j.id AND i.result_status = 'reachable'
    ),
    unreachable_count = (
        SELECT count(*)::int FROM lookup_items i
        WHERE i.job_id = j.id AND i.result_status = 'unreachable'
    );
