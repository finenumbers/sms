# Callback ingress

Контракт DLR/MO в эталоне Runexis **не документирован** (см. [`GAPS.md`](../reference/runexis/GAPS.md)).

## URL

`https://api.{domain}/internal/runexis/{dlr|mo}/{ingress_token}`

Глобальная регистрация: `PATCH /api/v1/sms/dlr-url` и `/sms/hook-url`. Per-number — не v1.

## Поведение

1. Принимать POST, PUT и GET (query как payload)
2. Неверный или отсутствующий `ingress_token` → **404** (не 401)
3. **Синхронно записать raw** в `provider_callback_events` (path без токена: `/internal/runexis/{dlr|mo}/*`)
4. Ответить **200** только после успешного persist. Дубликат по `idempotency_key` тоже 200. Ошибка БД → 5xx, не 200
5. Worker нормализует адаптером (`internal/runexis.ParseCallbacks`). Live DLR: `id` (= send `data.id` / statistic `sms_id`) + `message_status`. Вендор 2026-08-19: `0`/`2` → `delivered`, `1`/`3` → `failed`; иной код только в `provider_status`. Поля statistic `sent`/`delivered` по-прежнему читаются, если придут. Неузнанный payload: `processed_at` + `parsed.skipped=unrecognized`; raw остаётся в админке. MO — ещё provisional.

Идемпотентность: `sha256(method + path + query + body)`.

DLR с `id`/`sms_id` обновляет исходящее: `message_status` `0`/`2` → `delivered`; `1`/`3` → `failed`; иначе статус SMS не понижается (`UpdateSmsMessageFromStatistic` ELSE оставляет текущий). Reconcile statistic подстраховывает `accepted|sent` старше порога и inbox `incoming=true`. Просмотр raw и `parsed`: `GET /admin/v1/callbacks/{id}`.

Ротация токена: `PATCH /admin/v1/settings` с `rotate_ingress_token: true` — plaintext один раз в ответе, в БД только hash. Регистрация URL: `POST /admin/v1/settings/runexis/callbacks` с тем же токеном → `PATCH` глобальных `dlr-url` и `hook-url`. Регистрация отклоняет `localhost` / http / частные IP — Runexis должен достучаться до `https://{API_HOST}`.

Lifecycle `/api/v1/webhooks` Runexis в v1 не подключаем. Исходящие вебхуки **проверок** (HLR/Ping) — [`WEBHOOKS.md`](WEBHOOKS.md).

## SMSC callback

`https://api.{domain}/internal/smsc/callback` — GET/POST, подпись md5/sha1 от секрета из Настроек, не токен в path. Capture-first: raw → 200 → worker. Неизвестный id → 200 без apply. Канон: [`reference/smsc/CALLBACK.md`](../reference/smsc/CALLBACK.md). Хост-фильтр: путь разрешён на `API_HOST` и запрещён на admin/client.
