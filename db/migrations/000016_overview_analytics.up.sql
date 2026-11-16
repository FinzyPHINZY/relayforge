CREATE INDEX deliveries_org_completed_idx ON deliveries (organization_id, completed_at DESC)
WHERE status IN ('succeeded','exhausted','cancelled');
