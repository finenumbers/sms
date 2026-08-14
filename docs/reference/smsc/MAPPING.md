# Маппинг status / err

Канон: mapper старого HLR. Не упрощать «по документации SMSC с сайта».

| status | err | lifecycle | result | reachable |
|---|---|---|---|---|
| нет, нет err и нет текста | — | pending | unknown | null |
| -3 | любое | failed | error | null |
| -1, -2, 0 | любое | pending | pending | null |
| 1 или 2 | нет | pending | pending | null |
| 1 или 2 | 0 | completed | reachable | true |
| 1 или 2 | ≠ 0 | completed | unreachable | false |
| ≥ 3 (в т.ч. 20+) | любое | completed | unreachable | false |
| нет status | 0 | completed | reachable | true |
| нет status | ≠ 0 | completed | unreachable | false |
| нет status/err, есть error message | — | failed | error | null |
| не-JSON тело | — | failed | error | null |

`error_code` на уровне HTTP API SMSC (не поле `err` проверки) — отдельная ошибка send/status: item не уходит в «провайдер дал финал».

Policy B: `completed` с reachable **или** unreachable (включая err) → списание клиентской цены. RELEASE только если send не удался или вышел timeout до финала.
