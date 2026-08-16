# Авторизация

Публичной регистрации нет. Клиентов создаёт только Admin.

## Principals

- **AdminUser** — Argon2id, роль `admin`. Первый пользователь: seed из `SEED_ADMIN_EMAIL` / `SEED_ADMIN_PASSWORD`, если таблица пуста. Сброс пароля клиента — только админ (нет public reset в v1)
- **ClientUser** — роль `owner` (минимум один при создании Client; админ может добавить ещё owner с тем же доступом в ЛК). Тенант = `client_id` в сессии. Поле `name` (ФИО). Сброс пароля и отключение — только админ; сессии отзываются у этого пользователя.
- **Client API** — `Authorization: Bearer fnk_live_{prefix}_{secret}`. Ключи в v1 выдаёт Admin. В ЛК — read-only prefix / status / last_used. Hash: SHA-256(pepper ∥ secret). Scopes: `sms:send`, `sms:read`, `campaigns:write`. `allowed_cidrs` пустой = любой IP. Pepper = `API_KEY_PEPPER` или `APP_MASTER_KEY`.

Публичный `/v1` на `api.{domain}`: CORS allowlist (`CORS_ALLOW_ORIGINS`, пустой = без браузерного CORS). Опциональный заголовок `Idempotency-Key` на `POST /v1/messages` (повтор с тем же телом отдаёт сохранённый 202; другое тело — 409). Rate limit per-credential: 10 rps / burst 20.

## Сессии

Таблица `sessions` в PostgreSQL. Audience `admin` | `client`. TTL 12h, sliding. Logout = `revoked_at`. Redis не используется.

Cookie: `__Host-fn_admin_sid` / `__Host-fn_client_sid` (HttpOnly, Secure, SameSite=Lax, Path=/, без Domain). Cookie ставятся только на `admin.{domain}` / `client.{domain}`. Go режет поверхности по `ADMIN_HOST` / `CLIENT_HOST` / `API_HOST`: cookie-API только с своего хоста; неизвестный Host (в т.ч. `finenumbers-api:8080`) не отдаёт API и SPA — только `/healthz`, `/readyz`, `/metrics`. NPM должен пробрасывать `Host` публичного FQDN (`$host`), не имя контейнера.

Sliding 12h: PG двигает `expires_at`; authenticated-запрос заново ставит `Set-Cookie` с `MaxAge=TTL`.

CSRF на cookie-API: обязательный заголовок `X-Requested-With`. Если браузер прислал `Origin` или `Referer`, их host должен совпасть с `Host` запроса. Без Origin/Referer (curl) достаточно заголовка.

Email: `UNIQUE (LOWER(email))` в каждой таблице principal. Один email может быть и админом, и клиентом. Удаление клиента в админке затирает email пользователей (`deleted-{uuid}@invalid`), поэтому тот же адрес можно выдать новому клиенту.

RLS — не v1: фильтр `client_id` в каждом запросе + тесты.
