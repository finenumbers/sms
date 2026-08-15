ALTER TABLE lookup_jobs
    DROP COLUMN IF EXISTS reachable_count,
    DROP COLUMN IF EXISTS unreachable_count;
