-- SMSC credentials live in system_settings (encrypted), not env.
-- Adapter reads these columns; the old HLR smscBaseUrl field was unused.

ALTER TABLE system_settings
    ADD COLUMN smsc_base_url text,
    ADD COLUMN smsc_login text,
    ADD COLUMN smsc_password_ciphertext bytea,
    ADD COLUMN smsc_apikey_ciphertext bytea,
    ADD COLUMN smsc_callback_secret_ciphertext bytea,
    ADD COLUMN smsc_currency text NOT NULL DEFAULT 'RUB';
