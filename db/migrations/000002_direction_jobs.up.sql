CREATE TYPE direction_job_status AS ENUM ('pending', 'processing', 'done', 'retry', 'dead');

CREATE TABLE number_direction_jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    def_number_id uuid NOT NULL REFERENCES def_numbers (id),
    assignment_id uuid REFERENCES number_assignments (id),
    msisdn text NOT NULL,
    dir_in boolean NOT NULL,
    dir_dom_out boolean NOT NULL,
    dir_int_out boolean NOT NULL,
    dir_in_mass boolean NOT NULL,
    status direction_job_status NOT NULL DEFAULT 'pending',
    attempt int NOT NULL DEFAULT 0,
    available_at timestamptz NOT NULL DEFAULT now(),
    locked_at timestamptz,
    locked_by text,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT number_direction_jobs_msisdn_chk CHECK (msisdn ~ '^7[0-9]{10}$')
);

CREATE INDEX number_direction_jobs_claim_idx
    ON number_direction_jobs (available_at)
    WHERE status IN ('pending', 'retry');
CREATE INDEX number_direction_jobs_number_idx ON number_direction_jobs (def_number_id, created_at DESC);
