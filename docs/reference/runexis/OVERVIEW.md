# Runexis DIDAPI — Overview

Source of truth (immutable vendor archive):
[`docs/vendor/runexis/DIDAPI Documentation.html`](../../vendor/runexis/DIDAPI%20Documentation.html)

## Base URL

`https://didapi.runexis.ru`

## Authentication

Include header:

```http
Authorization: Bearer {token}
```

Obtain / refresh tokens via Auth routes (`/api/v1/login`, `/api/v1/refresh`). See [AUTH.md](AUTH.md).

## Response envelope

Success responses typically look like:

```json
{
  "data": {},
  "success": true
}
```

Error responses (4XX / 500):

```json
{
  "code": 400,
  "message": "...",
  "request_id": "...",
  "success": false
}
```

## HTTP status codes

| Code | Meaning |
|---|---|
| 200 | Success |
| 400 | Client error (validation, missing ID, etc.) |
| 401 | Missing / invalid auth token |
| 403 | Account lacks permission |
| 404 | URL not found |
| 405 | Method not allowed for URL |
| 500 | Internal server error |

## Focused references for Finenumbers SMS Service

| Doc | Scope |
|---|---|
| [AUTH.md](AUTH.md) | Platform login to Runexis |
| [SMS.md](SMS.md) | Send / stats / DLR & MO URL registration / directions |
| [NUMBERS.md](NUMBERS.md) | Partner number inventory (no purchase in our product) |
| [SIMS_SMS.md](SIMS_SMS.md) | Informational SIM SMS channel (not primary product send) |
| [WEBHOOKS.md](WEBHOOKS.md) | Lifecycle webhooks (not SMS DLR payloads) |
| [GAPS.md](GAPS.md) | Missing contracts that block implementation |
| [ENDPOINTS.json](ENDPOINTS.json) | Machine-readable catalog (all sections) |

## Regenerating

```bash
python3 scripts/extract_runexis_docs.py
```
