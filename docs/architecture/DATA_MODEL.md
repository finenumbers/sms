# Модель данных

PostgreSQL 16. UUID PK, `timestamptz` UTC. Текст SMS — ПДн: только `sms_messages`, не в `audit_log` и не в `ops_events`.

Источник схемы: [`db/migrations/`](../../db/migrations/). sqlc читает `*.up.sql`.

## Инварианты

- Не более одного открытого `number_assignments` на номер (`unassigned_at IS NULL`)
- `def_numbers.status` меняется в той же транзакции, что assignment
- После выхода кампании из `draft` поля `from_msisdn` и `text` immutable (код: update только `WHERE status = 'draft'`)
- `provider_sms_id` уникален, если задан
- Inbound без assignment: `sms_messages.client_id` NULL, в ЛК не показывается
- Сырые HTTP body провайдера — в `provider_send_attempts` (redacted JSON) и `provider_callback_events`. В `ops_events.detail` — redacted `request`/`response` DIDAPI (envelope `message`/`request_id` сохраняются; текст SMS маскируется). Не в `sms_messages`.
- `ops_events` — журнал админки (`http|didapi|queue|ingress|audit`). List без `detail`. HTTP `request_id` ≠ worker `job:{uuid}`: искать по `client_id` и окну времени
- `provider_send_attempts.request_meta` — только `from`/`to`, без текста SMS
- Stale `send_jobs.processing` → `uncertain` (не `pending`), чтобы не слать SMS дважды
- Assign пишет `number_assignments` и `def_numbers.status=assigned` в одной транзакции, затем outbox `number_direction_jobs` (worker: `PATCH /numbers/{n}/sms/directions`)
- Unassign сразу закрывает assignment (`unassigned_at`); направления у провайдера не откатываем

## Горячие индексы

- `sms_messages (client_id, created_at DESC)`
- `sms_messages (client_id, direction, created_at DESC)`
- `send_jobs (available_at)` WHERE status IN (`pending`, `retry`, `uncertain`)
- `campaign_recipients (campaign_id, status)`
- `ops_events (created_at DESC)` / `(category, created_at DESC)`
- `wallet_transactions (created_at DESC)` / `(type, created_at DESC)` / `(client_id, created_at DESC)`
- `lookup_jobs (client_id, created_at DESC)` / `(status, created_at DESC)`
- `lookup_items (job_id, status)` / `(status, next_poll_at)` WHERE pending
- `webhook_deliveries (status, next_attempt_at)`

## Биллинг

Предоплатный кошелёк клиента: `wallets` (`available_balance` + `held_balance`), журнал `wallet_transactions` (CREDIT/HOLD/DEBIT/RELEASE/ADJUSTMENT). Себестоимости в тарифе нет.

SMS: цена `sell_price × billed_segments` (PDU), снимок на `sms_messages`. HOLD на сообщение. DLR failed после `accepted_at` → CAPTURE. `settle()` / `ReapOpenHolds` — только SMS (hold закрыт любым дочерним DEBIT/RELEASE).

Lookup: цена за проверку, снимок на `lookup_jobs` / `lookup_items`. Один HOLD на job (`unit_sell_price × N`), доли DEBIT/RELEASE с `related_hold_id`. Открыт, пока `hold.amount - SUM(дети) > 0`. Свои методы и `ReapOpenLookupHolds`. FK `lookup_job_id` / `lookup_item_id` — `ON DELETE RESTRICT`. Policy B и поздний callback — [`LOOKUP.md`](../product/LOOKUP.md).

- нет тарифа → 409, нет денег → 402
- retention не удаляет SMS с открытым HOLD; lookup job/item с остатком HOLD не удалять
- строки леджера не чистятся

Продукты тарифа: `sms_domestic`, `sms_international`, `hlr`, `silent_sms`. Silent default нет.

Партиции по месяцу — не v1. Retention — hourly worker job по `system_settings.retention_days` (SMS, кампании, raw callbacks), `lookup_retention_days` (raw SMSC, дефолт 90; `lookup_items.phone_e164` не чистить), `audit_retention_days` (24 мес) и `ops_retention_days` (дефолт 14, 1–90). `ops_events` чистится отдельным батчем (LIMIT 5000).

Lookup-таблицы: `lookup_jobs`, `lookup_items`, `lookup_csv_previews`, `provider_lookup_requests`, `provider_lookup_callbacks`, `webhook_endpoints`, `webhook_deliveries`. Флаг `system_settings.lookup_enabled` (дефолт false). Креды SMSC — колонки `smsc_*` в `system_settings` (пароль/ключ/секрет шифруются), не env.
