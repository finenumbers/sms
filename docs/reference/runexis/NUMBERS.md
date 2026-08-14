# Runexis DIDAPI — Numbers (inventory)

Number inventory & status APIs useful for reconciling purchased DEF numbers.

**Out of product scope:** booking, purchase, MNP, and agreement-binding flows exist in DIDAPI
but are not exposed by Finenumbers SMS Service. Admin loads already-purchased numbers from
`GET /api/v1/numbers/management` (not CSV upload into Runexis `POST /numbers/load-data`).

Base URL: `https://didapi.runexis.ru`

## Endpoints

### Список номеров партнера

- **Method:** `GET`
- **Path:** `/api/v1/numbers/management`
- **Auth:** required

**Query parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| page | integer | yes | Номер страницы. Must be at least 1. Example: 1 |
| limit | integer | yes | Количество элементов на странице. Must be at least 1. Example: 30 |
| number_status_id | integer | yes | Статус номера. Example: 1 Must be one of: - 1 - 2 - 3 - 4 - 5 - 6 - 7 - 8 - 9 - 10 |
| project_id | string | yes | ID проекта. Must be a valid UUID. Example: 8ace264b-a78e-488e-b411-af2581bd3f24 |
| tariff_id | string | yes | ID тарифа. Must be a valid UUID. Example: 8ace264b-a78e-488e-b411-af2581bd3f25 |
| city_ids | integer[] | yes | Массив идентификаторов городов. |
| region_codes | string[] | yes | Массив кодов регионов. Must be 3 digits. |
| class_id | integer[] | yes | Must be one of: - 0 - 1 - 2 - 3 - 4 - 5 |
| operator_id | string | yes | ID оператора. Must not be greater than 32 characters. Example: 01.2008 |
| phone_number | string | yes | Номер телефона без кода или полный номер. Must be at least 3 characters. Example: 9011234567 |
| ip_address | string | yes | IP-адрес номера. Example: 10.0.0.5 |
| equipment_name | string | yes | Название оборудования. Must be at least 3 characters. Example: Mera equipment |
| responsible_id | string | yes | ID ответственного. Must be a valid UUID. Example: 17f495d4-88c4-46a7-b8f9-2d561c6af99d |
| last_action_date | string | yes | Дата последнего действия по номеру. Must be a valid date in the format Y-m-d . Example: 2026-06-23 |

**Example response**

```json
{
  "data": [
    {
      "id": "8ace264b-a78e-488e-b411-af2581bd3f11",
      "code": "999",
      "number": "1234567",
      "status": {
        "id": "c4f20b18-8b24-4c6f-8e3e-4d2f2f638c10",
        "name": "Free",
        "mnemonic": "free"
      },
      "sump_until": null,
      "partner": {
        "id": "c8c627d5-1e5a-4f7e-a98a-5c81289ae001",
        "name": "Partner name"
      },
      "ipAddress": "10.0.0.5",
      "project": {
        "id": "f6d574d6-1998-46c8-b3d8-d74c4ef0c100",
        "name": "Project name"
      },
      "city": {
        "id": 77,
        "name": "Moscow"
      },
      "tariff": {
        "id": "1118b6bf-a8d8-4efa-9f3c-05b2919b20d0",
        "name": "Starter Plan"
      },
      "installationCost": 10,
      "subscriptionFee": 99.99,
      "comment": "Test comment",
      "class": {
        "id": 0,
        "name": "Простой",
        "mnemonic": "simple"
      },
      "equipment": {
        "id": 123,
        "name": "Mera equipment"
      },
      "operator": {
        "id": "01.2008",
        "name": "Operator"
      },
      "lastActionAt": "2025-01-01T00:00:00Z",
      "responsible": {
        "id": "17f495d4-88c4-46a7-b8f9-2d561c6af99d",
        "name": "Manager"
      },
      "meraPrice": 50.5
    }
  ],
  "meta": {
    "total": 1,
    "page": 1,
    "limit": 30
  },
  "success": true
}
```

### Поиск номеров

- **Method:** `GET`
- **Path:** `/api/v1/numbers`
- **Auth:** required

