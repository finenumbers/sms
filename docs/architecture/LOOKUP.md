# Lookup jobs

Очередь — таблица + `FOR UPDATE SKIP LOCKED`, не BullMQ. `lookup.Worker.Tick` крутится в **своём** цикле 200 ms (ModeAPI, ModeWorker и ModeAll), не внутри SMS-loop: начатый вызов SMSC режется 3 с и иначе ставит `send_jobs`. SKIP LOCKED безопасен, если API и worker тикают оба. `balance.php` — отдельный тикер 2 мин, не из Tick.

Внутри tick: CSV parse → submit → poll → stored callbacks → HLR enrich → finalize → webhook deliver → reconcile + `ReapOpenLookupHolds`.

| Константа | Значение | Где |
|---|---|---|
| `check_timeout_sec` | 3600 | Settings |
| `poll_interval_sec` | 30 | Settings (и порог stale pending) |
| `poll_max_attempts` | 120 | константа кода |
| `submit_batch_size` | 50 | константа кода |
| `max_csv_rows` | 100000 | Settings |
| `max_csv_bytes` | 50 MiB | Settings |
| `max_batch_phones` | 1000 | Settings (JSON paste/API, не CSV preview) |

100k items: `UNNEST` батчами в одной TX с HOLD. Statement timeout для этой TX поднять явно.

Краш после accept SMSC: как `uncertain` у SMS — тот же `id`, poll, не второй send. Persist pending request до/вместе с вызовом.

`lookup_enabled=false`: HTTP «услуга выключена», новый submit нет, уже `pending` дожимаются.

Метрики: `fn_lookup_items` / `fn_lookup_jobs` / `fn_lookup_holds_open` (не `fn_billing_open_holds`), `fn_smsc_*`. Кэш баланса — [`BALANCE.md`](../reference/smsc/BALANCE.md). Алерты — [`alerts.yml`](../../deploy/compose/alerts.yml).

Ручной admin finalize — no-op, пока есть нетерминальные items.
