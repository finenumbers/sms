-- Operational journal for Admin → Logs. Shorter retention than audit_log.

ALTER TABLE system_settings
    ADD COLUMN ops_retention_days int NOT NULL DEFAULT 14;

CREATE TABLE ops_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at timestamptz NOT NULL DEFAULT now(),
    category text NOT NULL CHECK (category IN ('http', 'didapi', 'queue', 'ingress', 'audit')),
    level text NOT NULL CHECK (level IN ('info', 'warn', 'error')),
    request_id text,
    actor_type actor_type,
    actor_id uuid,
    client_id uuid REFERENCES clients (id),
    action text NOT NULL,
    resource_type text,
    resource_id uuid,
    http_method text,
    http_path text,
    http_status int,
    latency_ms int,
    summary text,
    detail jsonb NOT NULL DEFAULT '{}'::jsonb,
    error text,
    ip inet
);

CREATE INDEX ops_events_created_idx ON ops_events (created_at DESC);
CREATE INDEX ops_events_category_created_idx ON ops_events (category, created_at DESC);
CREATE INDEX ops_events_request_id_idx ON ops_events (request_id);
CREATE INDEX ops_events_client_created_idx ON ops_events (client_id, created_at DESC)
    WHERE client_id IS NOT NULL;
