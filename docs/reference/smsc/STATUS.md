# status.php

`POST /sys/status.php`, для HLR extras — `all=2`, `fmt=3`.

Используется:

- poll, если callback не пришёл (`poll_interval_sec`, дефолт 30 с);
- enrichment после terminal HLR;
- повтор после краша (тот же клиентский `id`).

Порог «устарел, надо опросить» = `poll_interval_sec`, не timeout. Жёсткий стоп: возраст ≥ `check_timeout_sec` (3600) **или** попыток > 120 (`poll_max_attempts`, константа, не Settings).
