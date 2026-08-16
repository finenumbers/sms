DROP INDEX IF EXISTS clients_pending_purge_idx;
ALTER TABLE clients DROP COLUMN IF EXISTS purged_at;
