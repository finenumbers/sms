# Адаптер SMSC

Пакет `internal/smsc`. Не расширять `internal/runexis`.

Handlers не вызывают SMSC, кроме админ-зондов (balance / cost=1 / connectivity = balance + локальная подпись). Submit и status — только worker.

Секреты и URL: Админка → Настройки (`system_settings`, пароль/API-ключ/секрет колбэка шифруются `APP_MASTER_KEY`). Адаптер читает их на каждый вызов, перезапуск не нужен. Нет кредов — fail-closed (submit не идёт, hold доли RELEASE), `/readyz` зелёный. Env `SMSC_*` не используется.

Идемпотентность send: не повторять, пока request `pending`; повтор после краша — тот же клиентский `id` + poll, не новый send. Find/SaveRequest до HTTP — живой `ctx` Tick (отмена не создаёт новый pending). Update/callback после HTTP — `WithoutCancel` + 5 с, иначе отмена Tick сотрёт `provider_message_id` и следующий poll уйдёт вторым send.

Сырой req/res и snapshot нормализации хранятся отдельно (`provider_lookup_requests` / `provider_lookup_callbacks`). Клиенту raw не отдаём. Бренд «SMSC» вычищать из `error_message` в ЛК, `/v1` и вебхуках.

Маппер: [`docs/reference/smsc/MAPPING.md`](../reference/smsc/MAPPING.md). Тесты маппера переносить из HLR, не писать статусы по памяти.
