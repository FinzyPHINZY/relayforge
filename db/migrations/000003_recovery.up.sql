ALTER TABLE endpoint_secrets ADD COLUMN overlap_until timestamptz;

CREATE TABLE replay_requests (
    organization_id uuid NOT NULL,
    source_delivery_id uuid NOT NULL,
    idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 255),
    replay_delivery_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, source_delivery_id, idempotency_key),
    UNIQUE (replay_delivery_id),
    FOREIGN KEY (source_delivery_id, organization_id) REFERENCES deliveries(id, organization_id),
    FOREIGN KEY (replay_delivery_id, organization_id) REFERENCES deliveries(id, organization_id)
);

CREATE INDEX replay_requests_created_idx ON replay_requests (created_at);
