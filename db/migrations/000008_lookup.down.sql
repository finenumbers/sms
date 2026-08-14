ALTER TABLE wallet_transactions
    DROP COLUMN IF EXISTS lookup_item_id,
    DROP COLUMN IF EXISTS lookup_job_id;

DROP TABLE IF EXISTS webhook_deliveries;
DROP TABLE IF EXISTS webhook_endpoints;
DROP TABLE IF EXISTS provider_lookup_callbacks;
DROP TABLE IF EXISTS provider_lookup_requests;
DROP TABLE IF EXISTS lookup_csv_previews;
DROP TABLE IF EXISTS lookup_items;
DROP TABLE IF EXISTS lookup_jobs;

ALTER TABLE system_settings
    DROP COLUMN IF EXISTS lookup_enabled,
    DROP COLUMN IF EXISTS lookup_check_timeout_sec,
    DROP COLUMN IF EXISTS lookup_poll_interval_sec,
    DROP COLUMN IF EXISTS lookup_max_csv_rows,
    DROP COLUMN IF EXISTS lookup_max_csv_bytes,
    DROP COLUMN IF EXISTS lookup_max_batch_phones,
    DROP COLUMN IF EXISTS lookup_webhook_max_attempts,
    DROP COLUMN IF EXISTS lookup_webhook_timeout_ms,
    DROP COLUMN IF EXISTS lookup_retention_days;

DELETE FROM tariff_plans WHERE code IN ('hlr-default', 'silent-sms-default');

DROP TYPE IF EXISTS webhook_delivery_status;
DROP TYPE IF EXISTS provider_lookup_request_status;
DROP TYPE IF EXISTS provider_lookup_kind;
DROP TYPE IF EXISTS lookup_csv_preview_status;
DROP TYPE IF EXISTS lookup_item_status;
DROP TYPE IF EXISTS lookup_job_status;
DROP TYPE IF EXISTS lookup_job_source;
DROP TYPE IF EXISTS lookup_check_type;
