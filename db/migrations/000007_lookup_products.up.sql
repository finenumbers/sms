-- New billing products for HLR / Silent SMS.
-- Must be a separate file from any INSERT/CHECK that uses the new values
-- (golang-migrate wraps each file in a transaction; PG sees ADD VALUE after commit).

ALTER TYPE billing_product ADD VALUE IF NOT EXISTS 'hlr';
ALTER TYPE billing_product ADD VALUE IF NOT EXISTS 'silent_sms';
