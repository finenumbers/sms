# Finenumbers SMS Service — Documentation

Эталонная и рабочая документация для реализации сервиса.

## Как устроена документация

| Path | Role |
|---|---|
| [`vendor/runexis/`](vendor/runexis/) | **Immutable эталон** — offline HTML Runexis DIDAPI (+ assets), как получен от поставщика |
| [`reference/runexis/`](reference/runexis/) | Структурированный reference, извлечённый из эталона для разработки |
| [`product/`](product/) | Домен продукта и mapping фич → DIDAPI / наша логика |
| [`architecture/`](architecture/) | Зафиксированный стек, схема БД, auth, очереди, ingress, деплой |

Правило Runexis: при расхождении HTML-эталон побеждает текст reference, пока gap не закрыт записью в [`reference/runexis/GAPS.md`](reference/runexis/GAPS.md).

HLR / Silent SMS: канон [`reference/smsc/`](reference/smsc/README.md) и [`product/LOOKUP.md`](product/LOOKUP.md). Vendor HTML SMSC в репо нет.

## Быстрый старт

1. Открыть эталон локально: [`vendor/runexis/DIDAPI Documentation.html`](vendor/runexis/DIDAPI%20Documentation.html)
2. Читать обзор: [`reference/runexis/OVERVIEW.md`](reference/runexis/OVERVIEW.md)
3. SMS-контракты: [`reference/runexis/SMS.md`](reference/runexis/SMS.md)
4. Пробелы до кода: [`reference/runexis/GAPS.md`](reference/runexis/GAPS.md)
5. Продукт: [`product/DOMAIN.md`](product/DOMAIN.md), [`product/RUNEXIS_MAPPING.md`](product/RUNEXIS_MAPPING.md)

## Reference index (SMSC)

| File | Contents |
|---|---|
| [README.md](reference/smsc/README.md) | Индекс |
| [OVERVIEW.md](reference/smsc/OVERVIEW.md) | Auth, fmt=3, ретраи, один аккаунт |
| [HLR.md](reference/smsc/HLR.md) | send.php hlr=1 |
| [PING.md](reference/smsc/PING.md) | Silent SMS / ping=1 |
| [STATUS.md](reference/smsc/STATUS.md) | status.php, poll, enrichment |
| [CALLBACK.md](reference/smsc/CALLBACK.md) | Подпись, 401 / 200 applied=false |
| [BALANCE.md](reference/smsc/BALANCE.md) | balance.php, cost=1 |
| [MAPPING.md](reference/smsc/MAPPING.md) | status/err → lifecycle |

## Reference index (Runexis)

| File | Contents |
|---|---|
| [OVERVIEW.md](reference/runexis/OVERVIEW.md) | Base URL, auth header, error envelope |
| [AUTH.md](reference/runexis/AUTH.md) | login / refresh / me / revoke |
| [SMS.md](reference/runexis/SMS.md) | send, statistic, DLR/MO URL, directions |
| [NUMBERS.md](reference/runexis/NUMBERS.md) | partner inventory / get number |
| [SIMS_SMS.md](reference/runexis/SIMS_SMS.md) | informational SIM SMS (not product send) |
| [WEBHOOKS.md](reference/runexis/WEBHOOKS.md) | lifecycle webhooks |
| [GAPS.md](reference/runexis/GAPS.md) | missing contracts |
| [ENDPOINTS.json](reference/runexis/ENDPOINTS.json) | machine-readable catalog (all sections) |

## Product index

| File | Contents |
|---|---|
| [DOMAIN.md](product/DOMAIN.md) | Roles, entities, invariants, v1 surfaces |
| [LOOKUP.md](product/LOOKUP.md) | HLR / Silent SMS: тарифы, Policy B, hold, CSV, пилот +79 |
| [RUNEXIS_MAPPING.md](product/RUNEXIS_MAPPING.md) | Feature → DIDAPI / our logic |

## Architecture index

| File | Contents |
|---|---|
| [README.md](architecture/README.md) | Index |
| [STACK.md](architecture/STACK.md) | ADR стека |
| [OVERVIEW.md](architecture/OVERVIEW.md) | Модули, хосты, потоки |
| [DATA_MODEL.md](architecture/DATA_MODEL.md) | Схема БД |
| [AUTH.md](architecture/AUTH.md) | Admin / Client / API |
| [QUEUES.md](architecture/QUEUES.md) | Outbox, fair share |
| [INGRESS.md](architecture/INGRESS.md) | DLR/MO и callback SMSC |
| [RUNEXIS_ADAPTER.md](architecture/RUNEXIS_ADAPTER.md) | Wire DIDAPI |
| [SMSC_ADAPTER.md](architecture/SMSC_ADAPTER.md) | Адаптер SMSC.ru |
| [LOOKUP.md](architecture/LOOKUP.md) | Очередь проверок, poll, finalize |
| [WEBHOOKS.md](architecture/WEBHOOKS.md) | Исходящие вебхуки проверок |
| [SECURITY.md](architecture/SECURITY.md) | Limits, audit, 152-ФЗ |
| [DEPLOY.md](architecture/DEPLOY.md) | Compose, cutover SMSC |
| [PORTAINER.md](deploy/PORTAINER.md) | Portainer + NPM, редеплой `:latest` |

## Updating the Runexis эталон

1. Replace files under `docs/vendor/runexis/` with a new export (HTML + `*_files/`).
2. Regenerate structured docs:

```bash
python3 scripts/extract_runexis_docs.py
```

3. Re-read `GAPS.md` and update product mapping if contracts changed.
4. Do **not** hand-edit `ENDPOINTS.json` or auto-generated endpoint sections as source of truth — change vendor HTML or the extractor.

## Scope reminder

- Product: Admin panel + client LK (no public registration)
- Provider SMS: Runexis DIDAPI (`https://didapi.runexis.ru`)
- Provider HLR / Silent SMS: SMSC.ru (тот же аккаунт, что старый HLR; один callback URL)
- Numbers: load already-purchased DEF from DIDAPI; no purchase UX in v1
- Campaigns: our jobs over `POST /api/v1/sms/send` (no bulk API in DIDAPI)
