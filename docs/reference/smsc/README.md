# SMSC.ru — структурированный reference

Канон для HLR Lookup и Silent SMS (Ping) в платформе SMS. Vendor HTML в репозитории нет: источник — боевой адаптер и mapper проекта HLR (переносим 1:1, не пересказываем сайт SMSC.ru). При расхождении с маркетингом SMSC.ru побеждает этот reference + тесты маппера.

| Файл | Содержание |
|---|---|
| [OVERVIEW.md](OVERVIEW.md) | Base URL, auth, `fmt=3`, ретраи |
| [HLR.md](HLR.md) | `send.php?hlr=1` |
| [PING.md](PING.md) | `send.php?ping=1` (Silent SMS) |
| [STATUS.md](STATUS.md) | `status.php?all=2` |
| [CALLBACK.md](CALLBACK.md) | Входящий callback, подпись |
| [BALANCE.md](BALANCE.md) | Баланс и оценка себестоимости (`cost=1`) |
| [MAPPING.md](MAPPING.md) | status/err → lifecycle / reachability |

Продуктовые правила (биллинг, hold, cutover): [`docs/product/LOOKUP.md`](../../product/LOOKUP.md).
