CREATE TABLE delivery_reviews (
    delivery_id uuid PRIMARY KEY,
    organization_id uuid NOT NULL,
    status text NOT NULL CHECK (status IN ('investigating','resolved')),
    note text NOT NULL DEFAULT '' CHECK (octet_length(note) <= 500),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (delivery_id, organization_id),
    FOREIGN KEY (delivery_id, organization_id) REFERENCES deliveries(id, organization_id)
);
CREATE INDEX delivery_reviews_org_status_idx ON delivery_reviews (organization_id, status);

CREATE TABLE delivery_review_history (
    id uuid PRIMARY KEY,
    delivery_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    api_key_id uuid NOT NULL,
    status text NOT NULL CHECK (status IN ('investigating','resolved')),
    note text NOT NULL DEFAULT '' CHECK (octet_length(note) <= 500),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (delivery_id, organization_id) REFERENCES deliveries(id, organization_id),
    FOREIGN KEY (api_key_id, organization_id) REFERENCES api_keys(id, organization_id)
);
CREATE INDEX delivery_review_history_delivery_idx ON delivery_review_history (delivery_id, created_at, id);
