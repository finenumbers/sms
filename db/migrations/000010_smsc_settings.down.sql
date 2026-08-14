ALTER TABLE system_settings
    DROP COLUMN IF EXISTS smsc_base_url,
    DROP COLUMN IF EXISTS smsc_login,
    DROP COLUMN IF EXISTS smsc_password_ciphertext,
    DROP COLUMN IF EXISTS smsc_apikey_ciphertext,
    DROP COLUMN IF EXISTS smsc_callback_secret_ciphertext,
    DROP COLUMN IF EXISTS smsc_currency;
