ALTER TABLE endpoint_circuits ADD COLUMN active_claims integer NOT NULL DEFAULT 0 CHECK (active_claims >= 0);
UPDATE endpoint_circuits ec SET active_claims=x.n FROM (
    SELECT endpoint_id,count(*)::integer AS n FROM deliveries WHERE status='processing' GROUP BY endpoint_id
) x WHERE ec.endpoint_id=x.endpoint_id;

CREATE TABLE organization_scheduling (
    organization_id uuid PRIMARY KEY REFERENCES organizations(id),
    active_claims integer NOT NULL DEFAULT 0 CHECK (active_claims >= 0),
    last_claimed_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE FUNCTION create_organization_scheduling() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO organization_scheduling (organization_id) VALUES (NEW.id);
    RETURN NEW;
END $$;
CREATE TRIGGER organization_scheduling_insert AFTER INSERT ON organizations FOR EACH ROW EXECUTE FUNCTION create_organization_scheduling();

INSERT INTO organization_scheduling (organization_id,active_claims)
SELECT o.id,COALESCE(x.n,0) FROM organizations o LEFT JOIN (
    SELECT organization_id,count(*)::integer AS n FROM deliveries WHERE status='processing' GROUP BY organization_id
) x ON x.organization_id=o.id
ON CONFLICT (organization_id) DO NOTHING;
