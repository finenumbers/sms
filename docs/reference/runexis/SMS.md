# Runexis DIDAPI — SMS

Primary SMS surface for the product: outbound send, statistics, per-number / global
DLR & incoming (MO) handler URL registration, and SMS direction flags.

**Note:** callback *payload* formats for DLR / incoming SMS are **not** documented in the vendor HTML.
See [GAPS.md](GAPS.md).

Base URL: `https://didapi.runexis.ru`

## Endpoints

### Установка URL обработчика отчетов о доставке СМС для номера

- **Method:** `PATCH`
- **Path:** `/api/v1/numbers/{number}/sms/dlr-url`
- **Auth:** required

**URL parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| number | string | no | Номер телефона. Example: 79996665522 |

**Body parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| url | string | no | Валидный URL (http, https). Example: https://www.oconnell.org/veniam-error-id-qui-iste-quibusdam-et-dicta |

**Example request body**

```json
{
  "url": "https://www.oconnell.org/veniam-error-id-qui-iste-quibusdam-et-dicta"
}
```

**Example response**

```json
{
  "data": {},
  "success": true
}
```

### Установка URL обработчика входящих СМС для номера

- **Method:** `PATCH`
- **Path:** `/api/v1/numbers/{number}/sms/hook-url`
- **Auth:** required

**URL parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| number | string | no | Номер телефона. Example: 79996665522 |

**Body parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| url | string | no | Валидный URL (http, https). Example: https://www.oconnell.org/veniam-error-id-qui-iste-quibusdam-et-dicta |

**Example request body**

```json
{
  "url": "https://www.oconnell.org/veniam-error-id-qui-iste-quibusdam-et-dicta"
}
```

**Example response**

```json
{
  "data": {},
  "success": true
}
```

### Удаление URL обработчика отчетов о доставке СМС для номера

- **Method:** `DELETE`
- **Path:** `/api/v1/numbers/{number}/sms/dlr-url`
- **Auth:** required

**URL parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| number | string | no | Номер телефона. Example: 79996665522 |

**Body parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| url | string | no | Валидный URL (http, https) или null. Example: https://www.oconnell.org/veniam-error-id-qui-iste-quibusdam-et-dicta |

**Example request body**

```json
{
  "url": "https://www.oconnell.org/veniam-error-id-qui-iste-quibusdam-et-dicta"
}
```

**Example response**

```json
{
  "data": {},
  "success": true
}
```

### Удаление URL обработчика входящих СМС для номера

- **Method:** `DELETE`
- **Path:** `/api/v1/numbers/{number}/sms/hook-url`
- **Auth:** required

**URL parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| number | string | no | Номер телефона. Example: 79996665522 |

**Body parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| url | string | no | Валидный URL (http, https) или null. Example: https://www.oconnell.org/veniam-error-id-qui-iste-quibusdam-et-dicta |

**Example request body**

```json
{
  "url": "https://www.oconnell.org/veniam-error-id-qui-iste-quibusdam-et-dicta"
}
```

**Example response**

```json
{
  "data": {},
  "success": true
}
```

### Обновление включенных СМС направлений для номера

- **Method:** `PATCH`
- **Path:** `/api/v1/numbers/{number}/sms/directions`
- **Auth:** required

**URL parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| number | string | no | Номер телефона. Example: 79996665522 |

**Body parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| in | boolean | yes | Флаг включения входящих СМС. Example: false |
| dom_out | boolean | yes | Флаг включения внутрироссийских исходящих СМС. Example: false |
| int_out | boolean | yes | Флаг включения международных исходящих СМС. Example: false |
| in_mass | boolean | yes | Флаг включения входящих A2P СМС. Example: false |

**Example request body**

```json
{
  "in": false,
  "dom_out": false,
  "int_out": false,
  "in_mass": false
}
```

**Example response**

```json
{
  "data": {},
  "success": true
}
```

### Получение СМС настроек аккаунта

- **Method:** `GET`
- **Path:** `/api/v1/numbers/{number}/sms/account`
- **Auth:** required

**URL parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| number | string | no | Номер телефона. Example: 79996665522 |

**Example response**

```json
{
  "data": {
    "hook_urls": [
      "http://www.mcclure.com/sms-handler"
    ],
    "dlr_urls": [
      "http://www.mcclure.com/sms-handler"
    ],
    "in": true,
    "dom_out": true,
    "int_out": false,
    "in_mass": true
  },
  "success": true
}
```

