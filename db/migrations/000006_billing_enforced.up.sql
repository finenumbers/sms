-- Strict billing is the only mode: no grandfather / migration toggle.
ALTER TABLE system_settings
    ALTER COLUMN billing_enforced SET DEFAULT true;

UPDATE system_settings SET billing_enforced = true WHERE id = 1;
