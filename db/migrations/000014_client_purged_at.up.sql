ALTER TABLE clients
    ADD COLUMN purged_at timestamptz;

CREATE INDEX clients_pending_purge_idx
    ON clients (updated_at)
    WHERE status = 'deleted' AND purged_at IS NULL;