**Query parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| name | string | yes | ФИО для ФЛ и ИП или название компании для ЮЛ (часть или полностью). Must be at least 3 characters. Example: Иванов |
| reference_number | string | yes | Номер договора (часть или полностью). Must be at least 3 characters. Example: Д89 |
| region_name | string | yes | Название региона (часть или полностью). Must be at least 3 characters. Example: Московская область |
| city_name | string | yes | Название города (часть или полностью). Must be at least 2 characters. Example: Москва |
| block_status | string | yes | Статус блокировки номера. Example: all Must be one of: - all - blocked - active |
| number_type | string | yes | Тип номера. Example: abc Must be one of: - abc - def - fed |
| phone_number | string | yes | Номер телефона без кода (часть или полностью). Must be between 3 and 7 digits. Phone number mask min 1 and max 6 digits( ⚹2 or 123456⚹). Example: 1112233 |
| numbers | string[] | yes | Номер не должен начинаться с 7. Must be 10 digits. |
| region_ids | integer[] | yes | Массив идентификаторов регионов. |
| city_ids | integer[] | yes | Массив идентификаторов городов. |
| region_codes | string[] | yes | Массив кодов регионов. Must be 3 digits. |
| data_status | string | yes | Статус данных номера. Example: not_filled Must be one of: - not_filled - need_gu_verification - ok |
| inn | string | yes | ИНН. Must match the regex /^([\d]{10}\|[\d]{12})$/. Example: 7701234567 |
| agreement_id | string | yes | ID договора. Must be a valid UUID. Example: 8ace264b-a78e-488e-b411-af2581bd3f23 |
| to_block_before | string | yes | Верхняя граница даты и времени истечения таймера для блокировки (в UTC). Must be a valid date in the format Y-m-d\TH:i:s\Z . Must be a date after now . Example: 2025-01-16T11:31:10Z |
| abonent_type | string | yes | Тип абонента. Example: person Must be one of: - m2m_device - person - not_person - not_m2m_device |
| abonent_id | string | yes | ID конечного абонента. Must be a valid UUID. Example: 8ace264b-a78e-488e-b411-af2581bd3f23 |
| abonent_last_name | string | yes | Фамилия конечного абонента. Must be at least 3 characters. Example: Иванов |
| page | integer | yes | Номер страницы. Must be at least 1. Example: 1 |
| limit | integer | yes | Количество записей на странице. Must be at least 1. Example: 30 |

### Получение номера

- **Method:** `GET`
- **Path:** `/api/v1/numbers/{number}`
- **Auth:** required

**URL parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| number | string | no | номер. Example: 79996665522 |

**Example response**

```json
{
  "data": {
    "region_code": "495",
    "phone_number": "1087652",
    "region_name": "Московская область",
    "city_name": "Москва",
    "city_id": 58,
    "data_bindings": {
      "agreement_type": "jur",
      "agreement_id": "9d178a5b-6462-4abe-ae70-13764638cc12",
      "agreement_name": "ООО \"Цветок\"",
      "agreement_inn": "1234567890",
      "agreement_reference_number": "2566hfgHJG522",
      "abonent_type": "person",
      "abonent_id": "9cebebe3-12e9-4573-938a-91feb0a8c109",
      "abonent_name": "Иванов Владимир Михайлович",
      "waiting_abonent_type": null,
      "waiting_abonent_id": null,
      "waiting_abonent_name": null
    },
    "data_status": "ok",
    "data_errors": null,
    "block_status": "active",
    "block_reason": null,
    "trial_time": true,
    "activated_at": "2024-09-03T17:44:47+00:00",
    "to_block_at": null,
    "mnp": false,
    "days_to_block": 0
  },
  "success": true
}
```

### Получение отчета по номерам

- **Method:** `POST`
- **Path:** `/api/v1/numbers/report`
- **Auth:** required

