CREATE INDEX events_expiry_idx ON events (expires_at, id);
CREATE INDEX idempotency_records_expiry_idx ON idempotency_records (expires_at, created_at);
CREATE INDEX deliveries_org_status_created_idx ON deliveries (organization_id, status, created_at DESC, id DESC);
