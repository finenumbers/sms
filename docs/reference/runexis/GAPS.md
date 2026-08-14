# Runexis DIDAPI — Gaps & open contracts

Gaps discovered while turning the vendor HTML into an implementation reference.
Resolve these before coding callback ingress and status correlation.

## Critical

### 1. DLR callback payload is undocumented

Vendor docs expose registration only:

- `PATCH /api/v1/sms/dlr-url`
- `PATCH /api/v1/numbers/{number}/sms/dlr-url`
- matching `DELETE` routes

There is **no** schema for the HTTP request Runexis sends to that URL:

- method (assumed POST, not stated for SMS DLR)
- content-type
- body fields (`sms_id`? `from`/`to`? delivery state codes?)
- auth / signature / shared secret
- retry policy

**Action:** obtain contract from Runexis support and/or capture live traffic into a fixture under `docs/reference/runexis/fixtures/` before implementing ingress.

### 2. Incoming SMS (MO) callback payload is undocumented

Same situation for:

- `PATCH /api/v1/sms/hook-url`
- `PATCH /api/v1/numbers/{number}/sms/hook-url`

No payload example for the message body delivered to our handler.

**Action:** same as DLR — confirm contract + store fixtures.

### 3. `POST /api/v1/sms/send` response example missing

Request body is documented:

```json
{
  "from_number": "79991112233",
  "to_number": 79993332211,
  "text": "Пример сообщения"
}
```

Vendor HTML has **no** `Example response (200)` for send. Unknown whether response includes `sms_id` (or equivalent) needed to correlate later DLR / statistic rows.

**Action:** smoke-test against sandbox/prod agent account; store the live envelope as `fixtures/sms_send_response.provisional.json` (replace) after redacting secrets/MSISDN.

Provisional parser (wave 3): success envelope `{data, success}`. `sms_id` is read from `data.sms_id` / `data.id` / `data.message_id`, or from a string `data`. Empty `data` is accepted (provider id unknown until statistic/DLR). Fixture: [`fixtures/sms_send_response.provisional.json`](fixtures/sms_send_response.provisional.json).

Live request (2026-08-12, sms `a068e8a6-…`, `79391125968` → `79994504444`, text `test`) matched the HTML curl/Python/JS example (integer `to_number`, not PHP Scribe `.0`). Both POSTs returned HTTP 500 `an unexpected error has occurred` — not 400 — so the documented request schema was accepted. The success-response gap remains.

## Important product gaps (not DIDAPI bugs)

### 4. No outbound mass-campaign API

Searches for campaign/bulk/рассылка in the vendor archive find nothing useful.
`in_mass` on SMS directions means **incoming A2P**, not outbound bulk send.

Group SMS in Finenumbers must be implemented as our domain:

`SmsCampaign` → queue → N × `POST /api/v1/sms/send` → aggregate DLR/statistic.

### 5. Number purchase APIs exist but are out of scope

DIDAPI includes booking/purchase/MNP/agreement flows under Numbers.
Finenumbers admin **loads already-purchased DEF numbers** from `GET /api/v1/numbers/management` (SMS-capable via `GET .../sms/account`) and assigns them to clients.
Do not build purchase UX against these endpoints in v1.

### 6. Lifecycle WebHooks ≠ SMS callbacks

`/api/v1/webhooks` covers `gu_verified`, `mnp`, `number_blocked`, SIM events, etc.
They are useful later for number health, but they do **not** replace SMS DLR/MO handlers.

## Capture workflow (do not invent JSON)

Provisional DLR/MO/send-response fixtures stay until a **live** DIDAPI callback is captured. Do not replace them from guesswork.

1. Portainer+NPM up, Settings: agent login → «Проверить обмен с DIDAPI» (`/me` must succeed).
2. `callback_base_url` = `https://api.{fqdn}` → register dlr-url / hook-url.
3. Send a real SMS (LK or `/v1`) and, if possible, trigger an inbound MO.
4. Admin → Callbacks: open the raw event, redact secrets/MSISDN.
5. Replace `fixtures/dlr_callback.provisional.json`, `dlr_callback.failed.provisional.json`, `mo_callback.provisional.json`, and/or `sms_send_response.provisional.json` with the captured envelope. Rename without `.provisional`.
6. Tighten [`internal/ingress`](../../../internal/ingress/ingress.go) and send-response parser against those files; update this table.

Until that happens, send goes through outbox + statistic; DLR/inbox is best-effort on the provisional parser.

## Minor / hygiene

| Item | Note |
|---|---|
| `to_number` type | Documented as `number` while `from_number` is `string` (11 digits, starts with `7`). Normalize to string MSISDN in our API. |
| Statistic via GET + body | `/api/v1/sms/statistic` is documented as GET with a JSON body. Confirm client/proxy behavior; may need special HTTP client handling. |
| Dual send channels | `/api/v1/sms/send` (product) vs `/api/v1/numbers/{number}/sim/send-sms` (informational SIM). Do not mix. |
| DELETE dlr/hook body | Extracted examples sometimes show a `url` body on DELETE; treat as unverified — confirm before relying on it. |
| `sms/account` as SMS capability | HTML documents settings (`in`/`dom_out`/`int_out`/`in_mass`), not a capability catalog. v1 treats HTTP 200 as “SMS exists” (even if all flags are false) and 4xx as skip. 200 is **not** proof that `POST /sms/send` will succeed; 4xx is **not** proof that send would fail. Do not invent a `sms` field on management. |
| `number_status_id` 1–10 | Documented as an enum without a mapping (example mnemonic `"free"`). Sync does **not** filter by status. |
| Live management shape | Parser follows the HTML example. If live JSON differs, capture a fixture — do not guess fields. |

## Resolution log

| Gap | Status | Resolved in |
|---|---|---|
| DLR payload | provisional — statistic field names | Wave 9: `fixtures/dlr_callback.provisional.json`; parser + worker. Replace with live capture |
| MO payload | provisional — statistic field names | Wave 9: `fixtures/mo_callback.provisional.json`; parser + worker. Replace with live capture |
| Send request vs HTML | verified | 2026-08-12 live POST matched HTML curl/Python/JS; see `TestMarshalSendLiveAttemptMatchesHTMLContract` |
| Send response | provisional | `fixtures/sms_send_response.provisional.json` — live send returned 500, no success body |
| Campaign API | accepted as ours | product design |
| Purchase APIs | out of scope | product design |
