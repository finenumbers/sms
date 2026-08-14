-- Wave 9: retention scans + unprocessed callback claim.

CREATE INDEX IF NOT EXISTS sms_messages_created_idx ON sms_messages (created_at);
CREATE INDEX IF NOT EXISTS provider_callback_events_unprocessed_idx
    ON provider_callback_events (created_at)
    WHERE processed_at IS NULL;
CREATE INDEX IF NOT EXISTS provider_callback_events_created_idx
    ON provider_callback_events (created_at);
CREATE INDEX IF NOT EXISTS idempotency_keys_expires_idx ON idempotency_keys (expires_at);
