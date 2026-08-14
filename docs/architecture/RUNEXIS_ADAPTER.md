# Runexis DIDAPI adapter

Единственное место знания wire-JSON: `internal/runexis` + фикстуры в [`docs/reference/runexis/fixtures/`](../reference/runexis/fixtures/).

Наш публичный API всегда использует строковые MSISDN. Имена полей Runexis наружу не торчат.

## Send (эталон HTML)

`POST https://didapi.runexis.ru/api/v1/sms/send`

- `from_number`: string, 11 цифр, начинается с `7`
- `to_number`: JSON **integer** (int64), не float64 (в PHP-примере эталона ошибочно `.0`)
- `text`: string
- Example response в HTML **нет** — фиксируется живой фикстурой
- Live audit (2026-08-12, sms `a068e8a6-…`): worker отправил HTML-контракт запроса (`POST /api/v1/sms/send`, заголовки `Authorization` / `Content-Type` / `Accept`, body `{from_number: string 7XXXXXXXXXX, to_number: int64, text}`). Runexis ответил HTTP 500 `an unexpected error has occurred` (не 400) — это не отказ валидации по эталону. Сырые байты того POST в `ops_events` не сохранились (журнал request появился позже); форма восстанавливается из `sms_messages` + `marshalSend`. Marshaler из-за 500 не менять.

Если живой API разойдётся с HTML — меняется marshaler, не домен.

## Statistic

`GET /api/v1/sms/statistic` **с JSON body**. Не переводить в query без live-проверки.

Пример `sender_numbers: [[...]]` в docs — артефакт Scribe. Сначала плоский `string[]`.

Окна дат: **Europe/Moscow**, пока live не докажет иное. Наши timestamps — UTC.

## Прочее

- Token: Redis cache, refresh до expire под lock, всегда сохранять новый refresh_token
- Никогда не вызывать `/numbers/{n}/sim/send-sms`
- `in_mass` — входящий A2P, не наша рассылка. Дефолт directions: `in=true`, `dom_out=true`, `int_out=false`, `in_mass=false`
- Assign номера: worker делает `PATCH /api/v1/numbers/{n}/sms/directions` со снимком Settings (не блокирует транзакцию назначения)
- Inventory: `GET /api/v1/numbers/management?page=&limit=` (query, без JSON body). Не `POST /numbers/load-data`, не витрина `GET /api/v1/numbers`. SMS-фильтр: `GET /numbers/{n}/sms/account` (200 = есть SMS-ресурс, даже если все directions false; 4xx = не импортировать). 200 **не** значит, что `POST /sms/send` пройдёт. Snapshot — поля management `id/status/city/tariff/class/operator`, без `comment` и без `GET /numbers/{n}` (PII абонента).
- Глобальные callback URL: `PATCH /api/v1/sms/dlr-url` и `PATCH /api/v1/sms/hook-url` (admin `POST /admin/v1/settings/runexis/callbacks`). Per-number — не v1
- Send response: HTML пустой; парсер читает `data.sms_id` (фикстура provisional). Live capture заменит файл. В `ops_events` — redacted request/response: текст SMS маскируется, envelope `message` и `request_id` оставляем для техподдержки.
- DLR/MO callback body в HTML нет. Provisional parser читает те же поля, что statistic (`sms_id`, `sent`, `delivered`, `sender_number`, `receiver_number`, `message`), плюс query/form. Фикстуры `dlr_callback.provisional.json` / `mo_callback.provisional.json`.
