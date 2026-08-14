# Webhooks проверок

Только события HLR/Ping, не SMS.

События: `check.completed`, `check.failed`, `job.completed`. Пустой `events[]` = все.

Доставка из очереди worker, at-least-once. Клиент дедупит по `id` доставки.

Подпись: HMAC-SHA256, заголовок `X-Finenumbers-Signature: t=<unix>,v1=<hex>`, тело `${t}.${rawBody}`. Ещё: `X-Finenumbers-Delivery-Id`, `X-Finenumbers-Event`, `User-Agent: Finenumbers-Webhooks/1.0`.

Retry: 30s × 2^(attempt-1), max из Settings (дефолт 8), timeout 5s. Ровно 20 подряд неудач на endpoint → auto-disable.

Секрет шифровать `secret.Keyring` (`APP_MASTER_KEY` / `APP_MASTER_KEY_PREVIOUS`). Payload `snake_case`. Raw SMSC и бренд провайдера не включать.

CRUD — кабинет. Public API — только чтение.