### Установка URL глобального обработчика отчетов о доставке СМС

- **Method:** `PATCH`
- **Path:** `/api/v1/sms/dlr-url`
- **Auth:** required

**Body parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| url | string | no | Валидный URL (http, https) или null. Example: https://www.oconnell.org/veniam-error-id-qui-iste-quibusdam-et-dicta |

**Example request body**

```json
{
  "url": "https://www.oconnell.org/veniam-error-id-qui-iste-quibusdam-et-dicta"
}
```

**Example response**

```json
{
  "data": {},
  "success": true
}
```

### Установка URL глобального обработчика входящих СМС

- **Method:** `PATCH`
- **Path:** `/api/v1/sms/hook-url`
- **Auth:** required

**Body parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| url | string | no | Валидный URL (http, https) или null. Example: https://www.oconnell.org/veniam-error-id-qui-iste-quibusdam-et-dicta |

**Example request body**

```json
{
  "url": "https://www.oconnell.org/veniam-error-id-qui-iste-quibusdam-et-dicta"
}
```

**Example response**

```json
{
  "data": {},
  "success": true
}
```

### Отправка СМС

- **Method:** `POST`
- **Path:** `/api/v1/sms/send`
- **Auth:** required

**Body parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| from_number | string | no | Номер отправителя СМС. Must be 11 digits. Must start with one of 7 . Example: 79991112233 |
| to_number | number | no | Номер получателя СМС. Example: 79993332211 |
| text | string | no | Текст СМС. Example: Пример сообщения |

**Example request body**

```json
{
  "from_number": "79991112233",
  "to_number": 79993332211,
  "text": "Пример сообщения"
}
```

**Example response**

_Not present in vendor HTML — see GAPS.md._

### Получение СМС настроек

- **Method:** `GET`
- **Path:** `/api/v1/sms/settings`
- **Auth:** required

**Example response**

```json
{
  "data": {
    "hook_url": "http://www.mcclure.com/sms-handler",
    "in": true,
    "dom_out": true,
    "int_out": false
  },
  "success": true
}
```

### Получение статистики по СМС

- **Method:** `GET`
- **Path:** `/api/v1/sms/statistic`
- **Auth:** required

**Body parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| from | string | no | Начальная дата. Must be a valid date. Example: 2025-12-01 00:00:00 |
| to | string | no | Конечная дата. Must be a valid date. Must be a date after or equal to from . Example: 2025-12-31 23:59:59 |
| sender_numbers | string[] | yes | Список номеров отправителей в формате 7XXXXXXXXXX. Must be 11 digits. Must start with one of 7 . |
| receiver_numbers | string[] | yes | Список номеров получателей в формате 7XXXXXXXXXX. Must be 11 digits. Must start with one of 7 . |
| incoming | boolean | yes | Направление: входящие (true), исходящие (false) или все (по умолчанию). Example: false |
| page | integer | yes | Номер страницы. Must be at least 1. Example: 1 |
| limit | integer | yes | Количество записей на странице. Must be between 1 and 50. Example: 30 |

**Example request body**

```json
{
  "from": "2025-12-01 00:00:00",
  "to": "2025-12-31 23:59:59",
  "sender_numbers": [
    [
      "79996665522",
      "79996665523",
      "79996665524"
    ]
  ],
  "receiver_numbers": [
    [
      "79996665522",
      "79996665523",
      "79996665524"
    ]
  ],
  "incoming": false,
  "page": 1,
  "limit": 30
}
```

**Example response**

```json
{
  "data": [
    {
      "sms_id": "8ace264b-a78e-488e-b411-af2581bd3f23",
      "date": "2025-12-01 16:00:45",
      "sender_number": "79991234567",
      "receiver_number": "79876543210",
      "message": "Per aspera ad astra.",
      "incoming": true,
      "pdu": 1,
      "sent": true,
      "delivered": true
    },
    {
      "sms_id": "71e4c84e-1a49-4b3a-9c14-5c6b0a17c0a2",
      "date": "2025-12-02 12:30:41",
      "sender_number": "79876543210",
      "receiver_number": "79991234567",
      "message": "Lorem ipsum.",
      "incoming": true,
      "pdu": 1,
      "sent": true,
      "delivered": true
    }
  ],
  "meta": {
    "total": 2,
    "page": 1,
    "limit": 30
  },
  "success": true
}
```
