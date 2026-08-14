# ADR-001 — Стек v1

Статус: accepted.

## Решение

- API и workers: Go (min 1.24), chi, slog JSON, один бинарь `cmd/sms` с режимом `api` / `worker` / `all` / `migrate`
- Контракт: OpenAPI 3 в [`api/openapi.yaml`](../../api/openapi.yaml) — источник для трёх поверхностей. HTTP-хендлеры v1 пишутся на chi вручную. SPA ходит в те же пути handwritten-клиентом (`web/ui`). `oapi-codegen` — позже, когда контракт перестанет двигаться.
- БД: PostgreSQL 16, sqlc + pgx, golang-migrate
- Redis 7: только rate limit и кэш токена Runexis. Сессии — в PostgreSQL
- Очередь: transactional outbox в PostgreSQL (`FOR UPDATE SKIP LOCKED`)
- UI (волна 8): React + TypeScript + Vite, `web/admin`, `web/client`, общий `web/ui`. Не Next.js. В проде dist кладётся в образ; отдаёт процесс `api` по `Host`
- Пароли: Argon2id. API-ключи: SHA-256(pepper ∥ secret)
- Секрет Runexis: AES-256-GCM в БД, `dek_key_id` для ротации
- Край: Nginx Proxy Manager (уже есть на хосте, сеть `proxy`). Деплой: Portainer-стек Docker Compose на VM в РФ. Caddy нет
- Наблюдаемость: Prometheus `/metrics` (не через публичные хосты NPM); `/healthz` `/readyz`; `request_id`. Grafana/Loki/Alertmanager — не v1

## Почему не иначе

- Nest/FastAPI: провайдер — bottleneck; Go даёт один статический бинарь и предсказуемые воркеры
- Kafka/NATS: лишняя операционка при 10–30 SMS/s и одном DIDAPI-агенте
- Сессии в Redis: рестарт кэша разлогинивает B2B-операторов
- Argon2id на API-ключах: CPU-DoS на каждый запрос
