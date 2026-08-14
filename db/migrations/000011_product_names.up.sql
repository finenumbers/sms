UPDATE tariff_plans
SET name = 'SMS / Russia',
    description = 'Цена за 1 PDU. Номера 7XXXXXXXXXX, как в направлениях платформы (включая 77…).'
WHERE code = 'sms-domestic';

UPDATE tariff_plans
SET name = 'SMS / International'
WHERE code = 'sms-international';

UPDATE tariff_plans
SET name = 'HLR Lookup'
WHERE code = 'hlr-default';
