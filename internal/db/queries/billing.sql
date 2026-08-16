-- name: InsertWallet :one
INSERT INTO wallets (client_id, currency)
VALUES (sqlc.arg(client_id), sqlc.arg(currency))
ON CONFLICT (client_id) DO UPDATE SET client_id = wallets.client_id
RETURNING *;

-- name: GetWalletByClientID :one
SELECT * FROM wallets WHERE client_id = sqlc.arg(client_id);

-- name: LockWalletByClientID :one
SELECT * FROM wallets WHERE client_id = sqlc.arg(client_id) FOR UPDATE;

-- name: UpdateWalletBalances :one
UPDATE wallets
SET
    available_balance = sqlc.arg(available_balance),
    held_balance = sqlc.arg(held_balance),
    version = version + 1,
    updated_at = now()
WHERE id = sqlc.arg(id) AND version = sqlc.arg(version)
RETURNING *;

-- name: InsertWalletTransaction :one
INSERT INTO wallet_transactions (
    wallet_id, client_id, type, amount, currency,
    balance_after_available, balance_after_held,
    related_hold_id, sms_message_id, lookup_job_id, lookup_item_id, idempotency_key,
    description, metadata, created_by
) VALUES (
    sqlc.arg(wallet_id), sqlc.arg(client_id), sqlc.arg(type), sqlc.arg(amount), sqlc.arg(currency),
    sqlc.arg(balance_after_available), sqlc.arg(balance_after_held),
    sqlc.narg(related_hold_id), sqlc.narg(sms_message_id), sqlc.narg(lookup_job_id), sqlc.narg(lookup_item_id),
    sqlc.narg(idempotency_key),
    sqlc.narg(description), sqlc.arg(metadata), sqlc.narg(created_by)
)
RETURNING *;

-- name: GetWalletTxByIdempotency :one
SELECT * FROM wallet_transactions
WHERE client_id = sqlc.arg(client_id)
  AND idempotency_key = sqlc.arg(idempotency_key);

-- name: GetOpenHoldForMessage :one
SELECT h.*
FROM wallet_transactions h
WHERE h.sms_message_id = sqlc.arg(sms_message_id)
  AND h.type = 'HOLD'
  AND NOT EXISTS (
      SELECT 1 FROM wallet_transactions s
      WHERE s.related_hold_id = h.id
        AND s.type IN ('DEBIT', 'RELEASE')
  )
ORDER BY h.created_at
LIMIT 1;

-- name: ListWalletTransactionsForClient :many
SELECT * FROM wallet_transactions
WHERE client_id = sqlc.arg(client_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: ListPlatformLedger :many
SELECT
    t.*,
    c.name AS client_name
FROM wallet_transactions t
JOIN clients c ON c.id = t.client_id AND c.status <> 'deleted'
WHERE (sqlc.narg(client_id)::uuid IS NULL OR t.client_id = sqlc.narg(client_id))
  AND (sqlc.narg(tx_type)::wallet_tx_type IS NULL OR t.type = sqlc.narg(tx_type))
ORDER BY t.created_at DESC, t.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: SumWalletBalances :one
SELECT
    COALESCE(sum(available_balance), 0)::billing_money AS available_total,
    COALESCE(sum(held_balance), 0)::billing_money AS held_total
FROM wallets w
JOIN clients c ON c.id = w.client_id AND c.status = 'active';

-- name: CountLowBalanceClients :one
SELECT count(*)::bigint AS n
FROM wallets w
JOIN clients c ON c.id = w.client_id
WHERE c.status = 'active'
  AND w.available_balance < sqlc.arg(threshold);

-- name: SumDebitsSince :one
SELECT COALESCE(sum(amount), 0)::billing_money AS spent
FROM wallet_transactions
WHERE type = 'DEBIT'
  AND created_at >= sqlc.arg(since);

-- name: SumDebitsSinceForClient :one
SELECT COALESCE(sum(amount), 0)::billing_money AS spent
FROM wallet_transactions
WHERE type = 'DEBIT'
  AND client_id = sqlc.arg(client_id)
  AND created_at >= sqlc.arg(since);

-- name: CountOpenHolds :one
-- SMS metric only. Lookup job HOLDs have sms_message_id IS NULL and a
-- separate CountOpenLookupHolds (remaining = hold − SUM(children)).
SELECT count(*)::bigint AS n
FROM wallet_transactions h
WHERE h.type = 'HOLD'
  AND h.sms_message_id IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM wallet_transactions s
      WHERE s.related_hold_id = h.id
        AND s.type IN ('DEBIT', 'RELEASE')
  );

-- name: ListOpenHoldMessages :many
SELECT
    h.id AS hold_id,
    h.client_id,
    h.sms_message_id,
    h.amount,
    m.status AS message_status,
    m.accepted_at,
    m.billing_action,
    j.status AS job_status
