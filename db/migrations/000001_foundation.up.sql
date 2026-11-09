CREATE TABLE organizations (
    id uuid PRIMARY KEY,
    name text NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    monthly_event_quota integer NOT NULL DEFAULT 10000 CHECK (monthly_event_quota > 0),
    max_endpoints integer NOT NULL DEFAULT 10 CHECK (max_endpoints > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE api_keys (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations(id),
    name text NOT NULL,
    key_prefix text NOT NULL UNIQUE,
    key_hash bytea NOT NULL,
    scopes text[] NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz,
    expires_at timestamptz,
    revoked_at timestamptz,
    UNIQUE (id, organization_id)
);

CREATE TABLE endpoints (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations(id),
    url text NOT NULL,
    description text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paused', 'disabled')),
    timeout_ms integer NOT NULL DEFAULT 10000 CHECK (timeout_ms BETWEEN 1000 AND 30000),
    max_attempts integer NOT NULL DEFAULT 8 CHECK (max_attempts BETWEEN 1 AND 8),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, organization_id)
);
CREATE INDEX endpoints_org_created_idx ON endpoints (organization_id, created_at DESC, id DESC);

CREATE TABLE endpoint_secrets (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL,
    endpoint_id uuid NOT NULL,
    ciphertext bytea NOT NULL,
    nonce bytea NOT NULL,
    key_id text NOT NULL,
    state text NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'overlap', 'retired')),
    created_at timestamptz NOT NULL DEFAULT now(),
    retired_at timestamptz,
    FOREIGN KEY (endpoint_id, organization_id) REFERENCES endpoints(id, organization_id)
);
CREATE UNIQUE INDEX endpoint_one_active_secret_idx ON endpoint_secrets (endpoint_id) WHERE state = 'active';

CREATE TABLE subscriptions (
    organization_id uuid NOT NULL,
    endpoint_id uuid NOT NULL,
    event_type text NOT NULL,
    PRIMARY KEY (endpoint_id, event_type),
    FOREIGN KEY (endpoint_id, organization_id) REFERENCES endpoints(id, organization_id)
);

CREATE TABLE events (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations(id),
    event_type text NOT NULL,
    payload jsonb NOT NULL,
    idempotency_key text,
    payload_sha256 bytea NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL DEFAULT (now() + interval '30 days'),
    UNIQUE (id, organization_id)
);
CREATE INDEX events_org_created_idx ON events (organization_id, created_at DESC, id DESC);

CREATE TABLE outbox (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL,
    event_id uuid NOT NULL UNIQUE,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'dispatched')),
    created_at timestamptz NOT NULL DEFAULT now(),
    dispatched_at timestamptz,
    FOREIGN KEY (event_id, organization_id) REFERENCES events(id, organization_id)
);
CREATE INDEX outbox_pending_idx ON outbox (created_at, id) WHERE status = 'pending';

CREATE TABLE idempotency_records (
    organization_id uuid NOT NULL REFERENCES organizations(id),
    idempotency_key text NOT NULL,
    request_fingerprint bytea NOT NULL,
    event_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL DEFAULT (now() + interval '24 hours'),
    PRIMARY KEY (organization_id, idempotency_key),
    FOREIGN KEY (event_id, organization_id) REFERENCES events(id, organization_id)
);

CREATE TABLE monthly_event_usage (
    organization_id uuid NOT NULL REFERENCES organizations(id),
    month date NOT NULL,
    event_count integer NOT NULL DEFAULT 0 CHECK (event_count >= 0),
    PRIMARY KEY (organization_id, month)
);
