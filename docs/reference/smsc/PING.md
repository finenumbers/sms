# Silent SMS (Ping)

В API и БД тип проверки — `ping`. В тарифах продукт — `silent_sms`. В меню ЛК — «Проверка Silent SMS».

`POST /sys/send.php` с `ping=1`, `fmt=3`. Тот же жизненный цикл и биллинг Policy B, что у HLR. Тариф HLR на Ping не действует.

Колонки Excel для Ping короче (без блока сети IMSI/MCC/MNC) — как в HLR export.
