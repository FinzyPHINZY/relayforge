ALTER TABLE outbox ADD COLUMN available_at timestamptz NOT NULL DEFAULT now();
CREATE INDEX outbox_available_idx ON outbox (available_at, id) WHERE status='pending';
