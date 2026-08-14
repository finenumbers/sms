# Callback SMSC

## URL

Один на аккаунт, задаётся **вручную** в кабинете SMSC.ru:

`https://api.{domain}/internal/smsc/callback`

Не путать с Runexis: там токен в path. Здесь подпись, токена в URL нет.

## Приём

`GET` (query) и `POST` (query + body; при конфликте ключей побеждает body).

Подпись: md5 и/или sha1 от строки `id:phone:status:<секрет из Настроек>`. Значение в полях `md5`/`sha1` или заголовках `X-SMSC-MD5` / `X-SMSC-SHA1`. Нет секрета / нет подписи / не совпало → **401**. IP allowlist нет; лимит 600 RPM с IP.

Неизвестный `id` / `provider_message_id` → **200** и `applied=false`, чтобы SMSC не ретраил вечно. Сверка телефона — по цифрам без `+`.

Сначала сырой payload в БД, затем 200. Нормализация и биллинг — worker. Тот же маппер, что у poll.

После timeout + RELEASE поздний callback **не** открывает item и **не** списывает деньги.
