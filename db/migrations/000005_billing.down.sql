ALTER TABLE sms_messages
    DROP COLUMN IF EXISTS billing_action,
    DROP COLUMN IF EXISTS currency,
    DROP COLUMN IF EXISTS tariff_plan_code,
    DROP COLUMN IF EXISTS tariff_plan_id,
    DROP COLUMN IF EXISTS billed_segments,
    DROP COLUMN IF EXISTS unit_sell_price;

DROP TABLE IF EXISTS wallet_transactions;
DROP TABLE IF EXISTS client_tariffs;
DROP TABLE IF EXISTS tariff_plans;
DROP TABLE IF EXISTS wallets;

ALTER TABLE system_settings
    DROP COLUMN IF EXISTS low_balance_threshold,
    DROP COLUMN IF EXISTS billing_enforced;

DROP TYPE IF EXISTS billing_action;
DROP TYPE IF EXISTS wallet_tx_type;
DROP TYPE IF EXISTS billing_product;
DROP DOMAIN IF EXISTS billing_money;
