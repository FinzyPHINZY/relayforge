CREATE INDEX delivery_attempts_org_started_idx ON delivery_attempts (organization_id, started_at DESC);
