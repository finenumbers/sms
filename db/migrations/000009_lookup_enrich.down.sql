DROP INDEX IF EXISTS lookup_items_hlr_enrich_idx;

ALTER TABLE lookup_items
    DROP COLUMN IF EXISTS enrich_attempts;
