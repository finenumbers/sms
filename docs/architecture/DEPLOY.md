# Деплой

v1: Docker Compose / Portainer на 1–2 VM в РФ (Selectel / Yandex Cloud / свой сервер). Kubernetes не в v1.

Край — **уже существующий Nginx Proxy Manager**. Caddy в стеке нет. TLS, Let’s Encrypt и три публичных хоста настраиваются в NPM. Процесс `api` (`container_name: finenumbers-api`) слушает `:8080` на сети `proxy` и отдаёт Admin/Client SPA по `Host`.

## Сервисы

`api` (HTTP + SPA), `worker`, `postgres`, `redis`, `backup` (`pg_dump` loop), `migrate` (one-shot). Prometheus — профиль `obs` только локально, Grafana/Alertmanager не в v1.

## Локально

[`deploy/compose/docker-compose.yml`](../../deploy/compose/docker-compose.yml) публикует Postgres/Redis/`api:8080`. UI: Vite.

```bash
cp deploy/compose/.env.example deploy/compose/.env
# задать APP_MASTER_KEY, SEED_ADMIN_*
make up
cd web && npm install && npm run dev:admin
# http://admin.sms.localhost:5173  (не 127.0.0.1 — Host должен совпасть с ADMIN_HOST)
```

```bash
curl -s http://127.0.0.1:8080/healthz
curl -s http://127.0.0.1:8080/metrics
curl -s -H 'Host: admin.sms.localhost' http://127.0.0.1:8080/admin/v1/health
curl -s http://127.0.0.1:8080/admin/v1/health   # 404: unknown Host, когда *_HOST заданы
```

`make up-prod` — тот же стек без опубликованных портов (для проверки overlay).

Операторский чеклист прода: [`docs/deploy/PORTAINER.md`](../deploy/PORTAINER.md). Образ `ghcr.io/finenumbers/sms:latest` — только с GitHub Release, linux/amd64.

## Portainer + NPM

Кратко. Полные шаги — [`PORTAINER.md`](../deploy/PORTAINER.md).

1. Сеть Docker `proxy` уже есть (NPM в ней). Имя сверить в Portainer, не угадывать.
2. CI пушит образ в GHCR только на Release: `ghcr.io/finenumbers/sms:<tag>` и `:latest`.
3. Stack из [`docker-compose.portainer.yml`](../../deploy/compose/docker-compose.portainer.yml). Env (только критичное):
   - `APP_MASTER_KEY` (≥32), `POSTGRES_PASSWORD`
   - `ADMIN_HOST` / `CLIENT_HOST` / `API_HOST` = публичные FQDN **без порта**, как в NPM (без `www`)
   - образ по умолчанию `ghcr.io/finenumbers/sms:latest` (`pull_policy: always`)
   - `COOKIE_SECURE=true` в compose
   - опционально `SEED_ADMIN_EMAIL` / `SEED_ADMIN_PASSWORD` — **убрать после первого логина**
   - `RUNEXIS_BASE_URL` по умолчанию `https://didapi.runexis.ru`
   - логин/пароль Runexis и SMSC **не** в env — Админка → Настройки
   - **Чеклист:** в stack env **нет** полей `SMSC_*` (`SMSC_LOGIN`, `SMSC_PASSWORD`, `SMSC_APIKEY`, `SMSC_CALLBACK_SECRET` и т.п.). Креды только в Настройках.
4. Порты стека не публиковать. `container_name: finenumbers-api` — один replica.
5. NPM: три Proxy Host → `finenumbers-api:8080`, SSL Let’s Encrypt, Force SSL, Websockets off.
   - **Не переписывать `Host`.** Дефолт `$host`. `proxy_set_header Host $proxy_host` ломает RestrictSurface, CSRF и cookie.
   - Advanced на admin, client и api: `client_max_body_size 50m;` (lookup CSV до 50 MiB; SMS CSV меньше).
   - Cache Assets выкл. на admin/client (иначе закэшируется `index.html`).
   - На `api.{domain}`: POST `/internal/runexis/*` и `/internal/smsc/callback` без auth и без WAF («Block Common Exploits» сначала выкл.). Access log NPM светит ingress token Runexis в URL — не публиковать логи.
   - Health: `https://{host}/healthz`. У контейнера `api` нет curl (distroless) — compose healthcheck не ставить.
6. Логин только `https://admin.{domain}` (не IP, не `:8080`).
7. Настройки: email/пароль агента → «Проверить обмен с DIDAPI» (`/me` обязателен; statistic — best-effort). `callback_base_url` = `https://api.{domain}` → зарегистрировать dlr/hook.
8. Restore: volume `postgres_backups`, `gunzip -c sms-….sql.gz | psql "$DATABASE_URL"`.

Принятый риск: сеть `proxy` общая, пиры могут подставить `Host` и `X-Real-IP`. Смягчение: порты не опубликованы; unknown Host режется; cookie + CSRF + API keys.

## Метрики

`make up-obs` → Prometheus `127.0.0.1:9090`. Скрейп **с docker-сети**, не через NPM: API `http://api:8080/metrics`, worker `http://worker:9091/metrics`. `/readyz` не зависит от SMSC.

