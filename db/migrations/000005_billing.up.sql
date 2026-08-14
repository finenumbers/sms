-- Client prepaid billing (sell price only; no provider cost).
-- See docs/architecture/DATA_MODEL.md and HLR packages/billing invariants.

CREATE DOMAIN billing_money AS numeric(18, 6);

CREATE TYPE billing_product AS ENUM ('sms_domestic', 'sms_international');
CREATE TYPE wallet_tx_type AS ENUM ('CREDIT', 'HOLD', 'DEBIT', 'RELEASE', 'ADJUSTMENT');
CREATE TYPE billing_action AS ENUM ('capture', 'release');

ALTER TABLE system_settings
    ADD COLUMN billing_enforced boolean NOT NULL DEFAULT false,
    ADD COLUMN low_balance_threshold billing_money NOT NULL DEFAULT 100;

CREATE TABLE wallets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id uuid NOT NULL UNIQUE REFERENCES clients (id),
    currency char(3) NOT NULL DEFAULT 'RUB',
    available_balance billing_money NOT NULL DEFAULT 0,
    held_balance billing_money NOT NULL DEFAULT 0 CHECK (held_balance >= 0),
    version int NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE tariff_plans (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code text NOT NULL UNIQUE,
    name text NOT NULL,
    product billing_product NOT NULL,
    sell_price billing_money NOT NULL CHECK (sell_price > 0),
    currency char(3) NOT NULL DEFAULT 'RUB',
    is_default boolean NOT NULL DEFAULT false,
    is_active boolean NOT NULL DEFAULT true,
    description text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX tariff_plans_product_active_idx ON tariff_plans (product, is_active);

CREATE TABLE client_tariffs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id uuid NOT NULL REFERENCES clients (id),
    product billing_product NOT NULL,
    tariff_plan_id uuid NOT NULL REFERENCES tariff_plans (id) ON DELETE RESTRICT,
    price_override billing_money CHECK (price_override IS NULL OR price_override > 0),
    effective_from timestamptz NOT NULL DEFAULT now(),
    effective_to timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (client_id, product)
);

CREATE INDEX client_tariffs_plan_idx ON client_tariffs (tariff_plan_id);

CREATE TABLE wallet_transactions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    wallet_id uuid NOT NULL REFERENCES wallets (id),
    client_id uuid NOT NULL REFERENCES clients (id),
    type wallet_tx_type NOT NULL,
    amount billing_money NOT NULL CHECK (amount >= 0),
    currency char(3) NOT NULL DEFAULT 'RUB',
    balance_after_available billing_money,
    balance_after_held billing_money,
    related_hold_id uuid REFERENCES wallet_transactions (id) ON DELETE SET NULL,
    sms_message_id uuid REFERENCES sms_messages (id) ON DELETE SET NULL,
    idempotency_key text,
    description text,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_by uuid REFERENCES admin_users (id),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX wallet_transactions_idempotency_idx
    ON wallet_transactions (client_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
CREATE INDEX wallet_transactions_wallet_created_idx ON wallet_transactions (wallet_id, created_at DESC);
CREATE INDEX wallet_transactions_client_created_idx ON wallet_transactions (client_id, created_at DESC);
CREATE INDEX wallet_transactions_type_created_idx ON wallet_transactions (type, created_at DESC);
CREATE INDEX wallet_transactions_sms_message_idx ON wallet_transactions (sms_message_id);
CREATE INDEX wallet_transactions_related_hold_idx ON wallet_transactions (related_hold_id);
CREATE INDEX wallet_transactions_created_idx ON wallet_transactions (created_at DESC);

ALTER TABLE sms_messages
    ADD COLUMN unit_sell_price billing_money,
    ADD COLUMN billed_segments int CHECK (billed_segments IS NULL OR billed_segments >= 1),
    ADD COLUMN tariff_plan_id uuid REFERENCES tariff_plans (id) ON DELETE SET NULL,
    ADD COLUMN tariff_plan_code text,
    ADD COLUMN currency char(3),
    ADD COLUMN billing_action billing_action;

CREATE INDEX sms_messages_billing_open_hold_idx ON sms_messages (id)
    WHERE direction = 'outbound' AND billing_action IS NULL AND unit_sell_price IS NOT NULL;

INSERT INTO wallets (client_id)
SELECT id FROM clients
ON CONFLICT (client_id) DO NOTHING;

INSERT INTO tariff_plans (code, name, product, sell_price, currency, is_default, is_active, description)
VALUES
    ('sms-domestic', 'SMS на номера 7…', 'sms_domestic', 3.5, 'RUB', true, true,
     'Цена за 1 PDU. Номера 7XXXXXXXXXX, как в направлениях платформы (включая 77…).'),
    ('sms-international', 'SMS международный', 'sms_international', 8, 'RUB', true, true,
     'Цена за 1 PDU на номера не 7XXXXXXXXXX.')
ON CONFLICT (code) DO NOTHING;
