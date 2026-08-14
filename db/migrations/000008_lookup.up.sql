-- HLR / Silent SMS domain + convergent wallet FKs + lookup settings.
-- Sell price only: no provider cost on tariff_plans.

CREATE TYPE lookup_check_type AS ENUM ('hlr', 'ping');
CREATE TYPE lookup_job_source AS ENUM ('single', 'bulk', 'api');
CREATE TYPE lookup_job_status AS ENUM (
    'queued',
    'processing',
    'completed',
    'completed_with_errors',
    'failed'
);
CREATE TYPE lookup_item_status AS ENUM (
    'queued',
    'reserved',
    'pending',
    'completed',
    'failed'
);
CREATE TYPE lookup_csv_preview_status AS ENUM (
    'ready',
    'invalid',
    'consuming',
    'consumed',
    'expired'
);
CREATE TYPE provider_lookup_kind AS ENUM ('send', 'status', 'cost', 'balance');
CREATE TYPE provider_lookup_request_status AS ENUM ('pending', 'succeeded', 'failed');
CREATE TYPE webhook_delivery_status AS ENUM ('pending', 'delivered', 'failed', 'dead');

ALTER TABLE system_settings
    ADD COLUMN lookup_enabled boolean NOT NULL DEFAULT false,
    ADD COLUMN lookup_check_timeout_sec int NOT NULL DEFAULT 3600
        CHECK (lookup_check_timeout_sec >= 30),
    ADD COLUMN lookup_poll_interval_sec int NOT NULL DEFAULT 30
        CHECK (lookup_poll_interval_sec >= 1),
    ADD COLUMN lookup_max_csv_rows int NOT NULL DEFAULT 100000
        CHECK (lookup_max_csv_rows >= 1),
    ADD COLUMN lookup_max_csv_bytes int NOT NULL DEFAULT 52428800
        CHECK (lookup_max_csv_bytes >= 1024),
    ADD COLUMN lookup_max_batch_phones int NOT NULL DEFAULT 1000
        CHECK (lookup_max_batch_phones >= 1),
    ADD COLUMN lookup_webhook_max_attempts int NOT NULL DEFAULT 8
        CHECK (lookup_webhook_max_attempts >= 1),
    ADD COLUMN lookup_webhook_timeout_ms int NOT NULL DEFAULT 5000
        CHECK (lookup_webhook_timeout_ms >= 100),
    ADD COLUMN lookup_retention_days int NOT NULL DEFAULT 90
        CHECK (lookup_retention_days >= 1);

