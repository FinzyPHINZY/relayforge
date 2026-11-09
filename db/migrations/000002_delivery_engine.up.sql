ALTER TABLE endpoint_secrets ADD CONSTRAINT endpoint_secrets_id_org_key UNIQUE (id, organization_id);

CREATE TABLE deliveries (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL,
    event_id uuid NOT NULL,
    endpoint_id uuid NOT NULL,
    run_number integer NOT NULL DEFAULT 1 CHECK (run_number >= 1),
    replayed_from_delivery_id uuid,
    destination_url text NOT NULL,
    timeout_ms integer NOT NULL CHECK (timeout_ms BETWEEN 1000 AND 30000),
    max_attempts integer NOT NULL CHECK (max_attempts BETWEEN 1 AND 8),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','succeeded','retry_wait','exhausted','cancelled')),
    attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0 AND attempt_count <= max_attempts),
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    lease_owner text,
    lease_expires_at timestamptz,
    lease_generation bigint NOT NULL DEFAULT 0 CHECK (lease_generation >= 0),
    last_http_status integer,
    last_error_code text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    UNIQUE (id, organization_id),
    UNIQUE (event_id, endpoint_id, run_number),
    FOREIGN KEY (event_id, organization_id) REFERENCES events(id, organization_id),
    FOREIGN KEY (endpoint_id, organization_id) REFERENCES endpoints(id, organization_id),
    FOREIGN KEY (replayed_from_delivery_id, organization_id) REFERENCES deliveries(id, organization_id),
    CHECK ((status = 'processing') = (lease_owner IS NOT NULL AND lease_expires_at IS NOT NULL))
);
CREATE INDEX deliveries_due_idx ON deliveries (next_attempt_at, id) WHERE status IN ('pending','retry_wait');
CREATE INDEX deliveries_expired_lease_idx ON deliveries (lease_expires_at, id) WHERE status = 'processing';
CREATE INDEX deliveries_org_created_idx ON deliveries (organization_id, created_at DESC, id DESC);
CREATE INDEX deliveries_endpoint_created_idx ON deliveries (endpoint_id, created_at DESC, id DESC);

CREATE TABLE delivery_attempts (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL,
    delivery_id uuid NOT NULL,
    attempt_number integer NOT NULL CHECK (attempt_number >= 1),
    worker_id text NOT NULL,
    lease_generation bigint NOT NULL CHECK (lease_generation >= 1),
    signing_secret_id uuid,
    started_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    duration_ms integer CHECK (duration_ms >= 0),
    http_status integer,
    outcome text NOT NULL DEFAULT 'unknown' CHECK (outcome IN ('success','http_error','timeout','network_error','security_blocked','unknown')),
    error_code text,
    response_excerpt text CHECK (octet_length(response_excerpt) <= 1024),
    UNIQUE (delivery_id, attempt_number),
    FOREIGN KEY (delivery_id, organization_id) REFERENCES deliveries(id, organization_id),
    FOREIGN KEY (signing_secret_id, organization_id) REFERENCES endpoint_secrets(id, organization_id)
);
CREATE INDEX delivery_attempts_delivery_order_idx ON delivery_attempts (delivery_id, attempt_number);