FROM wallet_transactions h
JOIN sms_messages m ON m.id = h.sms_message_id
LEFT JOIN send_jobs j ON j.sms_message_id = m.id
WHERE h.type = 'HOLD'
  AND NOT EXISTS (
      SELECT 1 FROM wallet_transactions s
      WHERE s.related_hold_id = h.id
        AND s.type IN ('DEBIT', 'RELEASE')
  )
ORDER BY h.created_at
LIMIT sqlc.arg(page_limit);

-- name: ListTariffPlans :many
SELECT * FROM tariff_plans
ORDER BY product, code
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: GetTariffPlanByID :one
SELECT * FROM tariff_plans WHERE id = sqlc.arg(id);

-- name: InsertTariffPlan :one
INSERT INTO tariff_plans (code, name, product, sell_price, currency, is_default, is_active, description)
VALUES (
    sqlc.arg(code), sqlc.arg(name), sqlc.arg(product), sqlc.arg(sell_price),
    sqlc.arg(currency), sqlc.arg(is_default), sqlc.arg(is_active), sqlc.narg(description)
)
RETURNING *;

-- name: UpdateTariffPlan :one
UPDATE tariff_plans
SET
    name = sqlc.arg(name),
    sell_price = sqlc.arg(sell_price),
    is_default = sqlc.arg(is_default),
    is_active = sqlc.arg(is_active),
    description = sqlc.narg(description),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: GetClientTariff :one
SELECT
    a.id,
    a.client_id,
    a.product,
    a.tariff_plan_id,
    a.price_override,
    a.effective_from,
    a.effective_to,
    p.code AS plan_code,
    p.name AS plan_name,
    p.sell_price AS plan_sell_price,
    p.currency,
    p.is_active AS plan_is_active
FROM client_tariffs a
JOIN tariff_plans p ON p.id = a.tariff_plan_id
WHERE a.client_id = sqlc.arg(client_id)
  AND a.product = sqlc.arg(product);

-- name: ListClientTariffs :many
SELECT
    a.id,
    a.client_id,
    a.product,
    a.tariff_plan_id,
    a.price_override,
    a.effective_from,
    a.effective_to,
    p.code AS plan_code,
    p.name AS plan_name,
    p.sell_price AS plan_sell_price,
    p.currency,
    p.is_active AS plan_is_active
FROM client_tariffs a
JOIN tariff_plans p ON p.id = a.tariff_plan_id
WHERE a.client_id = sqlc.arg(client_id)
ORDER BY a.product;

-- name: UpsertClientTariff :one
INSERT INTO client_tariffs (client_id, product, tariff_plan_id, price_override, effective_from, effective_to)
VALUES (
    sqlc.arg(client_id), sqlc.arg(product), sqlc.arg(tariff_plan_id),
    sqlc.narg(price_override), now(), sqlc.narg(effective_to)
)
ON CONFLICT (client_id, product) DO UPDATE SET
    tariff_plan_id = EXCLUDED.tariff_plan_id,
    price_override = EXCLUDED.price_override,
    effective_from = now(),
    effective_to = EXCLUDED.effective_to,
    updated_at = now()
RETURNING *;

-- name: DeleteClientTariff :execrows
DELETE FROM client_tariffs
WHERE client_id = sqlc.arg(client_id) AND product = sqlc.arg(product);

-- name: SetSmsMessageBillingAction :one
UPDATE sms_messages
SET billing_action = sqlc.arg(billing_action)
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: CountOutboundSmsSinceByStatus :many
SELECT status, count(*)::bigint AS n
FROM sms_messages
WHERE direction = 'outbound'
  AND created_at >= sqlc.arg(since)
GROUP BY status;

-- name: CountOutboundSmsSinceByProduct :many
SELECT
    CASE
        WHEN p.product IS NOT NULL THEN p.product::text
        WHEN m.to_msisdn ~ '^7[0-9]{10}$' THEN 'sms_domestic'
        ELSE 'sms_international'
    END AS product,
    count(*)::bigint AS n
FROM sms_messages m
LEFT JOIN tariff_plans p ON p.id = m.tariff_plan_id
WHERE m.direction = 'outbound'
  AND m.created_at >= sqlc.arg(since)
GROUP BY 1;

-- name: SumBilledSegmentsSince :one
SELECT COALESCE(sum(billed_segments), 0)::bigint AS n
FROM sms_messages
WHERE direction = 'outbound'
  AND billed_segments IS NOT NULL
  AND created_at >= sqlc.arg(since);

-- name: CountOutboundSmsSinceByStatusForClient :many
SELECT status, count(*)::bigint AS n
FROM sms_messages
WHERE direction = 'outbound'
  AND client_id = sqlc.arg(client_id)
  AND created_at >= sqlc.arg(since)
GROUP BY status;
