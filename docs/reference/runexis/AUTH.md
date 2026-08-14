# Runexis DIDAPI — Auth

Platform authentication for the agent account used by Finenumbers SMS Service.

Base URL: `https://didapi.runexis.ru`

## Endpoints

### Аутентификация

- **Method:** `POST`
- **Path:** `/api/v1/login`
- **Auth:** not required

**Body parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| email | string | no | Email пользователя. Must be a valid email address. Example: ivan.ivanov@example.com |
| password | string | no | Пароль пользователя. Example: GF[Eg5TfQp<cXo |

**Example request body**

```json
{
  "email": "ivan.ivanov@example.com",
  "password": "GF[Eg5TfQp<cXo"
}
```

**Example response**

```json
{
  "data": {
    "token": "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpYXQiOjIwNjAzMzY4ODAsImV4cCI6MjIxODE4OTY4MCwic3ViIjoxfQ.GgOp45IV-14cBYZ6nnp1XNrtfd9qqXKqXg7enROrMRc",
    "refresh_token": "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpYXQiOjE3NDQ4ODAxMzUsImV4cCI6MTc0NDg5ODEzNSwic3ViIjoxfQ.a6GXM7XCpXZz6APWzfTHW_Shl2RRMOZsSk614SmawCM",
    "token_expire": "2025-07-21T09:21:40.000Z",
    "refresh_token_expire": "2025-08-20T09:11:40.000Z"
  },
  "success": true
}
```

### Обновление токена

- **Method:** `POST`
- **Path:** `/api/v1/refresh`
- **Auth:** not required

**Body parameters**

| Name | Type | Optional | Description |
|---|---|---|---|
| token | string | no | refresh_token пользователя. Example: eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpYXQiOjIwNjAzMzY4ODAsImV4cCI6MjIxODE4OTY4MCwic3ViIjoxfQ.GgOp45IV-14cBYZ6nnp1XNrtfd9qqXKqXg7enROrMRc |

**Example request body**

```json
{
  "token": "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpYXQiOjIwNjAzMzY4ODAsImV4cCI6MjIxODE4OTY4MCwic3ViIjoxfQ.GgOp45IV-14cBYZ6nnp1XNrtfd9qqXKqXg7enROrMRc"
}
```

**Example response**

```json
{
  "data": {
    "token": "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpYXQiOjIwNjAzMzY4ODAsImV4cCI6MjIxODE4OTY4MCwic3ViIjoxfQ.GgOp45IV-14cBYZ6nnp1XNrtfd9qqXKqXg7enROrMRc",
    "refresh_token": "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpYXQiOjE3NDQ4ODAxMzUsImV4cCI6MTc0NDg5ODEzNSwic3ViIjoxfQ.a6GXM7XCpXZz6APWzfTHW_Shl2RRMOZsSk614SmawCM",
    "token_expire": "2025-07-21T09:21:40.000Z",
    "refresh_token_expire": "2025-08-20T09:11:40.000Z"
  },
  "success": true
}
```

### Получение текущего пользователя

- **Method:** `GET`
- **Path:** `/api/v1/me`
- **Auth:** required

**Example response**

```json
{
  "data": {
    "name": "test_user",
    "email": "user@example.com",
    "created_at": "2024-01-01"
  },
  "success": true
}
```

### Удаление всех refresh token пользователя

- **Method:** `POST`
- **Path:** `/api/v1/revoke-all`
- **Auth:** required

**Example response**

```json
{
  "data": {},
  "success": true
}
```
