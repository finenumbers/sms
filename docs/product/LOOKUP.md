# HLR Lookup и Silent SMS

Домен проверок номеров в платформе SMS. Провайдер — SMSC.ru ([reference](../reference/smsc/README.md)). Не смешивать с рассылками SMS и Runexis.

## Решения

- Данные старого проекта HLR не мигрируем.
- Кошелёк клиента один: SMS, HLR и Silent SMS тратят `available_balance`.
- В тарифе только `sell_price`. Себестоимость SMSC — зонд админа, не поле плана.
- Публичный API в стиле SMS (`snake_case`). Пути `/v1/checks`, `/v1/jobs`. Тип в JSON: `hlr` | `ping`.
- Пилот: только `+79XXXXXXXXX`. Чужой номер в списке — отказ всего списка.
- Аккаунт SMSC тот же, что у старого HLR. Параллельных приёмников колбэка нет.
- Креды SMSC и флаг `lookup_enabled` — Админка → Настройки, не env. Код выкатывается с `lookup_enabled=false`. Услуга открывается флагом и назначением тарифа.

## Сущности

Одиночная проверка = `lookup_jobs` + один `lookup_item`. Отдельной модели Check нет.

Статусы item: `queued → reserved → pending → completed | failed`. SENT в отчёты не кладём.

Статусы job: `queued → processing → completed | completed_with_errors | failed`. Отмены нет (в старом HLR статус был в схеме без API).

## Тарифы

Продукты: `hlr`, `silent_sms`. Назначение unique `(client_id, product)`. Нет тихого дефолта → `409 tariff_not_configured`. HLR-тариф не действует на Ping.

Матрица: none / hlr-only / ping-only / both. Оценка, создание, reserve и ЛК режутся по назначению запрошенного типа.

`ping` в API/БД ↔ продукт `silent_sms` только в биллинге.

## Биллинг (Policy B + hold на известный N)

Снимок `unit_sell_price` + план на job и каждый item в момент, когда items созданы. Дальше только снимок.

HOLD одной проводкой на `unit_sell_price × N` в той же транзакции, что insert items:

- JSON / submit CSV-preview: сразу.
- `POST /v1/jobs/csv`: shell без hold; после parse — affordability по снимку × N и HOLD. Не хватило — job `failed`, SMSC нет.

Текущий SMS `settle()` для этого не использовать: один DEBIT закрывает hold. Lookup: остаток = hold − SUM(дочерние DEBIT+RELEASE). SMS-reaper не трогать.

Поле `actual_cost` в JSON job/item — сумма **списанного sell** (capture доли). Это не себестоимость SMSC и не цена кабинета провайдера. После RELEASE поле пустое. Себестоимость SMSC клиенту не отдаём; админу — только зонд «Мониторинг».

| Исход item | Деньги |
|---|---|
| Финал провайдера (доступен / недоступен / err) | DEBIT доли |
| Send не удался, timeout, dead-letter, тариф снят, suspend до submit | RELEASE доли |
| Поздний callback после RELEASE | ничего: item не открывать |

Снятие тарифа после accept: SMSC не звать, `tariff_not_configured`, RELEASE доли. Живой assignment — gate на submit.

Клиент `suspended`: новые HTTP 403; worker не делает новый submit; уже `pending` дожимаем.

## CSV

Два потока:

- ЛК: preview `ready → consuming → consumed` (ошибка → снова `ready`). TTL 30 мин. Submit — единственное создание job+HOLD. Потолок `max_csv_rows` (100000), не `max_batch_phones` (1000).
- API: `POST /v1/jobs/csv` — async parse, без preview-таблицы.

## Номера

Нормализация в E.164 с `+`. Не использовать `msisdn.NormalizeDest` (пропустит `770…`). Callback сверяет цифры.

## Что не делаем

Миграция клиентов/истории HLR; себестоимость в тарифе; sync HTTP; ключи из ЛК (создаёт админ); автоназначение тарифов; чистка `phone_e164` retention; поле `smscBaseUrl` в админке (в HLR мёртвое).
