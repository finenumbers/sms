# Runexis DIDAPI — SIM informational SMS

Separate SIM channel: `POST /api/v1/numbers/{number}/sim/send-sms`.

This is **not** the primary product send path. Product outbound SMS uses `POST /api/v1/sms/send`
(see [SMS.md](SMS.md)).

Base URL: `https://didapi.runexis.ru`

## Endpoints

### Отправка информационного SMS-сообщения пользователю.

- **Method:** `POST`
- **Path:** `/api/v1/numbers/{number}/sim/send-sms`
- **Auth:** required

**URL parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| number | string | no | Номер. Example: 79996665522 |

**Query parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| message | string | no | Сообщение. Must be at least 3 characters. Must not be greater than 255 characters. Example: Test message example. |

**Example response**

```json
{
  "data": {},
  "success": true
}
```
