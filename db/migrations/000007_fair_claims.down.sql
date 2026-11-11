DROP TRIGGER organization_scheduling_insert ON organizations;
DROP FUNCTION create_organization_scheduling();
DROP TABLE organization_scheduling;
ALTER TABLE endpoint_circuits DROP COLUMN active_claims;
