-- Core schema for Finenumbers SMS Service v1.
-- See docs/architecture/DATA_MODEL.md

CREATE TYPE user_status AS ENUM ('active', 'disabled');
CREATE TYPE client_status AS ENUM ('active', 'suspended', 'deleted');
CREATE TYPE client_user_role AS ENUM ('owner');
CREATE TYPE def_number_status AS ENUM ('inventory', 'assigned', 'disabled');
CREATE TYPE sms_direction AS ENUM ('outbound', 'inbound');
CREATE TYPE sms_status AS ENUM ('queued', 'accepted', 'sent', 'delivered', 'failed');
CREATE TYPE campaign_status AS ENUM ('draft', 'queued', 'running', 'completed', 'failed', 'cancelled');
CREATE TYPE campaign_recipient_status AS ENUM ('pending', 'enqueued', 'skipped', 'failed');
CREATE TYPE send_job_status AS ENUM ('pending', 'processing', 'done', 'retry', 'uncertain', 'dead');
CREATE TYPE callback_kind AS ENUM ('dlr', 'mo');
CREATE TYPE actor_type AS ENUM ('admin', 'client_user', 'api_key', 'system');
CREATE TYPE session_audience AS ENUM ('admin', 'client');
CREATE TYPE credential_status AS ENUM ('active', 'revoked');
CREATE TYPE send_attempt_kind AS ENUM ('accepted', 'rejected_4xx', 'rate_limited', 'timeout', '5xx');

CREATE TABLE admin_users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email text NOT NULL,
    password_hash text NOT NULL,
    name text NOT NULL DEFAULT '',
    status user_status NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX admin_users_email_lower_idx ON admin_users (LOWER(email));

CREATE TABLE clients (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL,
    status client_status NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE client_users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id uuid NOT NULL REFERENCES clients (id),
    email text NOT NULL,
    password_hash text NOT NULL,
    role client_user_role NOT NULL DEFAULT 'owner',
    status user_status NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX client_users_email_lower_idx ON client_users (LOWER(email));
CREATE INDEX client_users_client_id_idx ON client_users (client_id);

CREATE TABLE api_credentials (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id uuid NOT NULL REFERENCES clients (id),
    name text NOT NULL,
    key_prefix text NOT NULL,
    secret_hash text NOT NULL,
    scopes text[] NOT NULL DEFAULT ARRAY['sms:send', 'sms:read', 'campaigns:write']::text[],
    status credential_status NOT NULL DEFAULT 'active',
    allowed_cidrs inet[] NOT NULL DEFAULT ARRAY[]::inet[],
    last_used_at timestamptz,
    created_by uuid REFERENCES admin_users (id),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX api_credentials_key_prefix_idx ON api_credentials (key_prefix);
CREATE INDEX api_credentials_client_id_idx ON api_credentials (client_id);

CREATE TABLE sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    audience session_audience NOT NULL,
    admin_user_id uuid REFERENCES admin_users (id) ON DELETE CASCADE,
    client_user_id uuid REFERENCES client_users (id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    ip inet,
    user_agent text,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT sessions_principal_chk CHECK (
        (audience = 'admin' AND admin_user_id IS NOT NULL AND client_user_id IS NULL)
        OR (audience = 'client' AND client_user_id IS NOT NULL AND admin_user_id IS NULL)
    )
);

CREATE INDEX sessions_admin_user_id_idx ON sessions (admin_user_id) WHERE revoked_at IS NULL;
CREATE INDEX sessions_client_user_id_idx ON sessions (client_user_id) WHERE revoked_at IS NULL;
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at) WHERE revoked_at IS NULL;

CREATE TABLE def_numbers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    msisdn text NOT NULL,
    status def_number_status NOT NULL DEFAULT 'inventory',
    region text,
    notes text,
    supports_sms boolean NOT NULL DEFAULT true,
    runexis_snapshot jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT def_numbers_msisdn_chk CHECK (msisdn ~ '^7[0-9]{10}$')
);

CREATE UNIQUE INDEX def_numbers_msisdn_idx ON def_numbers (msisdn);

CREATE TABLE number_assignments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    def_number_id uuid NOT NULL REFERENCES def_numbers (id),
    client_id uuid NOT NULL REFERENCES clients (id),
    assigned_at timestamptz NOT NULL DEFAULT now(),
    unassigned_at timestamptz,
    assigned_by uuid REFERENCES admin_users (id)
);

CREATE UNIQUE INDEX number_assignments_open_idx
    ON number_assignments (def_number_id)
    WHERE unassigned_at IS NULL;
CREATE INDEX number_assignments_client_id_idx ON number_assignments (client_id);

CREATE TABLE sms_campaigns (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id uuid NOT NULL REFERENCES clients (id),
    from_msisdn text NOT NULL,
    text text NOT NULL,
    status campaign_status NOT NULL DEFAULT 'draft',
    total_count int NOT NULL DEFAULT 0,
    accepted_count int NOT NULL DEFAULT 0,
    delivered_count int NOT NULL DEFAULT 0,
    failed_count int NOT NULL DEFAULT 0,
    created_by uuid REFERENCES client_users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT sms_campaigns_from_msisdn_chk CHECK (from_msisdn ~ '^7[0-9]{10}$')
);

CREATE INDEX sms_campaigns_client_id_idx ON sms_campaigns (client_id, created_at DESC);

