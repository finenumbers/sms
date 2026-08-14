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
5. Worker нормализует адаптером (`internal/runexis.ParseCallbacks`) по provisional-фикстурам полей statistic (`sms_id`, `sent`, `delivered`, `sender_number`, `receiver_number`, `message`). Неузнанный payload: `processed_at` + `parsed.skipped=unrecognized`; raw остаётся в админке. Live capture должен заменить файлы в `docs/reference/runexis/fixtures/`.

Идемпотентность: `sha256(method + path + query + body)`.

DLR с `sms_id` обновляет исходящее (`delivered` / `sent` / `failed`). MO без assignment: `client_id` NULL, в ЛК не показывается. Reconcile statistic по-прежнему подстраховывает `accepted|sent` старше порога и inbox `incoming=true`. Просмотр raw: `GET /admin/v1/callbacks`.

Ротация токена: `PATCH /admin/v1/settings` с `rotate_ingress_token: true` — plaintext один раз в ответе, в БД только hash. Регистрация URL: `POST /admin/v1/settings/runexis/callbacks` с тем же токеном → `PATCH` глобальных `dlr-url` и `hook-url`.

Lifecycle `/api/v1/webhooks` Runexis в v1 не подключаем. Исходящие вебхуки **проверок** (HLR/Ping) — [`WEBHOOKS.md`](WEBHOOKS.md).

## SMSC callback

`https://api.{domain}/internal/smsc/callback` — GET/POST, подпись md5/sha1 от секрета из Настроек, не токен в path. Capture-first: raw → 200 → worker. Неизвестный id → 200 без apply. Канон: [`reference/smsc/CALLBACK.md`](../reference/smsc/CALLBACK.md). Хост-фильтр: путь разрешён на `API_HOST` и запрещён на admin/client.
