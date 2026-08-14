# Live / vendor Runexis wire fixtures

Source of truth for `internal/runexis` marshalers. No real credentials or production MSISDN.

| File | Origin |
|---|---|
| `auth_login_*.json` | Vendor HTML example (`POST /api/v1/login`) |
| `auth_refresh_*.json` | Vendor HTML example (`POST /api/v1/refresh`) |
| `auth_me_response.json` | Vendor HTML example (`GET /api/v1/me`) |
| `sms_send_request.json` | Live DIDAPI contract (support 2026-08-14). `to_number` is a JSON **string**; HTML documents `number` and is wrong |
| `sms_send_response.provisional.json` | Assumed envelope until live capture: `data.sms_id` (same field as statistic) |
| `sms_send_response.empty.json` | HTML gap: success with empty `data` |
| `sms_statistic_request.json` | What **we** send: flat `string[]` (Scribe nested `[[...]]` is an artifact) |
| `sms_statistic_response.json` | Vendor HTML example |
| `sms_directions_request.json` | What **we** send on assign: Settings snapshot (`in`/`dom_out`/`int_out`/`in_mass`) |
| `sms_directions_response.json` | Vendor HTML example (`PATCH /numbers/{n}/sms/directions`) |
| `numbers_management_response.json` | Vendor HTML example (`GET /api/v1/numbers/management`) |
| `sms_account_response.json` | Vendor HTML example (`GET /numbers/{n}/sms/account`) |
| `error_400.json` | Vendor error envelope |
| `dlr_callback.provisional.json` | Assumed DLR body (statistic field names) until live capture |
| `dlr_callback.failed.provisional.json` | Assumed failed DLR (`status=failed`) |
| `mo_callback.provisional.json` | Assumed MO body (statistic incoming row) until live capture |

Replace `sms_send_response.provisional.json` and `*_callback.provisional.json` with live captures (redact secrets/MSISDN) when the agent account is available. Capture path: Admin → Callbacks (raw body) after registering dlr/hook URL. See [GAPS.md](../GAPS.md). GET query and `application/x-www-form-urlencoded` are also accepted by the parser.
