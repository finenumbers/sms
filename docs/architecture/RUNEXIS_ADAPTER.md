# Runexis DIDAPI adapter

Единственное место знания wire-JSON: `internal/runexis` + фикстуры в [`docs/reference/runexis/fixtures/`](../reference/runexis/fixtures/).

Наш публичный API всегда использует строковые MSISDN. Имена полей Runexis наружу не торчат.

## Send (эталон HTML)

`POST https://didapi.runexis.ru/api/v1/sms/send`

- `from_number`: string, 11 цифр, начинается с `7`
- `to_number`: JSON **string** (digits, 8–15). HTML documents `number`; that is wrong. Support 2026-08-14: integer `to_number` → HTTP 500 `an unexpected error has occurred`. Live 2026-08-12 integer POST is the failed case, not the contract.
- `text`: string
- Example response в HTML **нет** — фиксируется живой фикстурой

Если живой API разойдётся с HTML — меняется marshaler, не домен.

## Statistic

`GET /api/v1/sms/statistic` **с JSON body**. Не переводить в query без live-проверки.

Пример `sender_numbers: [[...]]` в docs — артефакт Scribe. Сначала плоский `string[]`.

Окна дат: **UTC** naive `YYYY-MM-DD HH:MM:SS` (live 2026-08-14). `date` in statistic matches our `created_at` Z, not Europe/Moscow. Moscow `from`/`to` miss the row.

## Прочее

- Token: Redis cache, refresh до expire под lock, всегда сохранять новый refresh_token
- Никогда не вызывать `/numbers/{n}/sim/send-sms`
- `in_mass` — входящий A2P, не наша рассылка. Дефолт directions: `in=true`, `dom_out=true`, `int_out=false`, `in_mass=false`
- Assign номера: worker делает `PATCH /api/v1/numbers/{n}/sms/directions` со снимком Settings (не блокирует транзакцию назначения)
- Inventory: `GET /api/v1/numbers/management?page=&limit=` (query, без JSON body). Не `POST /numbers/load-data`, не витрина `GET /api/v1/numbers`. SMS-фильтр: `GET /numbers/{n}/sms/account` (200 = есть SMS-ресурс, даже если все directions false; 4xx = не импортировать). 200 **не** значит, что `POST /sms/send` пройдёт. Snapshot — поля management `id/status/city/tariff/class/operator`, без `comment` и без `GET /numbers/{n}` (PII абонента).
- Глобальные callback URL: `PATCH /api/v1/sms/dlr-url` и `PATCH /api/v1/sms/hook-url` (admin `POST /admin/v1/settings/runexis/callbacks`). Per-number — не v1
- Send response: live `{success:true, data:{id, pdu}}` ([`sms_send_response.json`](../reference/runexis/fixtures/sms_send_response.json)). Parser reads `data.id` (also `sms_id` / `message_id`). В `ops_events` — redacted request/response: текст SMS маскируется, envelope `message` и `request_id` оставляем для техподдержки.
- DLR/MO callback body в HTML нет. Live DLR: [`dlr_callback.json`](../reference/runexis/fixtures/dlr_callback.json) (`id` + `message_status`). Код `2` → `sent`, не `delivered`, пока поддержка не подтвердит таблицу кодов. Поля statistic `sent`/`delivered` тоже читаются. Failed/MO — provisional.