CREATE TABLE sms_messages (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id uuid REFERENCES clients (id),
    direction sms_direction NOT NULL,
    from_msisdn text NOT NULL,
    to_msisdn text NOT NULL,
    text text NOT NULL,
    provider text NOT NULL DEFAULT 'runexis',
    provider_sms_id text,
    provider_status text,
    pdu_count int,
    campaign_id uuid REFERENCES sms_campaigns (id),
    status sms_status NOT NULL DEFAULT 'queued',
    idempotency_key text,
    created_at timestamptz NOT NULL DEFAULT now(),
    accepted_at timestamptz,
    sent_at timestamptz,
    delivered_at timestamptz,
    failed_at timestamptz,
    CONSTRAINT sms_messages_to_msisdn_chk CHECK (to_msisdn ~ '^[0-9]{8,15}$')
);

CREATE INDEX sms_messages_client_created_idx ON sms_messages (client_id, created_at DESC);
CREATE INDEX sms_messages_client_direction_idx ON sms_messages (client_id, direction, created_at DESC);
CREATE UNIQUE INDEX sms_messages_provider_sms_id_idx
    ON sms_messages (provider_sms_id)
    WHERE provider_sms_id IS NOT NULL;
CREATE UNIQUE INDEX sms_messages_idempotency_idx
    ON sms_messages (client_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
CREATE INDEX sms_messages_campaign_id_idx ON sms_messages (campaign_id);

CREATE TABLE campaign_recipients (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    campaign_id uuid NOT NULL REFERENCES sms_campaigns (id) ON DELETE CASCADE,
    to_msisdn text NOT NULL,
    status campaign_recipient_status NOT NULL DEFAULT 'pending',
    sms_message_id uuid REFERENCES sms_messages (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT campaign_recipients_to_msisdn_chk CHECK (to_msisdn ~ '^[0-9]{8,15}$')
);

CREATE UNIQUE INDEX campaign_recipients_unique_idx ON campaign_recipients (campaign_id, to_msisdn);
CREATE INDEX campaign_recipients_status_idx ON campaign_recipients (campaign_id, status);

CREATE TABLE send_jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    sms_message_id uuid NOT NULL REFERENCES sms_messages (id),
    client_id uuid REFERENCES clients (id),
    status send_job_status NOT NULL DEFAULT 'pending',
    attempt int NOT NULL DEFAULT 0,
    available_at timestamptz NOT NULL DEFAULT now(),
    locked_at timestamptz,
    locked_by text,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX send_jobs_sms_message_id_idx ON send_jobs (sms_message_id);
CREATE INDEX send_jobs_claim_idx
    ON send_jobs (available_at)
    WHERE status IN ('pending', 'retry', 'uncertain');
CREATE INDEX send_jobs_client_claim_idx
    ON send_jobs (client_id, available_at)
    WHERE status IN ('pending', 'retry', 'uncertain');

CREATE TABLE provider_send_attempts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    send_job_id uuid NOT NULL REFERENCES send_jobs (id),
    attempt int NOT NULL,
    request_meta jsonb,
    http_status int,
    response_body text,
    latency_ms int,
    error_kind send_attempt_kind,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX provider_send_attempts_job_idx ON provider_send_attempts (send_job_id, created_at);

CREATE TABLE provider_callback_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind callback_kind NOT NULL,
    idempotency_key text NOT NULL,
    method text NOT NULL,
    path text NOT NULL,
    query text,
    headers jsonb,
    raw_body bytea,
    content_type text,
    parsed jsonb,
    sms_message_id uuid REFERENCES sms_messages (id),
    processed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX provider_callback_events_idempotency_idx ON provider_callback_events (idempotency_key);

CREATE TABLE system_settings (
    id smallint PRIMARY KEY CHECK (id = 1),
    runexis_email text,
    runexis_password_ciphertext bytea,
    dek_key_id text,
    callback_base_url text,
    sms_dir_in boolean NOT NULL DEFAULT true,
    sms_dir_dom_out boolean NOT NULL DEFAULT true,
    sms_dir_int_out boolean NOT NULL DEFAULT false,
    sms_dir_in_mass boolean NOT NULL DEFAULT false,
    provider_rps numeric(6, 2) NOT NULL DEFAULT 20,
    client_rps_default numeric(6, 2) NOT NULL DEFAULT 5,
    retention_days int NOT NULL DEFAULT 365,
    audit_retention_days int NOT NULL DEFAULT 730,
    ingress_token_hash text,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid REFERENCES admin_users (id)
);

INSERT INTO system_settings (id) VALUES (1);

CREATE TABLE audit_log (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_type actor_type NOT NULL,
    actor_id uuid,
    client_id uuid REFERENCES clients (id),
    action text NOT NULL,
    resource_type text NOT NULL,
    resource_id uuid,
    ip inet,
    user_agent text,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_log_created_idx ON audit_log (created_at DESC);
CREATE INDEX audit_log_client_idx ON audit_log (client_id, created_at DESC);
CREATE INDEX audit_log_resource_idx ON audit_log (resource_type, resource_id);

CREATE TABLE idempotency_keys (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    principal_type actor_type NOT NULL,
    principal_id uuid NOT NULL,
    key text NOT NULL,
    request_hash text NOT NULL,
    response_status int,
    response_body jsonb,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idempotency_keys_principal_key_idx ON idempotency_keys (principal_type, principal_id, key);
CREATE INDEX idempotency_keys_expires_idx ON idempotency_keys (expires_at);
