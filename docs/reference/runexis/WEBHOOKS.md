# Runexis DIDAPI — WebHooks (lifecycle)

Lifecycle / provisioning webhooks (`gu_verified`, `mnp`, `number_blocked`, SIM events, etc.).

These are **not** the SMS DLR / incoming-SMS callbacks. SMS callbacks are registered via
`/api/v1/sms/dlr-url`, `/api/v1/sms/hook-url` and per-number equivalents (see [SMS.md](SMS.md)).

Base URL: `https://didapi.runexis.ru`

## Endpoints

### Получение всех вебхуков

- **Method:** `GET`
- **Path:** `/api/v1/webhooks`
- **Auth:** required

**Example response**

```json
{
  "data": [
    {
      "url": "https://webhook.site/22cf0069-1945-451b-acc0-ec14e258f112",
      "type": "mnp_status_changed",
      "token": "https://webhook.site/22cf0069-1945-451b-acc0-ec14e258f112"
    },
    {
      "url": "https://webhook.site/22cf0069-1945-451b-acc0-ec14e258f112",
      "type": "gu_verified",
      "token": "https://webhook.site/22cf0069-1945-451b-acc0-ec14e258f112"
    },
    {
      "url": "https://webhook.site/22cf0069-1945-451b-acc0-ec14e258f112",
      "type": "ready_for_activation",
      "token": "https://webhook.site/22cf0069-1945-451b-acc0-ec14e258f112"
    }
  ],
  "success": true
}
```

### Создание или обновление вебхука

- **Method:** `PUT`
- **Path:** `/api/v1/webhooks`
- **Auth:** required

**Body parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| url | string | no | URL вебхука. Example: https://sporer.com/explicabo-est-officia-sit-natus-dolorem-illum-aut.html |
| token | string | yes | Токен вебхука. Example: token_name |
| type | string | no | Тип вебхука. Example: gu_verified Must be one of: - gu_verified - mnp - mnp_cancel - ready_for_activation - gu_refused - ksim_actions - number_blocked - number_unblocked - sim_blocked - sim_unblocked - sim_service_blocked - sim_service_unblocked - sim_service_limited - timer - check_self_ban - self_ban_token - monthly_report - label_number_moderation_event - outgoing_mav_moderation_event |

**Example request body**

```json
{
  "url": "https://sporer.com/explicabo-est-officia-sit-natus-dolorem-illum-aut.html",
  "token": "token_name",
  "type": "gu_verified"
}
```

**Example response**

```json
{
  "data": {
    "url": "http://www.quigley.com/libero-exercitationem-sed-occaecati-aperiam-repudiandae-consequatur.html",
    "type": "gu_verified",
    "token": "perspiciatis"
  },
  "success": true
}
```

### Удаление вебхука по типу

- **Method:** `DELETE`
- **Path:** `/api/v1/webhooks/{type}`
- **Auth:** required

**URL parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| type | string | no | Тип вебхука. Example: gu_verified |

**Example response**

```json
{
  "success": true
}
```
