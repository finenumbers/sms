CREATE INDEX sms_campaigns_created_idx ON sms_campaigns (created_at DESC, id DESC);

CREATE INDEX sms_campaigns_from_created_idx ON sms_campaigns (from_msisdn, created_at DESC, id DESC);

CREATE INDEX campaign_recipients_campaign_created_idx ON campaign_recipients (campaign_id, created_at, id);

CREATE INDEX sms_messages_from_created_idx ON sms_messages (from_msisdn, created_at DESC, id DESC);

CREATE INDEX sms_messages_to_created_idx ON sms_messages (to_msisdn, created_at DESC, id DESC);

CREATE INDEX sms_messages_inbound_created_idx ON sms_messages (created_at DESC, id DESC)
    WHERE direction = 'inbound';

CREATE INDEX sms_messages_direct_outbound_idx ON sms_messages (created_at DESC, id DESC)
    WHERE direction = 'outbound' AND campaign_id IS NULL;

CREATE INDEX sms_messages_outbound_inflight_idx ON sms_messages (created_at DESC, id DESC)
    WHERE direction = 'outbound' AND status IN ('queued', 'accepted', 'sent');

CREATE INDEX sms_messages_failed_created_idx ON sms_messages (direction, created_at DESC, id DESC)
    WHERE status = 'failed';

CREATE INDEX provider_callback_events_sms_message_idx ON provider_callback_events (sms_message_id, created_at DESC)
    WHERE sms_message_id IS NOT NULL;