CREATE TABLE lookup_jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id uuid NOT NULL REFERENCES clients (id),
    check_type lookup_check_type NOT NULL,
    source lookup_job_source NOT NULL,
    status lookup_job_status NOT NULL DEFAULT 'queued',
    item_count int NOT NULL DEFAULT 0 CHECK (item_count >= 0),
    success_count int NOT NULL DEFAULT 0 CHECK (success_count >= 0),
    failure_count int NOT NULL DEFAULT 0 CHECK (failure_count >= 0),
    unit_sell_price billing_money,
    tariff_plan_id uuid REFERENCES tariff_plans (id) ON DELETE SET NULL,
    tariff_plan_code text,
    currency char(3) NOT NULL DEFAULT 'RUB',
    estimated_cost billing_money,
    actual_cost billing_money,
    original_filename text,
    idempotency_key text,
    created_by uuid REFERENCES client_users (id) ON DELETE SET NULL,
    api_credential_id uuid REFERENCES api_credentials (id) ON DELETE SET NULL,
    error_code text,
    error_message text,
    started_at timestamptz,
    completed_at timestamptz,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX lookup_jobs_client_idempotency_idx
    ON lookup_jobs (client_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
CREATE INDEX lookup_jobs_client_created_idx ON lookup_jobs (client_id, created_at DESC);
CREATE INDEX lookup_jobs_client_status_idx ON lookup_jobs (client_id, status);
CREATE INDEX lookup_jobs_status_created_idx ON lookup_jobs (status, created_at DESC);

CREATE TABLE lookup_items (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id uuid NOT NULL REFERENCES lookup_jobs (id) ON DELETE CASCADE,
    client_id uuid NOT NULL REFERENCES clients (id),
    check_type lookup_check_type NOT NULL,
    status lookup_item_status NOT NULL DEFAULT 'queued',
    phone_e164 text NOT NULL,
    phone_digits text NOT NULL,
    provider_code text NOT NULL DEFAULT 'smsc',
    provider_message_id text,
    unit_sell_price billing_money,
    tariff_plan_id uuid REFERENCES tariff_plans (id) ON DELETE SET NULL,
    tariff_plan_code text,
    currency char(3),
    estimated_cost billing_money,
    actual_cost billing_money,
    result_status text,
    is_reachable boolean,
    imsi text,
    mcc text,
    mnc text,
    operator_name text,
    country_code text,
    ported boolean,
    roaming boolean,
    normalized_result jsonb NOT NULL DEFAULT '{}'::jsonb,
    error_code text,
    error_message text,
    billing_action billing_action,
    next_poll_at timestamptz,
    poll_attempts int NOT NULL DEFAULT 0,
    sent_at timestamptz,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX lookup_items_job_status_idx ON lookup_items (job_id, status);
CREATE INDEX lookup_items_client_created_idx ON lookup_items (client_id, created_at DESC);
CREATE INDEX lookup_items_status_poll_idx ON lookup_items (status, next_poll_at)
    WHERE status = 'pending';
CREATE INDEX lookup_items_status_updated_idx ON lookup_items (status, updated_at);
CREATE INDEX lookup_items_phone_digits_idx ON lookup_items (phone_digits);
CREATE UNIQUE INDEX lookup_items_provider_msg_idx
    ON lookup_items (provider_code, provider_message_id)
    WHERE provider_message_id IS NOT NULL;

CREATE TABLE lookup_csv_previews (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id uuid NOT NULL REFERENCES clients (id),
    check_type lookup_check_type NOT NULL,
    status lookup_csv_preview_status NOT NULL DEFAULT 'ready',
    original_filename text,
    phone_count int NOT NULL DEFAULT 0,
    phones_json jsonb,
    error_message text,
    job_id uuid REFERENCES lookup_jobs (id) ON DELETE SET NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX lookup_csv_previews_client_idx ON lookup_csv_previews (client_id, created_at DESC);
CREATE INDEX lookup_csv_previews_expires_idx ON lookup_csv_previews (status, expires_at);

CREATE TABLE provider_lookup_requests (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id uuid REFERENCES clients (id) ON DELETE SET NULL,
    job_item_id uuid REFERENCES lookup_items (id) ON DELETE SET NULL,
    provider_code text NOT NULL DEFAULT 'smsc',
    kind provider_lookup_kind NOT NULL,
    status provider_lookup_request_status NOT NULL DEFAULT 'pending',
    provider_message_id text,
    http_status int,
    error_code text,
    error_message text,
    request_payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    response_payload jsonb,
    normalized_result jsonb,
    idempotency_key text,
    started_at timestamptz,
    finished_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX provider_lookup_requests_item_idx ON provider_lookup_requests (job_item_id, created_at DESC);
CREATE INDEX provider_lookup_requests_kind_idx ON provider_lookup_requests (kind, created_at DESC);
CREATE UNIQUE INDEX provider_lookup_requests_active_idem_idx
    ON provider_lookup_requests (provider_code, idempotency_key)
    WHERE idempotency_key IS NOT NULL AND status IN ('pending', 'succeeded');

CREATE TABLE provider_lookup_callbacks (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id uuid REFERENCES clients (id) ON DELETE SET NULL,
    job_item_id uuid REFERENCES lookup_items (id) ON DELETE SET NULL,
    provider_code text NOT NULL DEFAULT 'smsc',
    provider_message_id text,
    raw_payload jsonb NOT NULL,
    normalized_result jsonb,
    dedupe_key text,
    signature_valid boolean,
    processed_at timestamptz,
    process_error text,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX provider_lookup_callbacks_dedupe_idx
    ON provider_lookup_callbacks (provider_code, dedupe_key)
    WHERE dedupe_key IS NOT NULL;
CREATE INDEX provider_lookup_callbacks_item_idx ON provider_lookup_callbacks (job_item_id);
CREATE INDEX provider_lookup_callbacks_created_idx ON provider_lookup_callbacks (created_at DESC);

CREATE TABLE webhook_endpoints (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id uuid NOT NULL REFERENCES clients (id) ON DELETE CASCADE,
    url text NOT NULL,
    secret_ciphertext bytea NOT NULL,
    dek_key_id text NOT NULL,
    description text,
    enabled boolean NOT NULL DEFAULT true,
    events text[] NOT NULL DEFAULT '{}'::text[],
    consecutive_failures int NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX webhook_endpoints_client_idx ON webhook_endpoints (client_id, enabled);

CREATE TABLE webhook_deliveries (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id uuid NOT NULL REFERENCES clients (id) ON DELETE CASCADE,
    endpoint_id uuid NOT NULL REFERENCES webhook_endpoints (id) ON DELETE CASCADE,
    job_id uuid REFERENCES lookup_jobs (id) ON DELETE SET NULL,
    job_item_id uuid REFERENCES lookup_items (id) ON DELETE SET NULL,
    event_type text NOT NULL,
    payload jsonb NOT NULL,
    status webhook_delivery_status NOT NULL DEFAULT 'pending',
    attempt_count int NOT NULL DEFAULT 0,
    max_attempts int NOT NULL DEFAULT 8,
    next_attempt_at timestamptz,
    last_response_code int,
    last_error text,
    delivered_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX webhook_deliveries_claim_idx
    ON webhook_deliveries (status, next_attempt_at);
CREATE INDEX webhook_deliveries_client_idx ON webhook_deliveries (client_id, created_at DESC);
CREATE INDEX webhook_deliveries_endpoint_idx ON webhook_deliveries (endpoint_id, status);

ALTER TABLE wallet_transactions
    ADD COLUMN lookup_job_id uuid REFERENCES lookup_jobs (id) ON DELETE RESTRICT,
    ADD COLUMN lookup_item_id uuid REFERENCES lookup_items (id) ON DELETE RESTRICT;

CREATE INDEX wallet_transactions_lookup_job_idx ON wallet_transactions (lookup_job_id);
CREATE INDEX wallet_transactions_lookup_item_idx ON wallet_transactions (lookup_item_id);

INSERT INTO tariff_plans (code, name, product, sell_price, currency, is_default, is_active, description)
VALUES
    ('hlr-default', 'HLR Lookup', 'hlr', 1.50, 'RUB', true, true,
     'Цена за 1 проверку HLR. Назначается клиенту вручную.'),
    ('silent-sms-default', 'Silent SMS', 'silent_sms', 2.00, 'RUB', true, true,
     'Цена за 1 проверку Silent SMS (ping). Назначается клиенту вручную.')
ON CONFLICT (code) DO NOTHING;
