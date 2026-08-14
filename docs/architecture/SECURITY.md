# Безопасность

## Секреты

- `APP_MASTER_KEY` — в env, не в git. Шифрование пароля Runexis, пароля/API-ключа/секрета SMSC и секретов webhook (AES-256-GCM), pepper API-ключей
- Runexis и SMSC: логин/пароль (или API-ключ) и `callback secret` — Админка → Настройки, не env. Процесс стартует без них (fail-closed). Тот же аккаунт и секрет SMSC, что у старого HLR
- Пароли пользователей: Argon2id (PHC)
- API secret показывается один раз при создании

## Rate limits (Redis token bucket)

- Login: 5 / 5 мин на IP+email
- Public API: per-credential, дефолт 10 rps / burst 20
- Cookie send: Redis token bucket `rl:http:client:{id}` = `client_rps_default`
- Worker: `rl:provider` (`provider_rps`) + `rl:worker:client:{id}` (`client_rps_default`); 429 провайдера → `Drain(rl:provider)`
- Ingress: `rl:ingress:{ip}` 200/с; при недоступности Redis — fail-open (не теряем DLR/MO)
- Campaign start: Redis `SET NX` `campaign:start:{client_id}` на время запроса (один in-flight start на клиента)

Ответ 429 + `Retry-After`. Лимиты не заменяют проверку текущего assignment.

## Host split

`ADMIN_HOST` / `CLIENT_HOST` / `API_HOST` режут поверхности. `/metrics` с публичных FQDN — 404. Неизвестный Host (имя контейнера на сети `proxy`) отдаёт только `/healthz`, `/readyz`, `/metrics`. NPM должен пробрасывать `Host` публичного FQDN.

## Аудит

Пишем: CRUD/suspend client, assign/unassign, settings, API key create/revoke, campaign start/cancel, login success/failure (без пароля).

Не пишем: тело SMS, пароль Runexis, полный API secret.

Retention audit: `audit_retention_days` (дефолт 24 мес). Операционный журнал `ops_events`: `ops_retention_days` (дефолт 14). Worker чистит `audit_log`, `ops_events` и SMS по Settings раз в час.

Админ `GET /admin/v1/logs` не пишет тела SMS, пароли, `token`/`refresh_token`, cookie, Authorization. DIDAPI-вызовы (включая `/sms/send`) пишут redacted `request`/`response` в `detail` — envelope `message` и `request_id` сохраняются, чтобы разбирать инциденты с техподдержкой Runexis. Поле statistic/MO `message` (текст SMS) маскируется. Успешный `GET /sms/statistic` 200 с `request_id=reconcile` (heartbeat inbox) в `ops_events` не пишется; 4xx/5xx и statistic по `job:{uuid}` пишутся.

## 152-ФЗ

Хостинг в РФ. SMS text — ПДн в `sms_messages`. Номера проверок — ПДн в `lookup_items.phone_e164` (retention их не чистит). В `ops_events` маскировать MSISDN lookup. Retention raw SMSC — `lookup_retention_days` (90). CSV: UTF-8 (BOM) и Windows-1251.
