# Finenumbers SMS Service — Architecture

Зафиксированная production-архитектура v1. Домен продукта: [`docs/product/DOMAIN.md`](../product/DOMAIN.md). Mapping на Runexis: [`docs/product/RUNEXIS_MAPPING.md`](../product/RUNEXIS_MAPPING.md). Проверки номеров: [`docs/product/LOOKUP.md`](../product/LOOKUP.md).

| Документ | Содержание |
|---|---|
| [STACK.md](STACK.md) | ADR стека |
| [OVERVIEW.md](OVERVIEW.md) | Модули, хосты, потоки |
| [DATA_MODEL.md](DATA_MODEL.md) | Схема БД и инварианты |
| [AUTH.md](AUTH.md) | Admin / Client / API |
| [QUEUES.md](QUEUES.md) | Outbox, fair share, анти-дубли |
| [INGRESS.md](INGRESS.md) | DLR/MO и SMSC callback |
| [RUNEXIS_ADAPTER.md](RUNEXIS_ADAPTER.md) | Wire-контракт DIDAPI |
| [SMSC_ADAPTER.md](SMSC_ADAPTER.md) | Адаптер SMSC.ru |
| [LOOKUP.md](LOOKUP.md) | Очередь HLR / Ping |
| [WEBHOOKS.md](WEBHOOKS.md) | Исходящие вебхуки проверок |
| [SECURITY.md](SECURITY.md) | Rate limits, аудит, секреты, 152-ФЗ |
| [DEPLOY.md](DEPLOY.md) | Portainer + NPM, cutover SMSC |
| [PORTAINER.md](../deploy/PORTAINER.md) | Чеклист Portainer + NPM, редеплой `:latest` |

Правило: при расхождении кода и этих файлов побеждает architecture + product domain, пока не принят новый ADR.
