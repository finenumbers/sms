-- name: GetSystemSettings :one
SELECT * FROM system_settings WHERE id = 1;

-- name: UpdateSystemSettings :one
UPDATE system_settings
SET
    runexis_email = $1,
    runexis_password_ciphertext = $2,
    dek_key_id = $3,
    callback_base_url = $4,
    sms_dir_in = $5,
    sms_dir_dom_out = $6,
    sms_dir_int_out = $7,
    sms_dir_in_mass = $8,
    provider_rps = $9,
    client_rps_default = $10,
    retention_days = $11,
    audit_retention_days = $12,
    ops_retention_days = $13,
    ingress_token_hash = $14,
    billing_enforced = $15,
    low_balance_threshold = $16,
    lookup_enabled = $17,
    lookup_check_timeout_sec = $18,
    lookup_poll_interval_sec = $19,
    lookup_max_csv_rows = $20,
    lookup_max_csv_bytes = $21,
    lookup_max_batch_phones = $22,
    lookup_webhook_max_attempts = $23,
    lookup_webhook_timeout_ms = $24,
    lookup_retention_days = $25,
    smsc_base_url = $26,
    smsc_login = $27,
    smsc_password_ciphertext = $28,
    smsc_apikey_ciphertext = $29,
    smsc_callback_secret_ciphertext = $30,
    smsc_currency = $31,
    updated_by = $32,
    updated_at = now()
WHERE id = 1
RETURNING *;
