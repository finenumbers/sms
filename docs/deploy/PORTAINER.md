# Деплой: Portainer + Nginx Proxy Manager

Прод: один образ `ghcr.io/finenumbers/sms:latest` (linux/amd64, последний GitHub Release). Край — уже существующий NPM. Сеть Docker — `proxy`. Caddy в стеке нет.

Локальный compose (`make up`) сюда не относится. Файл стека: [`deploy/compose/docker-compose.portainer.yml`](../../deploy/compose/docker-compose.portainer.yml).

## Образ

CI пушит в GHCR **только при GitHub Release**. Теги: `vX.Y.Z` и `latest`. Коммит в `main` образ не двигает.

После первого релиза пакет должен быть **public**: GitHub → Packages → `sms` → Package settings → Change visibility → Public. Иначе Portainer не стянет образ без login.

Редеплой: новый Release → в Portainer **Pull and redeploy**. Всегда `:latest`. Не sha, не ручной тег, не сборка на сервере.

## Env в Portainer

Только это:

| Переменная | Смысл |
|---|---|
| `APP_MASTER_KEY` | ≥32 символа, шифрование секретов Настроек |
| `POSTGRES_PASSWORD` | пароль Postgres стека |
| `ADMIN_HOST` | публичный FQDN админки, без порта и `www` |
| `CLIENT_HOST` | публичный FQDN кабинета |
| `API_HOST` | публичный FQDN API и колбэков |

Остальное уже в compose: `COOKIE_SECURE=true`, `RUNEXIS_BASE_URL=https://didapi.runexis.ru`, `POSTGRES_USER/DB=sms`, образ `ghcr.io/finenumbers/sms:latest`.

Первый вход (потом убрать из stack env):

- `SEED_ADMIN_EMAIL`
- `SEED_ADMIN_PASSWORD`

Не класть в env: логин/пароль Runexis, API-ключ SMSC, секрет колбэка SMSC, любые `SMSC_*`. Это Админка → Настройки.

## Portainer

1. Сеть `proxy` уже есть (в ней NPM). Имя сверить в Portainer, не угадывать.
2. Stacks → Add stack. Имя `finenumbers`.
3. Стек из GitHub (предпочтительно) **или** Web editor. Образ на VM не собирать.
   - **Repository:** `https://github.com/finenumbers/sms`, ветка `main` или тег `v1.0.0`. **Compose path** (от корня репо, не имя файла): `deploy/compose/docker-compose.portainer.yml`. Путь `docker-compose.portainer.yml` в корне не существует — Portainer тогда пишет `no such file or directory`.
   - **Web editor:** вставить содержимое [`docker-compose.portainer.yml`](../../deploy/compose/docker-compose.portainer.yml) с GitHub. Имя файла в редакторе не задавать.
4. Environment variables — пять обязательных из таблицы. Порты стека не публиковать.
5. Deploy. Дождаться `migrate` (exit 0), затем `api` и `worker`.
6. Контейнер API: `finenumbers-api`, одна replica, слушает `:8080` на `proxy`.

Проверка с хоста NPM-сети: `https://admin.{domain}/healthz` после шага NPM. У контейнера `api` нет curl (distroless) — healthcheck в compose не ставить.

## NPM

Три Proxy Host → `finenumbers-api:8080` (имя контейнера, не IP и не `localhost`).

| Host | Scheme | SSL |
|---|---|---|
| `admin.{domain}` | https | Let’s Encrypt, Force SSL |
| `client.{domain}` | https | Let’s Encrypt, Force SSL |
| `api.{domain}` | https | Let’s Encrypt, Force SSL |

На каждом хосте:

- Websockets **off**
- **Не переписывать `Host`.** Оставить `$host`. `proxy_set_header Host $proxy_host` ломает маршрутизацию, CSRF и cookie.
- Cache Assets **выкл.** на admin и client (иначе закэшируется `index.html`)
- Advanced:

```nginx
client_max_body_size 50m;
```

На `api.{domain}`: POST `/internal/runexis/*` и `/internal/smsc/callback` без Basic Auth и без WAF («Block Common Exploits» сначала выкл.). Access log NPM светит ingress token Runexis в URL — логи не публиковать.

Логин только `https://admin.{domain}` (не IP, не `:8080`).

## После старта

1. Войти в админку. Убрать `SEED_ADMIN_*` из stack env и обновить стек.
2. Настройки: почта/пароль агента Runexis → «Проверить обмен с DIDAPI».
3. Базовый URL колбэков = `https://api.{domain}` → зарегистрировать DLR/hook.
4. SMSC: API-ключ и секрет колбэка (тот же, что у старого HLR). «Проверить связь».
5. В кабинете SMSC.ru URL колбэка — значение `smsc_callback_url` из Настроек (`https://api.{domain}/internal/smsc/callback`). Не переключать, пока не готов cutover.
6. `lookup_enabled` в этом выкате **не включать**.

## Редеплой

1. GitHub → Release (двигает `ghcr.io/finenumbers/sms:latest`).
2. Portainer → стек → Editor → **Pull and redeploy**.
3. `migrate` отработает до старта api/worker. Данные в volume `postgres_data` не трогать.

Откат образа: выставить `FINENUMBERS_IMAGE=ghcr.io/finenumbers/sms:vX.Y.Z` предыдущего релиза, Pull and redeploy, затем вернуть дефолт `:latest` на следующем релизе.

## Бэкап

Сервис `backup` пишет `pg_dump` в volume `postgres_backups`, ротация `BACKUP_KEEP_DAYS` (7). Restore: `gunzip -c sms-….sql.gz | psql "$DATABASE_URL"`. Redis эфемерный.
