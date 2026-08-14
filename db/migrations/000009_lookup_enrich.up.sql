ALTER TABLE lookup_items
    ADD COLUMN enrich_attempts int NOT NULL DEFAULT 0;

CREATE INDEX lookup_items_hlr_enrich_idx
    ON lookup_items (updated_at, id)
    WHERE check_type = 'hlr'
      AND status IN ('completed', 'failed');