**Body parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| filters | object | yes | Параметры фильтра. |
| name | string | yes | ФИО для ФЛ и ИП или название компании для ЮЛ (часть или полностью). Must be at least 3 characters. Example: Иванов |
| reference_number | string | yes | Номер договора (часть или полностью). Must be at least 3 characters. Example: Д89 |
| region_name | string | yes | Название региона (часть или полностью). Must be at least 3 characters. Example: Москва |
| block_status | string | yes | Статус блокировки номера. Example: all Must be one of: - all - blocked - active |
| number_type | string | yes | Тип номера. Example: abc Must be one of: - abc - def - fed |
| phone_number | string | yes | Номер телефона без кода (часть или полностью). Must be between 3 and 7 digits. Phone number mask min 1 and max 6 digits( ⚹2 or 123456⚹). Example: 1112233 |
| numbers | string[] | yes | Номер не должен начинаться с 7. Must be 10 digits. |
| region_ids | integer[] | yes | Массив идентификаторов регионов. |
| city_ids | integer[] | yes | Массив идентификаторов городов. |
| region_codes | string[] | yes | Массив кодов регионов. Must be 3 digits. |
| data_status | string | yes | Статус данных номера. Example: not_filled Must be one of: - not_filled - need_gu_verification - ok |
| inn | string | yes | ИНН. Must match the regex /^([\d]{10}\|[\d]{12})$/. Example: 7701234567 |
| agreement_id | string | yes | ID договора. Must be a valid UUID. Example: 8ace264b-a78e-488e-b411-af2581bd3f23 |
| to_block_before | string | yes | Верхняя граница даты и времени истечения таймера для блокировки (в UTC). Must be a valid date in the format Y-m-d\TH:i:s\Z . Must be a date after now . Example: 2025-01-16T11:31:10Z |
| abonent_type | string | yes | Тип абонента. Example: person Must be one of: - m2m_device - person - not_person - not_m2m_device |
| abonent_id | string | yes | ID конечного абонента. Must be a valid UUID. Example: 8ace264b-a78e-488e-b411-af2581bd3f23 |
| abonent_last_name | string | yes | Фамилия конечного абонента. Must be at least 3 characters. Example: Иванов |
| columns | object[] | no | Колонки отчета. |
| name | string | no | Название статуса. Example: region_code |
| title | string | no | Название колонки. Example: Код региона |
| cell | object | yes | Значение статусов. |

**Example request body**

```json
{
  "filters": {
    "name": "Иванов",
    "reference_number": "Д89",
    "region_name": "Москва",
    "block_status": "all",
    "number_type": "abc",
    "phone_number": "1112233",
    "numbers": [
      "9023525581"
    ],
    "region_ids": [
      1
    ],
    "city_ids": [
      1
    ],
    "region_codes": [
      [
        "777",
        "781"
      ]
    ],
    "data_status": "not_filled",
    "inn": "7701234567",
    "agreement_id": "8ace264b-a78e-488e-b411-af2581bd3f23",
    "to_block_before": "2025-01-16T11:31:10Z",
    "abonent_type": "person",
    "abonent_id": "8ace264b-a78e-488e-b411-af2581bd3f23",
    "abonent_last_name": "Иванов"
  },
  "columns": [
    {
      "name": "region_code",
      "title": "Код региона"
    }
  ]
}
```

**Example response**

```json
{
  "data": {
    "name": "report_20250521143701.csv",
    "content": "data:text/csv;charset=utf-8,JVBERi0xLjcKCjE..."
  },
  "success": true
}
```

### Активация номера

- **Method:** `POST`
- **Path:** `/api/v1/numbers/{number}/activate`
- **Auth:** required

**URL parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| number | string | no | Номер телефона. Example: 79991234567 |

**Example response**

```json
{
  "data": {},
  "success": true
}
```

### Бронирование номеров партнером

- **Method:** `POST`
- **Path:** `/api/v1/numbers/book`
- **Auth:** required

**Body parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| numbers | string[] | no | Массив номеров телефона. Must be 11 digits. Must start with one of 7 . |
| project_id | string | no | Must be a valid UUID. Example: 17d4a99f-15cb-36ad-ae01-5862627f81b2 |

**Example request body**

```json
{
  "numbers": [
    "79071112233"
  ],
  "project_id": "17d4a99f-15cb-36ad-ae01-5862627f81b2"
}
```

**Example response**

```json
{
  "success": true,
  "data": {
    "booked": [
      "79991111111",
      "79812222222"
    ],
    "failed": {
      "79994444444": "not_found",
      "79995555555": "already_bound",
      "79995555566": "already_bound"
    }
  }
}
```

### Загрузка данных по номерам

- **Method:** `POST`
- **Path:** `/api/v1/numbers/load-data`
- **Auth:** required

**Body parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| load_file_name | string | no | Имя файла с данными по номерам. Example: 202502191012.csv |
| load_file_content | string | no | Файл с данными по номерам (csv) в кодировке base64. Example: data:application/csv;base64,JVBERi0xLjcKCjE... |

**Example request body**

```json
{
  "load_file_name": "202502191012.csv",
  "load_file_content": "data:application/csv;base64,JVBERi0xLjcKCjE..."
}
```

**Example response**

