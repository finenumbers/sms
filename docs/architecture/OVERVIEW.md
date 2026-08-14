# Обзор системы

Платформа мультиплексирует клиентов через назначение DEF-номеров. Один агентский аккаунт Runexis принадлежит **только** этой платформе.

## Хосты

- `admin.{domain}` — Admin SPA + cookie-API `/admin/v1`; cookie `__Host-fn_admin_sid`
- `client.{domain}` — Client LK + cookie-API `/client/v1`; cookie `__Host-fn_client_sid`
- `api.{domain}` — Client API `/v1` (Bearer-ключи, CORS allowlist) и ingress `/internal/runexis/*`

Cookie на `api.` не ставятся. `__Host-` = Secure, Path=/, без Domain.

Край в проде — уже существующий **Nginx Proxy Manager** (сеть Docker `proxy`). Caddy в стеке нет. TLS и публичные DNS — у NPM. Процесс `api` слушает `:8080` и сам отдаёт статику SPA по `Host` (без Node в проде).

Go режет поверхности, если заданы `ADMIN_HOST` / `CLIENT_HOST` / `API_HOST`:

- `admin.{domain}` — SPA (GET/HEAD), `/admin/v1`, `/healthz`, `/readyz`
- `client.{domain}` — SPA (GET/HEAD), `/client/v1`, `/healthz`, `/readyz`
- `api.{domain}` — `/v1`, `/internal/runexis`, `/internal/smsc`, `/healthz`, `/readyz`
- неизвестный Host — только `/healthz`, `/readyz`, `/metrics` (scrape с docker-сети). API и SPA с `Host: finenumbers-api` — 404
- `/metrics` с публичных FQDN — 404

Локально: `admin.sms.localhost`, `client.sms.localhost`, `api.sms.localhost`. UI-dev: Vite на `http://admin.sms.localhost:5173` (прокси на `:8080`, `changeOrigin: false`). `*.localhost` — secure context, `__Host-` cookie с `COOKIE_SECURE=true` работает по HTTP.

## Префиксы HTTP

| Prefix | Auth | Назначение |
|---|---|---|
| `/healthz`, `/readyz` | нет | liveness / readiness |
| `/admin/v1` | cookie admin | админка, в т.ч. `GET /logs` |
| `/client/v1` | cookie client | ЛК |
| `/v1` | API key | публичный Client API |
| `/internal/runexis` | ingress token в path | DLR/MO |
| `/internal/smsc` | подпись SMSC | HLR/Ping callback |
| `/metrics` | нет (только не-публичный Host) | Prometheus scrape |

## Процессы

- `cmd/sms api` — HTTP + SPA
- `cmd/sms worker` — directions, campaign fan-out, DLR/MO normalize, затем send/outbox и retention
- `cmd/sms all` — оба (dev / одна VM)
- `cmd/sms migrate` — `migrate up`

Handlers не вызывают DIDAPI для send. Admin Settings может звать DIDAPI для проверки обмена и регистрации callback URL. Send только из worker: `sms_messages` (`queued`) + `send_jobs` (`pending`) → **202** + наш UUID. Ingress: persist raw, затем 200; worker нормализует DLR/MO.

`/metrics` (Prometheus) на процессе API; worker — `METRICS_ADDR` (compose `:9091`). NPM `/metrics` не должен проксировать (Go отдаёт 404 на публичных хостах).

## Пакеты

`internal/identity`, `apikeys`, `inventory`, `messaging`, `campaigns`, `outbox`, `runexis`, `smsc`, `lookup`, `webhooks`, `ingress`, `settings`, `billing`, `audit`, `ops`, `ratelimit`, `metrics`, `retention`, `http/{admin,client,publicapi,ingress}`.
