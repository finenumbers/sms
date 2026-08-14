# Очереди рассылок

Единый путь: запись в БД → worker → Runexis. UI не ждёт DIDAPI.

## Outbox

`send_jobs` + `FOR UPDATE SKIP LOCKED`.

Reclaim **direction jobs**: `processing` старше TTL → снова `pending` (PATCH направлений идемпотентен).

Reclaim **send jobs**: `processing` старше TTL → `uncertain` + `last_error=uncertain:need_stat`, `attempt = GREATEST(attempt, 1)`. Не возвращаем в `pending`: HTTP send мог уже уйти, слепой повтор даёт двойную SMS. Дальше — statistic; повторный POST только если последняя попытка была timeout/сеть (не явный HTTP 5xx).

Статусы: `pending` | `processing` | `done` | `retry` | `uncertain` | `dead`.

На assign номера — отдельный outbox `number_direction_jobs` (тот же SKIP LOCKED). Снимок directions из Settings на момент назначения. 4xx кроме 429 → `dead`; 429/5xx/timeout/`not configured` → `retry`. Unassign job не создаёт.

## Fair share

- Глобальный `provider_rps` (Settings)
- Per-client лимит в v1 = глобальный `client_rps_default` (отдельного per-client `client_rps` нет)
- Claim round-robin по `client_id`

## Анти-дубли

- 4xx кроме 429 → `failed`, без retry
- 429 → backoff, уменьшить bucket
- Timeout / сеть (HTTP не ответил) → `uncertain`: сверка `GET /sms/statistic` по from/to/окну/тексту. Нашли `sms_id` → `accepted`. Иначе один повторный POST, затем `dead` + алерт
- Явный HTTP 5xx (JSON envelope) → тоже `uncertain` + statistic. Нашли `sms_id` → `accepted`. Не нашли → `dead` **без** второго POST (провайдер уже ответил; повтор даёт дубль)
- Каждая попытка — строка `provider_send_attempts`
- Park `retry`/`uncertain`/`dead` и direction `dead` пишутся в `ops_events` (категория `queue`); исходящий DIDAPI — категория `didapi`. HTTP `request_id` и `job:{uuid}` — разные идентификаторы.

## Lookup (HLR / Ping)

Отдельные таблицы `lookup_jobs` / `lookup_items`, не `send_jobs`. Свой цикл Tick (не хвост SMS-loop), бюджет и fair по `client_id`. SMSC rate limit — свой Redis bucket, не `rl:provider`. Подробно: [`LOOKUP.md`](LOOKUP.md).

## Кампании

CSV/список → `campaign_recipients` (батч `UNNEST … ON CONFLICT DO NOTHING`, unique `(campaign_id, to_msisdn)`). Только в `draft`. Лимит 10⁵ получателей.

Start → `queued` (202). `from_msisdn` и `text` после выхода из `draft` immutable. Worker переводит `queued` → `running` и батчами, **fair по `client_id`**, создаёт `sms_messages` + `send_jobs`. Cancel ставит `cancelled` и не клеймит новые recipients (`pending` → `skipped`); уже созданные send jobs дожимаются. Счётчики — периодическая агрегация по `sms_messages`, не триггер на каждую DLR. `completed`, когда нет `pending` recipients и нет открытых send jobs.