```json
{
  "data": {
    "request_id": "9d178a5b-6462-4abe-ae70-13764638cd12"
  },
  "success": true
}
```

### Отказ партнера от номеров

- **Method:** `POST`
- **Path:** `/api/v1/numbers/refuse`
- **Auth:** required

**Body parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| numbers | string[] | no | Массив номеров телефона. Must be 11 digits. Must start with one of 7 . |

**Example request body**

```json
{
  "numbers": [
    "79071112233"
  ]
}
```

**Example response**

```json
{
  "success": true,
  "data": {
    "updated": [
      "79991111111",
      "79812222222"
    ],
    "failed": {
      "79994444444": "not_found",
      "79995555555": "mnp_not_allowed"
    }
  }
}
```

### Отмена MNP заявки

- **Method:** `POST`
- **Path:** `/api/v1/numbers/{number}/mnp-cancel`
- **Auth:** required

**URL parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| number | string | no | номер. Example: 74952408902 |

**Body parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| comment | string | yes | Комментарий. Example: Тестовый комментарий |

**Example request body**

```json
{
  "comment": "Тестовый комментарий"
}
```

**Example response**

```json
{
  "data": {},
  "success": true
}
```

### Отмена заявки

- **Method:** `DELETE`
- **Path:** `/api/v1/numbers/{number}/applications/{application_id}`
- **Auth:** required

**URL parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| number | string | no | номер. Example: 74952408902 |
| application_id | integer | no | id заявки. Example: 1 |

**Body parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| comment | string | yes | Комментарий. Example: Тестовый комментарий |

**Example request body**

```json
{
  "comment": "Тестовый комментарий"
}
```

**Example response**

```json
{
  "data": {},
  "success": true
}
```

### Поиск заявок

- **Method:** `GET`
- **Path:** `/api/v1/numbers/{number}/applications`
- **Auth:** required

**URL parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| number | string | no | Номер. Example: 79996665522 |

**Query parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| type | string | yes | Тип заявки. Example: abonent Must be one of: - agreement - abonent - free - trilateral - mnp - mnp_cancel |

**Example response**

```json
{
  "data": [
    {
      "number": "74952408902",
      "type": "abonent",
      "data": {
        "abonent_id": "36fc1ac3-8b5d-39c6-b0e4-0a9df361cf95",
        "abonent_type": "person"
      },
      "status": "waiting",
      "comment": "",
      "created_at": "2024-12-19 11:30:31",
      "id": 2
    },
    {
      "number": "74952408902",
      "type": "agreement",
      "data": {
        "agreement_id": "9c6aeca8-cc4e-476b-b87a-12345ef4c717",
        "additional_id": null
      },
      "status": "done",
      "comment": "",
      "created_at": "2024-12-19 11:24:30",
      "id": 1
    }
  ],
  "meta": {
    "total": 2
  },
  "success": true
}
```

### Покупка номеров партнером

- **Method:** `POST`
- **Path:** `/api/v1/numbers/buy`
- **Auth:** required

**Body parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| numbers | string[] | no | Массив номеров телефона. Must be 11 digits. Must start with one of 7 . |
| project_id | string | no | Must be a valid UUID. Example: a801e94e-7fda-35eb-b5c8-d05dc3f40ace |

**Example request body**

```json
{
  "numbers": [
    "79071112233"
  ],
  "project_id": "a801e94e-7fda-35eb-b5c8-d05dc3f40ace"
}
```

**Example response**

```json
{
  "success": true,
  "data": {
    "allocated": [
      "79991111111",
      "79812222222"
    ],
    "failed": {
      "79994444444": "not_found",
      "79995555555": "already_bound",
      "79995555576": "not_allowed_status"
    }
  }
}
```

### Получение заявки

- **Method:** `GET`
- **Path:** `/api/v1/numbers/{number}/applications/{application_id}`
- **Auth:** required

**URL parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| number | string | no | Номер. Example: 74952408902 |
| application_id | string | no | ID заявки. Example: 1 |

**Example response**

```json
{
  "data": {
    "number": "74952408902",
    "type": "agreement",
    "data": {
      "agreement_id": "9c6aeca8-cc4e-476b-b87a-12345ef4c717",
      "additional_id": null
    },
    "status": "done",
    "comment": "",
    "created_at": "2024-12-19 11:24:30",
    "id": 1
  },
  "success": true
}
```

### Получение истории верификаций номера на Портале Госуслуг

