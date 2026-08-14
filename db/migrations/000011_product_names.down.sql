UPDATE tariff_plans
SET name = 'SMS на номера 7…'
WHERE code = 'sms-domestic';

UPDATE tariff_plans
SET name = 'SMS международный'
WHERE code = 'sms-international';

UPDATE tariff_plans
SET name = 'HLR Lookup'
WHERE code = 'hlr-default';