Lookup / SMSC (не смешивать с `fn_send_jobs` / `fn_billing_open_holds`):

| Метрика | Смысл |
|---|---|
| `fn_lookup_items{status=}` | глубина очереди items |
| `fn_lookup_jobs{status=}` | задания |
| `fn_lookup_holds_open` | открытые HOLD lookup (`CountOpenLookupHolds`) |
| `fn_smsc_requests_total{kind,status}` | send/status/cost/balance |
| `fn_smsc_error_code_9_total` | flood SMSC |
| `fn_smsc_callback_lag_seconds` | возраст необработанного raw callback |
| `fn_smsc_balance` | баланс кабинета из Redis `sms:provider:smsc:balance` (нет кредов / нет кэша — метрики нет, не ноль) |
| `fn_lookup_enabled` / `fn_smsc_configured` | флаги для алертов |

Правила в [`alerts.yml`](../../deploy/compose/alerts.yml): `SendJobsDead`, `LookupEnabledWithoutSMSC`, `LookupHoldsStuck`, `SMSCErrorCode9`, `SMSCCallbackLag`. Alertmanager не в v1 — смотреть вкладку Alerts.

## Правила

- `.env` не в git
- Postgres/Redis/API в проде не публиковать наружу
- `migrate` отрабатывает до старта api
- CORS allowlist только для `api.{domain}/v1` (`CORS_ALLOW_ORIGINS`, пустой = без браузерного CORS)
- Pepper API-ключей: `API_KEY_PEPPER` или `APP_MASTER_KEY`
- Бэкап: сервис `backup`, `pg_dump` в volume `postgres_backups`, ротация `BACKUP_KEEP_DAYS` (дефолт 7). Redis эфемерный
- Retention: worker раз в час чистит SMS/`audit_log`/`ops_events` по Settings
- Prod: образ из registry, stack up в Portainer, NPM руками по чеклисту выше
- SMSC (тот же аккаунт, что старый HLR): логин/пароль или API-ключ и секрет колбэка — Админка → Настройки, **не env**. После сохранения «Проверить связь». URL колбэка в кабинете SMSC.ru вручную: значение `smsc_callback_url` (`https://api.{domain}/internal/smsc/callback`)

## Грязная миграция

`schema_migrations.dirty = true` — api/worker к этой БД не поднимать (или сразу остановить). Процедуры в коде нет, чинить руками.

1. Остановить контейнеры `api` и `worker`, которые пишут в эту БД. Не чинить dirty, пока процесс ещё пишет.
2. Прочитать failed-файл миграции в `db/migrations/` и фактическое состояние схемы (`\d`, `schema_migrations`).
3. Починить SQL или откатить частично применённые объекты так, чтобы схема совпала с **последней успешно применённой** версией.
4. `migrate force <version>` на эту успешную версию (не на грязную и не на целевую).
5. `migrate up`.
6. Только потом стартовать api/worker.

Не `migrate drop`. Не `force` на номер упавшей миграции, если её объекты наполовину применены.

## Cutover SMSC (тот же аккаунт)

Параллель со старым HLR запрещена: один callback URL. До смены URL в SMSC.ru новый SMS уже в проде, `lookup_enabled=false` — в SMSC ничего не шлём.

1. Задеплоить SMS. В Настройках — **те же** логин/пароль (или API-ключ) и **тот же** секрет колбэка, что у старого HLR. Другой секрет → после смены URL все колбэки 401. Полей `SMSC_*` в Portainer нет.
2. Настройки → «Проверить связь» (balance + локальная подпись). Боевые HLR/Ping с SMS не слать.
3. Тарифы `hlr` / `silent_sms` + один пилот-клиент + деньги в кошельке SMS (это не баланс SMSC и не кошелёк старого HLR). Скоуп `lookup:*` на ключ пилота — руками.
4. **Слив старого HLR:** запретить новые задания. Worker HLR оставить (poll хвоста). Ждать пустых `QUEUED`/`RESERVED`/`PENDING` (до `check_timeout_sec`, час). URL не переключать, пока хвост жив.
5. Хвост пуст → в кабинете SMSC.ru URL = `smsc_callback_url` из Настроек (`https://api.{domain}/internal/smsc/callback`). Сразу стоп API и worker старого HLR. Назад «на минутку» не переключать.
6. На стенде уже должны быть доказаны HOLD→DEBIT, HOLD→RELEASE и ingress. Только тогда в Настройках `lookup_enabled=true`. Одна HLR и одна Ping (reachable + unreachable). Сверка: колбэк на SMS, в леджере DEBIT доли, в ЛК нет бренда SMSC.
7. Webhook, XLSX, небольшой CSV. Затем остальные клиенты и скоупы ключей руками.

Откат в окне: `lookup_enabled=false` (pending дожимаются). URL вернуть на старый HLR **только если** на SMS ещё не было успешного submit. После первого боевого submit откат URL оставит новые задания без колбэка (останется poll на SMS).

Дефолты: `check_timeout_sec=3600`, `poll_interval_sec=30`, `lookup_retention_days=90`, `max_csv_rows=100000`, `max_csv_bytes=50MiB`, `max_batch_phones=1000`, `poll_max_attempts=120`, `submit_batch_size=50`.