- **Method:** `GET`
- **Path:** `/api/v1/numbers/{number}/gu-verification-history`
- **Auth:** required

**URL parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| number | string | no | Номер телефона. Example: 79991234567 |

**Query parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| agreement_id | string | yes | ID договора. Must be a valid UUID. Example: 8ace264b-a78e-488e-b411-af2581bd3f23 |
| reference_number | string | yes | Номер договора. Example: Д89-ФЛ |

### Получение списка загрузок данных

- **Method:** `GET`
- **Path:** `/api/v1/numbers/load-data`
- **Auth:** required

**Query parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| page | integer | yes | Номер страницы. Must be at least 1. Example: 1 |
| limit | integer | yes | Количество записей на странице. Must be at least 1. Example: 30 |

**Example response**

```json
{
  "data": [
    {
      "request_id": "9d178a5b-6462-4abe-ae70-13764638cb12",
      "file_name": "input.csv",
      "processed": true,
      "load_date": "2025-02-25 12:10:00",
      "file_error_name": "20250225121100.csv",
      "file_error_link": "http://develop.did-api.dev3.co.runexis.ru/api/v1/numbers/load-data/9d178a5b-6462-4abe-ae70-13764638cb12"
    },
    {
      "request_id": "9cf1cc27-8fc0-4bd1-9598-670b143bda8b",
      "file_name": "input2.csv",
      "processed": false,
      "load_date": "2025-02-25 12:20:00",
      "file_error_name": null,
      "file_error_link": null
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

### Получение файла с ошибками загрузки данных

- **Method:** `GET`
- **Path:** `/api/v1/numbers/load-data/{request_id}`
- **Auth:** required

**URL parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| request_id | string | no | The ID of the request. Example: voluptatem |

**Example response**

```json
{
  "data": {
    "name": "error.csv",
    "content": "data:text/csv;charset=utf-8,JVBERi0xLjcKCjE..."
  },
  "success": true
}
```

### Портация номеров по MNP

- **Method:** `POST`
- **Path:** `/api/v1/numbers/mnp`
- **Auth:** required

**Body parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| numbers | string[] | no | Массив номеров телефона. Must be 11 digits. Must start with one of 7 . |
| agreement_id | string | no | ID договора. Must be a valid UUID. Example: 8ace264b-a78e-488e-b411-af2581bd3f23 |
| equipment_id | integer | yes | ID оборудования, на которое автоматически отрулится номер в случае успешной портации (по умолчанию отрулится на первое оборудование из списка доступных). Example: 321 |
| mnp_application_file_name | string | no | Имя файла с заявлением на перенос номера mnp. Example: заявление.pdf |
| mnp_application_file_content | string | no | Файл с заявлением на перенос номера mnp (pdf, doc, docx) в кодировке base64. Example: data:application/pdf;base64,JVBERi0xLjcKCjE... |
| duration_mode | string | yes | Режим длительности портации (по умолчанию big). Example: big Must be one of: - big - small |

**Example request body**

```json
{
  "numbers": [
    "79071112233"
  ],
  "agreement_id": "8ace264b-a78e-488e-b411-af2581bd3f23",
  "equipment_id": 321,
  "mnp_application_file_name": "заявление.pdf",
  "mnp_application_file_content": "data:application/pdf;base64,JVBERi0xLjcKCjE...",
  "duration_mode": "big"
}
```

**Example response**

```json
{
  "data": [
    {
      "number": "79071223334",
      "type": "mnp",
      "data": {
        "equipment_id": 321,
        "agreement_id": "0fe38012-49a2-345d-9127-f4dc07b9f76b",
        "mnp_application_file_name": "заявление.pdf"
      },
      "status": "waiting",
      "comment": "",
      "created_at": "2024-12-05 14:37:45",
      "id": 1
    },
    {
      "number": "79079998877",
      "type": "mnp",
      "data": {
        "equipment_id": 321,
        "agreement_id": "0fe38012-49a2-345d-9127-f4dc07b9f76b",
        "mnp_application_file_name": "заявление.pdf"
      },
      "status": "waiting",
      "comment": "",
      "created_at": "2024-12-05 14:37:45",
      "id": 2
    }
  ],
  "success": true
}
```

### Привязка номера к абоненту

- **Method:** `POST`
- **Path:** `/api/v1/numbers/{number}/abonent`
- **Auth:** required

**URL parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| number | string | no | Номер телефона. Example: 79991234567 |

**Body parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| abonent_type | string | no | Тип абонента. Example: person Must be one of: - m2m_device - person - not_person - not_m2m_device |
| abonent_id | string | yes | ID конечного абонента. This field is required when abonent_type is person . Must be a valid UUID. Example: 8ace264b-a78e-488e-b411-af2581bd3f23 |
| notify | boolean | yes | Посылать уведомление в ГУ. Example: false |

**Example request body**

```json
{
  "abonent_type": "person",
  "abonent_id": "8ace264b-a78e-488e-b411-af2581bd3f23",
  "notify": false
}
```

**Example response**

```json
{
  "data": {
    "number": "79771234555",
    "type": "abonent",
    "data": {
      "abonent_id": "9da0bdae-751b-4298-b72d-d88365201d70",
      "abonent_type": "person"
    },
    "status": "waiting",
    "comment": "",
    "created_at": "2024-12-02 14:35:21",
    "id": 557
  },
  "success": true
}
```

### Привязка номера к договору

- **Method:** `POST`
- **Path:** `/api/v1/numbers/{number}/agreement`
- **Auth:** required

**URL parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| number | string | no | The number. Example: 057b1805-8d97-d19b-bd59-43ccc7990fcd |
| c | string | yes | Example: voluptate |

**Body parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| agreement_id | string | yes | This field is required when additional_id is not present. Must be a valid UUID. Example: 8a5b6b53-9082-346b-bf66-1072b9860639 |
| additional_id | string | yes | This field is required when agreement_id is not present. Must be a valid UUID. Example: a1b03038-7aad-3754-a1f1-87866c745e93 |
| token | string | yes | Must be a valid UUID. Example: 4d95f917-e211-3714-b73c-d415c4a40b1b |

**Example request body**

```json
{
  "agreement_id": "8a5b6b53-9082-346b-bf66-1072b9860639",
  "additional_id": "a1b03038-7aad-3754-a1f1-87866c745e93",
  "token": "4d95f917-e211-3714-b73c-d415c4a40b1b"
}
```

**Example response**

```json
{
  "data": {
    "number": "79771251339",
    "type": "agreement",
    "data": {
      "agreement_id": "9da08d4d-b196-41b1-8df5-76180c2b8ee7",
      "additional_id": null,
      "agreement_type": null
    },
    "status": "done",
    "comment": "",
    "created_at": "2024-12-02 08:42:28",
    "id": 527
  },
  "success": true
}
```

### Проверка лимита количества сим карт

- **Method:** `GET`
- **Path:** `/api/v1/numbers/sim/check-limit`
- **Auth:** required

**Query parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| doc_type | string | no | Тип документа. Example: internal_rf_passport Must be one of: - internal_rf_passport - foreign_rf_passport - not_rf_passport - residence_permit |
| doc_series | string | yes | Серия документа (если у документа нет серии, то не заполняется). Example: 1234 |
| doc_number | string | no | Номер документа. Example: 567890 |
| doc_issue_date | string | no | Дата выдачи документа. Must be a valid date in the format Y-m-d . Must be a date before or equal to 2026-08-10 . Example: 2015-05-15 |

**Example response**

```json
{
  "data": {
    "doc_type": "internal_rf_passport",
    "doc_number": "526884",
    "doc_issue_date": "2024-10-22",
    "allowed": 5,
    "registered": 15,
    "doc_series": "5860"
  },
  "success": true
}
```

### Ручная отправка номера в ГЭПС

- **Method:** `POST`
- **Path:** `/api/v1/numbers/{number}/send-geps`
- **Auth:** required

**URL parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| number | string | no | номер. Example: 74952408902 |

**Example response**

```json
{
  "data": {},
  "success": true
}
```

### Удаление всех привязок номера

- **Method:** `POST`
- **Path:** `/api/v1/numbers/{number}/free`
- **Auth:** required

**URL parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| number | string | no | Номер. Example: 79996665522 |

**Body parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| aggr_reject_source | string | yes | Источник расторжения договора. Example: person Must be one of: - person - operator |

**Example request body**

```json
{
  "aggr_reject_source": "person"
}
```

**Example response**

```json
{
  "data": {
    "number": "79771251339",
    "type": "free",
    "data": "",
    "status": "done",
    "comment": "",
    "created_at": "2024-12-02 08:42:22",
    "id": 526
  },
  "success": true
}
```
